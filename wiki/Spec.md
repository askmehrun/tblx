# The TBLX Specification

**Version 1.0** · status: frozen · magic: `TBLX`

tblx (pronounced "tab-lix") is a binary, column-oriented container for
tabular data. This page is the *complete* specification: if a detail is
not written here, it is not part of the format.

> TL;DR — a 14-byte fixed header, a list of typed column definitions, an
> array of per-column byte lengths, then one contiguous data block per
> column. Little-endian everywhere. Three types. No padding. No NULLs.

---

## 1. Design goals

1. **Trivially implementable.** A correct reader or writer fits in a
   couple of hundred lines in any language. The reference implementation
   (Go) uses only the standard library.
2. **Column-seekable.** Reading one column out of a hundred reads only
   that column's bytes — one `seek`, no scanning.
3. **Terminal-friendly.** Built for tools that live in a TTY: fast to
   open, cheap to introspect, predictable errors.
4. **Honest about scope.** No compression, no schema evolution ritual,
   no nested types, no dictionary encoding *in v1*. Reserved space is
   provided so these can arrive later without breaking the layout
   (see §9).

## 2. Conventions

| Term | Meaning |
|---|---|
| **u8 / u16 / u64** | unsigned integers of 8 / 16 / 64 bits, **little-endian** |
| **i64** | signed 64-bit integer, little-endian, two's complement |
| **f64** | IEEE 754 binary64 (double), little-endian bit layout |
| **string** | UTF-8 bytes prefixed with a u32 byte length; no terminator |
| **column** | an ordered sequence of exactly `row_count` values of one type |

All multi-byte integers are little-endian. There is **no padding or
alignment** anywhere: every field starts immediately after the previous
one ends.

## 3. File layout

A TBLX file is the concatenation of six regions, in order:

```
┌────────────────────────────────────────────┐
│ 1. magic         4 bytes   "TBLX"          │  offset 0
│ 2. row_count     8 bytes   u64             │  offset 4
│ 3. col_count     2 bytes   u16             │  offset 12
│ 4. column defs   variable  (see §5)        │  offset 14
│ 5. length array  8 × col_count bytes       │  after defs
│ 6. data blocks   variable, back-to-back    │  after lengths
└────────────────────────────────────────────┘
```

The fixed header is therefore always exactly **14 bytes**.

## 4. Header fields

### 4.1 Magic

| offset | size | content |
|---|---|---|
| 0 | 4 | `0x54 0x42 0x4C 0x58` — ASCII `TBLX` |

The magic is both the identity check and the format version. Readers
**must** reject any file whose first four bytes are not `TBLX`, and
**should** fail fast with a clear message (the reference reader does).

### 4.2 row_count

| offset | size | type | meaning |
|---|---|---|---|
| 4 | 8 | u64 | total number of rows in the table |

`0` is legal (a table with columns but no rows).

### 4.3 col_count

| offset | size | type | meaning |
|---|---|---|---|
| 12 | 2 | u16 | number of columns (0 … 65 535) |

`0` columns is *not* legal in practice: every writer must emit at least
one column definition.

## 5. Column definitions

Repeated `col_count` times, immediately after the header:

| field | size | type | notes |
|---|---|---|---|
| `name_len` | 1 | u8 | byte length of the name, 1 … 255 |
| `name` | name_len | UTF-8 | column name, **not** null-terminated |
| `type` | 1 | u8 | data type (see §6) |
| `flags` | 1 | u8 | reserved — **must be 0** in v1 |

Names are opaque to the format; duplicates are discouraged but the
format does not forbid them (the reference writer passes them through).

## 6. Data types

| code | name | on-disk encoding | Go type | missing value |
|---|---|---|---|---|
| `1` | int | i64, little-endian (8 bytes) | `int64` | `0` |
| `2` | float | f64 IEEE 754 LE (8 bytes) | `float64` | `0.0` |
| `3` | string | u32 length + UTF-8 bytes | `string` | `""` |

Any other code is invalid; readers must reject the file.

