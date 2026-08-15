# tblx — binary columnar files for the terminal

`tblx` is the CLI + TUI of **tblx** (called **Tablix**, pronounced
*tab-lix*): it converts CSV → TBLX through an interactive type wizard,
browses `.tblx` files in the terminal, prints metadata, and exports back
to CSV.

📖 **Full documentation lives in the
[wiki](https://github.com/askmehrun/tblx/wiki)** — the complete
[TBLX spec](https://github.com/askmehrun/tblx/wiki/Spec), the
[library guide](https://github.com/askmehrun/tblx/wiki/Library),
[extending TBLX](https://github.com/askmehrun/tblx/wiki/Extending),
and a [tour of every file](https://github.com/askmehrun/tblx/wiki/Codebase).

The format itself lives in its own library,
[libtblx](https://github.com/askmehrun/libtblx) — this repo only
contains the command-line tool. The website lives in
[tblxweb](https://github.com/askmehrun/tblxweb)

```
tblx/
├── cmd/tblx/main.go             import · view · info · export
├── internal/tui/import.go       Bubble Tea wizard (↑↓ select, ←→ cycle type)
├── internal/tui/view.go         Bubble Tea viewer (frozen header, j/k, pan)
├── packaging/rpm/tblx.spec      Fedora / RHEL package
├── packaging/arch/PKGBUILD      Arch Linux package
├── wiki/                        GitHub wiki pages (see wiki/README.md)
└── samples/test.csv             demo data (includes missing values)
```

## Build

Requires Go ≥ 1.26.

```sh
go mod tidy        # generates go.sum on first checkout
go build -o tblx ./cmd/tblx
```

## Commands

```
tblx import <file.csv>            CSV → TBLX via an interactive type wizard
tblx view   <file.tblx>           scrollable table viewer
tblx info   <file.tblx>           metadata + schema
tblx export <file.tblx> [out.csv] TBLX → CSV (stdout if out.csv is omitted)
```

### Typical Session

```sh
$ tblx import samples/test.csv    # wizard: ↑/↓ select column, ←/→ cycle
                                  #         int → float → string, enter confirms
wrote samples/test.tblx — 5 rows x 3 cols, 180 B
   1. name             string
   2. age              int
   3. score            float

$ tblx info samples/test.tblx
file     samples/test.tblx
format   TBLX (magic "TBLX")
rows     5
columns  3
size     180 B (180 bytes)
schema
   1. name             string  block 26 B
   2. age              int     block 40 B
   3. score            float   block 40 B

$ tblx view samples/test.tblx     # frozen header, j/k scroll, ←/→ pan, q quits

$ tblx export samples/test.tblx roundtrip.csv
```

The import wizard guesses each column's type (via `libtblx.GuessTypes`:
`int` if every sample parses as int64, else `float`, else
`string`) — you only touch the columns it got wrong. TBLX has no NULL:
empty cells are stored as `0` / `0.0` / `""`.

## Packaging

### Fedora / RHEL (RPM)

```sh
cd packaging/rpm
rpmbuild -bb tblx.spec
sudo dnf install "$HOME/rpmbuild/RPMS/$(uname -m)/tblx-1.0.0-1.$(uname -m).rpm"
```

### Arch Linux (pacman)

```sh
cd packaging/arch
makepkg -si
```

Both packages install a single statically-linked binary to `/usr/bin/tblx`
(`CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`).

### Automated: every GitHub Release builds packages for you

`.github/workflows/release.yml` runs whenever you publish a release and
attaches ready-to-install assets to it:

| asset | built in | install |
|---|---|---|
| `tblx-<ver>-1.fc41.<arch>.rpm` | a `fedora:41` container | `sudo dnf install ./tblx-….rpm` |
| `tblx-<ver>-<arch>.pkg.tar.zst` | an `archlinux` container (`makepkg`) | `sudo pacman -U ./tblx-….pkg.tar.zst` |
| `tblx-<ver>-{linux,darwin}-{amd64,arm64}` | static binaries | `chmod +x` and run |
| `tblx-<ver>.tar.gz` | source tarball | — |


## Verifying the format

```sh
$ tblx import samples/test.csv && xxd samples/test.tblx | head -3
00000000: 5442 4c58 0500 0000 0000 0000 0300 046e  TBLX...........n
00000010: 616d 6503 0003 6167 6501 0005 7363 6f72  ame...age...scor
00000020: 6502 0029 0000 0000 0000 0028 0000 0000  e..).......(....
```

The file opens with `54 42 4C 58` ("TBLX"), then `row_count = 5`,
`col_count = 3` — exactly as specified.

## License

MIT
