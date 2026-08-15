# libtblx — the library

`libtblx` is the reference implementation of the [[Spec|TBLX format]].
One codebase, three doors:

```
        ┌─────────────── Go  (import the module directly)
libtblx ┼─────────────── C   (libtblx.so via cgo, -buildmode=c-shared)
        └─────────────── Python (ctypes bridge; no format logic in Python)
```

Everything below the C boundary is the *same compiled Go code* the CLI
uses, so all three doors produce byte-identical files.

---

## Go

```sh
go get github.com/askmehrun/libtblx
```

### Writing

```go
import tblx "github.com/askmehrun/libtblx"

w, err := tblx.NewWriter(
    []string{"name", "age", "score"},
    []tblx.DataType{tblx.DTypeString, tblx.DTypeInt, tblx.DTypeFloat},
    [][]any{
        {"Alice", "Bob"},
        {int64(30), int64(25)},
        {95.5, 87.0},
    },
)
if err != nil { /* shape/type validation failed */ }
err = w.Write("people.tblx")
```

`NewWriter` validates eagerly: equal column lengths, legal types,
non-empty names ≤ 255 bytes, ≤ 65 535 columns. `Write` streams the
blocks, emits zero placeholders for the length array, and patches them
by seeking back — the table is never buffered twice.

### Reading

```go
r, err := tblx.NewReader("people.tblx")
if err != nil { /* bad magic, truncated header, invalid type… */ }
defer r.Close()

fmt.Println(r.NRows, r.NCols, r.ColNames, r.ColTypes)

ages, _ := r.ReadColumn(1)       // []any of int64 — ONE seek, one block read
n, _ := r.ColLen(2)              // byte size of the score block

rows, _ := r.ReadAll()           // []map[string]any — everything, transposed
```

`NewReader` parses only the header + length array (O(columns)), so
opening is cheap no matter how many rows the file has; data is decoded
lazily, per column.

### CSV helpers

```go
data, err := tblx.ConvertCSV(headers, rows, types) // strings → typed columns
types := tblx.GuessTypes(cols)                     // int > float > string
s := tblx.FormatValue(v)                           // canonical cell rendering
```

Errors from `ConvertCSV` carry the column name **and** the CSV line
number, e.g. `tblx: column "age", line 6: cannot parse "thirty" as int`.

---

## The C ABI

Build the shared library (emits `libtblx.so` **and** `libtblx.h`):

```sh
cd libtblx && make lib
# = CGO_ENABLED=1 go build -buildmode=c-shared -o libtblx.so ./clib
```

### Conventions

* Every fallible function returns a negative code or `NULL` and stores a
  human-readable message — fetch it with `tbl_last_error()`.
* Every string/buffer the library hands you is heap-allocated: **free it
  with `tbl_free()`**.
* Columns cross the boundary as packed little-endian bytes, using
  exactly the [[Spec#8-data-blocks|in-block encodings]]:

  | type | packing |
  |---|---|
  | int | 8 bytes × n (i64 LE) |
  | float | 8 bytes × n (f64 LE) |
  | string | per value: u32 length + UTF-8 bytes |

  This means a C/Python/… consumer can `memcpy`/`frombuffer` numeric
  columns with zero per-value work.

### Function reference

| function | returns | purpose |
|---|---|---|
| `tbl_version()` | `char*` | core version, e.g. `"1.0.0"` |
| `tbl_last_error()` | `char*` | message of the last failed call |
| `tbl_free(p)` | — | release any returned pointer |
| `tbl_open(path)` | handle ≥ 0 | open + parse header |
| `tbl_close(h)` | — | close a reader |
| `tbl_rows(h)` / `tbl_cols(h)` | i64 / i32 | table shape |
| `tbl_col_name(h, i)` | `char*` | column name |
| `tbl_col_type(h, i)` | i32 | 1 int · 2 float · 3 string |
| `tbl_col_data(h, i, &len)` | `void*` | packed column bytes (+ length) |
| `tbl_export_csv(h)` | `char*` | whole table as CSV text |
| `tbl_import_csv(csv, out, types)` | 0 / −1 | CSV → TBLX in one call; `types` is `"string,int,float"` or `""` to guess |
| `tbl_guess_types(csv)` | `char*` | guessed types, comma-separated |
| `tbl_writer_new(rows, cols)` | handle | begin a write session |
| `tbl_writer_set_col(h, i, name, type, data, len)` | 0 / −1 | feed one packed column |
| `tbl_writer_finish(h, path)` | 0 / −1 | validate + write the file |
| `tbl_writer_free(h)` | — | drop the writer handle |

Handles are process-global and mutex-guarded, so the library is safe to
call from multithreaded hosts.

---

## Python

```sh
cd libtblx && make lib           # once
python3 python/example.py        # round-trip demo
```

```python
import tblx

t = tblx.from_csv("samples/test.csv")          # types guessed by the Go core
print(t.schema())                              # name:string age:int score:float

tblx.write("people.tblx", t.names, t.types, t.columns)
t2 = tblx.read("people.tblx")
ages = t2.columns[t2.names.index("age")]       # plain Python list of ints
print(t2.to_dicts()[:3])                       # row-major, if you must

tblx.import_csv("big.csv", "big.tblx")         # conversion entirely in Go
print(tblx.to_csv(t2))                         # CSV rendering, also in Go
```

| call | returns | notes |
|---|---|---|
| `tblx.version()` | `str` | loaded core version |
| `tblx.read(path)` | `Table` | header + all columns, validated |
| `tblx.write(path, names, types, columns)` | `None` | bytes produced by the Go writer |
| `tblx.from_csv(path, types=None)` | `Table` | guesses types when omitted |
| `tblx.import_csv(csv, out, types=None)` | `out` | fast path, all in Go |
| `tblx.to_csv(table, path=None)` | CSV `str` | writes the file when path given |
| `Table.rows()` / `.to_dicts()` | rows | transposed views |

Types may be the `INT/FLOAT/STRING` codes or `"int"/"float"/"string"`.
Errors surface as `TblxError` carrying the Go core's message verbatim.

If the shared library isn't next to the module, point at it:

```sh
TBLX_LIB=/usr/local/lib/libtblx.so python3 my_script.py
```

## Other languages

Anything with FFI can drive `libtblx.so` — the ABI above is the whole
surface. Rust (`libloading`), Node (`ffi-napi`/`koffi`), Zig, Julia:
link, declare the signatures from `libtblx.h`, respect the ownership
rules. See [[Extending#adding-a-language-binding]] for a checklist.