**TBLX has no NULL.** Absence is encoded as the type's default value
above. If your data needs real nulls, model them in a companion
`*_present` int column (0/1) until §9.2 lands.

## 7. Length array

Immediately after the last column definition:

| field | size | meaning |
|---|---|---|
| `lengths[i]` | 8 × col_count | byte size of data block *i* |

This array is the heart of the format. Because blocks are stored
back-to-back in definition order, the absolute offset of block *k* is:

```
offset(block k) = 14
                + Σ (2 + len(name_i))   for i in 0..col_count   (definitions)
                + 8 × col_count                                  (this array)
                + Σ lengths[i]          for i in 0..k-1
```

A reader parses the header once, keeps `lengths`, and can then jump to
*any* column with a single seek — regardless of how many rows the file
holds. That is the whole trick.

## 8. Data blocks

`col_count` blocks, back-to-back, in definition order, with no
separators. Each block contains exactly `row_count` values:

* **int** — `row_count × 8` bytes; row *r* lives at `r × 8`.
* **float** — `row_count × 8` bytes; row *r* lives at `r × 8`.
* **string** — per row: u32 length, then that many UTF-8 bytes.
  Variable-width, so strings are read sequentially *within* their block.

For int and float columns, `lengths[i]` is always exactly
`row_count × 8`; readers may use this as a cheap integrity check (the
reference reader does).

## 9. Worked example

A table with 2 rows and 2 columns — `name` (string), `age` (int) —
holding `("Alice", 30)` and `("Bob", 25)`:

```
54 42 4C 58                           ← magic "TBLX"
02 00 00 00 00 00 00 00               ← row_count = 2
02 00                                 ← col_count = 2
-------------------------------------------
04 6E 61 6D 65 03 00                  ← name_len=4, "name", type=3 (string), flags=0
03 61 67 65 01 00                     ← name_len=3, "age",  type=1 (int),    flags=0
-------------------------------------------
10 00 00 00 00 00 00 00               ← lengths[0] = 16 bytes (string block)
10 00 00 00 00 00 00 00               ← lengths[1] = 16 bytes (int block)
-------------------------------------------
    -- block 0 (string)
05 00 00 00 41 6C 69 63 65            ← len=5, "Alice"
03 00 00 00 42 6F 62                  ← len=3, "Bob"
    -- block 1 (int)
1E 00 00 00 00 00 00 00               ← 30
19 00 00 00 00 00 00 00               ← 25
```

Total size: **75 bytes**. Reading only `age` touches exactly 16 of them.

## 10. Limits & validation

| constraint | bound |
|---|---|
| columns per file | 65 535 (u16) |
| column-name length | 1 … 255 bytes (u8) |
| single string value | < 4 GiB (u32 length) |
| rows | 2⁶⁴ − 1 (u64) — bounded in practice by disk |
| type codes | 1, 2, 3 only |
| flags byte | must be 0 in v1 |

Readers must validate the magic, type codes, and that the file is long
enough for the declared lengths; the reference implementation reports
the offending column and row in every decode error.

## 11. Versioning & the future

The magic **is** the version: a layout change means a new magic. Within
TBLX, two escape hatches are reserved for the future:

1. **Flag bits** (one byte per column). Bit proposals so far:
   * `0x01` — block is zstd-compressed (`lengths[i]` stays the
     *compressed* size; a second pass yields the value count).
   * `0x02` — a presence bitmap precedes the block (real NULLs).
   Readers seeing an unknown set bit must refuse the column, not guess.
2. **Trailing footer.** Nothing forbids appending bytes after the last
   block; a future revision may add an end-of-file index for streaming
   append. v1 readers ignore trailing bytes.

What is explicitly **out of scope** for TBLX: nested/structured types,
in-format compression dictionaries, encryption, and multi-file datasets.
When you need those, you have outgrown the napkin — and that is fine.

---

**Reference implementations:** the [Go core](Library) (`libtblx`),
its [C ABI](Library#the-c-abi) (`libtblx.so`), the
[Python binding](Library#python), the `tblx` CLI, and the interactive
encoder on the project website — all byte-compatible with this page.
