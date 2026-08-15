# Codebase tour

Every file in the project, what it does, and the decisions that aren't
obvious from reading it. Four trees: **libtblx** (the format), **tblx**
(the CLI), and **tblxweb**
(the website).

```
libtblx/     the format   (Go module: github.com/askmehrun/libtblx)
tblx/        the CLI+TUI  (Go module: github.com/askmehrun/tblx)
tblxweb/     the website  (React + Vite)
```

---

## libtblx — the format core

The core is deliberately split so each concern has exactly one home.
Dependency rule: **stdlib only** — every dependency belongs in the CLI.

| file | role |
|---|---|
| `doc.go` | package overview, `Magic` (`"TBLX"`), `Version`. The map for newcomers. |
| `type.go` | `DataType` (u8 codes 1/2/3), `String()`, `Validate()`, and the `Next()`/`Prev()` cycle the wizard's ←/→ keys rotate through. |
| `header.go` | `Header`/`ColumnDef` structs and their (de)serialisation — `writeHeader`/`readHeader` work on plain `io.Reader`/`io.Writer`, so they're testable without touching a file. |
| `column.go` | **the codec table** — one `codec{fixedSize, encode, decode}` entry per type. This is THE extension point: a new type is a constant in `type.go` plus one entry here. Also hosts the per-value encoders (int64 LE, float64 LE, u32-prefixed UTF-8). |
| `writer.go` | public `Writer`. `encodeTo(io.WriteSeeker)` writes header → zero length placeholders → blocks → seeks back and patches the lengths. Never buffers the data twice. |
| `reader.go` | public `Reader`. Parses header + length array on open (O(columns)), then `ReadColumn` seeks to `dataStart + Σ lengths[..k)` and decodes through the codec. Fixed-width columns get an integrity check against the length array before decoding. |
| `csv.go` | `ConvertCSV` (strings → typed columns, errors carry column + CSV line) and `GuessTypes` (int if all samples parse as int64, else float, else string). |
| `value.go` | `FormatValue` — the single renderer shared by `info`, `view`, `export` and the C bridge, so outputs can't drift. |
| `libtblx_test.go` | golden-byte test pinning the spec's 75-byte worked example, round-trip over all three types + unicode + missing values, bad-magic and unknown-type rejection, `GuessTypes` behaviour. |
| `go.mod` | module declaration. Zero `require` lines — that's the point. |

### `clib/` — the C ABI

`clib.go` compiles the core to `libtblx.so` via `-buildmode=c-shared`
(`make lib` also emits `libtblx.h`). Design notes:

* **Handles, not pointers.** `tbl_open` returns an int64 handle into a
  mutex-guarded registry; every call re-validates it.
* **Ownership is one-directional.** Anything returned (`tbl_col_data`,
  `tbl_col_name`, `tbl_export_csv`, `tbl_guess_types`) is `C.CString`/
  `C.CBytes`-allocated and MUST be freed with `tbl_free`.
* **Columns cross as packed bytes** in exactly the on-disk encodings —
  so a binding's decode is a bulk `frombuffer`, not a loop.
* **Errors travel out-of-band**: negative return + `tbl_last_error()`.
* The exported symbols keep the short `tbl_` prefix (the format-level
  ABI); the Go package itself is `tblx`.

### `python/` — the binding

* `tblx.py` is stdlib-only **ctypes**: no format logic in Python at all.
  It loads `libtblx.so` (search order: `$TBLX_LIB`, repo root, CWD),
  declares the ABI, and marshals columns with `struct` packs that match
  the packed layouts. `Table` offers `rows()`, `to_dicts()`, `schema()`.
* `example.py` is the round-trip demo: `from_csv` → `write` → `read` →
  `import_csv` fast path → `to_csv`.

## tblx — the CLI

| file | role |
|---|---|
| `cmd/tblx/main.go` | subcommand switch (`import`, `view`, `info`, `export`, `help`) plus small composable `cmdX()` functions. `readCSV` tolerates ragged rows; `humanSize` formats sizes. |
| `internal/tui/import.go` | the Bubble Tea wizard. Initial types come from `libtblx.GuessTypes`; ←/→ rotate via `DataType.Next()/Prev()`; the wizard owns presentation only — all format knowledge stays in the library. |
| `internal/tui/view.go` | the viewer. Key decision: it **pre-renders** the table to padded strings once; scrolling (`j/k`, pgup/pgdn via `LineUp/LineDown` — portable across bubbles versions) and horizontal pan (a rune offset) never re-decode. Frozen header and sticky row gutter are drawn around a `bubbles/viewport`. |
| `go.mod` | requires `libtblx v0.1.0` with a `replace => ../libtblx` for development; distro builds strip the `replace` and resolve from GitHub. |
| `packaging/rpm/tblx.spec`, `packaging/arch/PKGBUILD` | single-static-binary packages; both strip the `replace` and `go mod tidy` before building. |
| `.github/workflows/release.yml` | on every published release: RPM in a `fedora:41` container, pacman package via `makepkg` in `archlinux`, static binaries linux/darwin × amd64/arm64, all attached to the release. |
| `.github/workflows/ci.yml` | vet + build on push/PR. |


## tblxweb — the website

| file | role |
|---|---|
| `src/lib/tbl.ts` | the browser mirror of the codec (powers the live playground). A third implementation that must stay byte-identical. |
| `src/lib/csv.ts`, `highlight.tsx`, `i18n.tsx`, `clipboard.ts` | CSV parsing (+ `guessType` mirroring `GuessTypes`), the syntax highlighter, the string table, clipboard helpers. |
| `src/components/HexDump.tsx` | the annotated byte dump — hover/click a field, watch the annotation follow. |
| `src/components/Playground.tsx` | live encode (CSV → hex) and decode (drop a `.tblx`). |
| `src/components/SourceBrowser.tsx` | embeds the real sources of libtblx and tblx via `?raw` imports. |
| `src/data/files.ts` | the file registry the browser renders. |
| `.github/workflows/deploy.yml` | GitHub Pages deploy with `--base=./` (the fix for the classic blank-page problem). |

## Cross-cutting decisions

1. **One codec table.** Go, TypeScript and the C bridge all reduce "how
   is type T stored?" to a single lookup. New types touch one registry
   per language.
2. **Length placeholders, patched in place.** The writer seeks back
   instead of buffering — memory stays flat regardless of table size.
3. **The length array is an integrity check, not just an index.**
   Fixed-width columns must satisfy `length == rows × 8` before a
   single value is decoded.
4. **Errors name the place.** `tblx: column "age", line 7: cannot parse
   "xx" as int` — every decoder error carries column and row.
5. **Three implementations, one golden file.** `libtblx_test.go` pins
   the spec's example byte-for-byte; the TS mirrors and the binding are
   held to it.
