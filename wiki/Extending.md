# Extending Tablix

Tablix is deliberately small, but every seam is load-bearing and
documented. This page is the how-to for each extension axis — with the
exact files you'll touch, in order.

---

## Adding a data type (e.g. `bool`, `timestamp`)

Type codes are wire format, so **pick the next free code and never reuse
one** (1 int · 2 float · 3 string → `bool = 4`, `timestamp = 5` …).

1. **`libtblx/schema.go`** — add the constant, its `String()` name, and
   accept it in `Validate()`. Decide the cycle order in `Next()`/`Prev()`
   (the wizard rotates through this).
2. **`libtblx/writer.go`** — a new case in `writeValue`: encode one
   value, and enforce the Go type (`bool`, `int64`-nanoseconds, …).
3. **`libtblx/reader.go`** — the mirror case in `ReadColumn`.
4. **`libtblx/converter.go`** — `ConvertCSV` parsing rule and, if
   sensible, let `GuessTypes` detect it.
5. **`libtblx/clib/clib.go`** — the packed encoding for the new type in
   `tbl_col_data` / `tbl_writer_set_col` (document it in the package
   comment; bit-packed `bool` would be 1 bit per value — but then
   `lengths` math changes, so v1-style 1 byte/value is the safe start).
6. **`libtblx/python/tblx.py`** — the constant, `_TYPE_NAMES`, and the
   pack/unpack lines in `_encode_column` / `_decode_column`.
7. **`tblxweb` (`src/lib/tbl.ts` + playground)** — keep the website's
   mirror engine honest; it exists precisely to catch drift like this.
8. **[[Spec]]** — amend §6 and bump nothing else: new codes are
   backwards-compatible for readers that reject unknown types.

Tests to add: a round-trip fixture file and a Go↔Python
byte-identity case (see the conformance idea below).

## Adding a CLI command

Commands are a flat switch in **`tblx/cmd/tblx/main.go`** (`run()`),
each backed by one small `cmdX()` function that composes `libtblx`.

```go
case "describe":
    if len(args) != 2 { return fmt.Errorf("usage: tblx describe <file.tblx>") }
    return cmdDescribe(args[1])
```

Conventions worth keeping: errors wrap with `%w` and mention the file;
human output goes to stdout, diagnostics to stderr; anything
interactive lives in `internal/tui`, never in `main.go`.

Ideas that fit the architecture especially well:

* **`describe`** — min/max/mean/distincts per column, computed by
  reading *one column at a time* (the format's payoff, on display).
* **`slice --cols a,b --rows 1000:2000`** — partial reads; print
  bytes-read vs file-size to prove the seek index works.
* **`query`** — register columns as a SQLite virtual table and hand
  users real SQL over a columnar file.
* **`diff a.tblx b.tblx`** — schema diff + row diff.

## Adding a TUI feature

Both models are plain Bubble Tea `Model`s in `tblx/internal/tui/`:

* **`view.go`** pre-renders the table to padded strings once; scroll
  (`j/k`, `pgup/pgdn` via `LineUp/LineDown` — portable across bubbles
  versions) and pan (`←/→` shifting a rune offset) never re-decode data.
  Sort/filter features should operate on the pre-rendered lines, not on
  the decoded table.
* **`import.go`** is the wizard; the type cycle comes from
  `tblx.DataType.Next()/Prev()`, and initial guesses from
  `tblx.GuessTypes` — extend the *library*, not the TUI.

Charm pins matter: `bubbletea v0.25`, `bubbles v0.18` (`Update` returns
`(tea.Model, tea.Cmd)`; no `HalfPageUp` yet), `lipgloss v0.10`.

## Adding a language binding

The C ABI is the contract ([[Library#the-c-abi]]):

1. `make lib` in `libtblx` → `libtblx.so` + `libtblx.h`.
2. Declare the signatures in your FFI of choice.
3. Implement the ownership dance: copy out of every returned pointer,
   then `tbl_free` it; check negative returns and call `tbl_last_error`.
4. Decode/encode columns with the packed layouts — for numerics this is
   a bulk memcpy/frombuffer, not a loop.
5. Add a round-trip test against a fixture written by the Go core.

## Using the reserved flag bits (compression sketch)

The `flags` byte per column is the sanctioned extension point
([[Spec#11-versioning--the-future]]). A zstd sketch:

* writer: compress each block, set `flags |= 0x01`, store the
  *compressed* size in the length array;
* reader: if the bit is set, decompress before decoding; `ColLen`
  reports the on-disk size, a new `ColLenUncompressed` the rest;
* old readers see an unknown bit and refuse the column loudly — which
  is exactly the intended failure mode.

NULLs (`0x02`) would prepend a presence bitmap to the block; the value
bytes for absent rows stay the type default, keeping offsets stable.

Rule of thumb: **a flag bit may change how a block is stored, never how
it is located** — the length array must stay trustworthy.

## Packaging for another distro

* **Debian/Ubuntu** — mirror `tblx/packaging/rpm/tblx.spec` as a
  `debian/` directory; the build is one static `go build`, so the
  packaging is almost entirely metadata.
* **Nix** — a flake wrapping the same build; `CGO_ENABLED=0` keeps it
  trivial.
* **AUR** — the existing `PKGBUILD` is already AUR-shaped
  (`pkgname=tblx`, source tarball, `makepkg`); submit it as-is.
* Whatever the target: keep the `sed -i '/^replace /d' go.mod &&
  go mod tidy` step — distro builds must resolve `libtblx` from GitHub,
  not from a sibling checkout.

## Conformance & testing (the boring, load-bearing part)

The format's value is that independent implementations agree byte for
byte. The test suite worth building:

1. **Golden files** — a handful of committed `.tblx` fixtures (empty
   table, all three types, unicode strings, ragged CSV) with expected
   SHA-256s; the writer must reproduce them exactly.
2. **Go ↔ Python identity** — write with Go, read with Python (via the
   C bridge), write back, compare hashes.
3. **Fuzz the reader** — `go test -fuzz` with random/truncated/corrupt
   inputs; the reader must return errors, never panic or hang.
4. **Website mirror tests** — the TypeScript engine encodes/decodes in
   the browser; a vitest suite comparing its output to the golden files
   catches spec drift at PR time.

## Contributing conventions

* Errors: `fmt.Errorf("tblx: …: %w", err)` — prefix, context, wrap.
* The format core stays **stdlib-only**; dependencies belong in the CLI.
* Docs live in three places that must agree: godoc comments, the
  [[Spec]], and the website. Change all three in one PR.
