# TBLX — binary columnar files for the terminal

**TBLX** (called **Tablix**, pronounced *tab-lix*) is a small ecosystem
for getting tabular data *into* a binary columnar format, *looking at
it* in the terminal, and *getting it back out* — without
pulling in a data-engineering stack to do it.

```
CSV ──tblx import──▶  .tblx  ──tblx view────▶  your terminal
                        │      ──tblx export──▶  CSV
                        │      ──tblx info────▶  schema + block sizes
                        ▼
              libtblx (Go · C · Python · …)
```

## Why?

CSV is text: slow to slice, untyped, painful at scale. Parquet is
powerful but heavy — a dependency you feel. TBLX sits in the gap:

* **a spec you can read in two minutes** — [[The TBLX Spec|Spec]]
* **one seek per column** — a length array makes every column directly
  addressable, so reading `age` in a 100-column file never touches the
  other 99
* **three types, zero deps** — int64, float64, string; the Go core is
  stdlib-only and split around a single codec table so new types are a
  one-entry change
* **honest about missing data** — no NULLs; empty becomes `0` / `0.0` / `""`

## The repositories

| repo | what lives there |
|---|---|
| [`libtblx`](https://github.com/askmehrun/libtblx) | the format itself: Go library, C shared library (`libtblx.so`), Python binding |
| [`tblx`](https://github.com/askmehrun/tblx) | the CLI + TUI (`import`, `view`, `info`, `export`), RPM & pacman packaging, release automation, **this wiki** |
| `tblxweb` | the project website with the live in-browser encoder/decoder |

## Five-minute quickstart

```sh
# 1. the CLI (needs Go ≥ 1.21; uses a local replace until libtblx is tagged)
git clone https://github.com/askmehrun/tblx && cd tblx
go mod tidy && go build -o tblx ./cmd/tblx

# 2. convert, browse, export
./tblx import samples/test.csv     # interactive type wizard: ↑↓ ←→ enter
./tblx view  samples/test.tblx     # j/k scroll · ←/→ pan · q quit
./tblx info  samples/test.tblx
./tblx export samples/test.tblx roundtrip.csv
```

Or grab a package from the
[latest release](https://github.com/askmehrun/tblx/releases):
`sudo dnf install ./tblx-*.rpm` · `sudo pacman -U ./tblx-*.pkg.tar.zst`.

## Using the library

```go
import tblx "github.com/askmehrun/libtblx"

r, _ := tblx.NewReader("people.tblx")
defer r.Close()
ages, _ := r.ReadColumn(1)   // seeks straight to column 1 — nothing else is read
```

```python
import tblx                          # ctypes bridge → libtblx.so → Go core
t = tblx.from_csv("samples/test.csv")
tblx.write("people.tblx", t.names, t.types, t.columns)
```

Full API reference: [[libtblx — the library|Library]].

## Documentation

| page | what you'll find |
|---|---|
| [[Spec]] | the complete TBLX file format, byte by byte, with a worked hex example |
| [[Library]] | using `libtblx` from Go, C, and Python — full API & ownership rules |
| [[Extending]] | adding types, commands, languages, compression — the roadmap with receipts |
| [[Codebase]] | a guided tour of **every file** in all the repositories |
| [[FAQ]] | build problems, packaging gotchas, etc. |

## Status

`v1.0` — format frozen, magic `TBLX` (`54 42 4C 58`). Releases ship
RPMs, pacman packages, and static binaries for linux/darwin × amd64/arm64
via GitHub Actions on every tag.