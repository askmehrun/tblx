# FAQ & troubleshooting

## Building

**`go mod tidy` fails with "replacement directory ../libtblx does not exist"**
The CLI's `go.mod` carries a local `replace` for development. Either
clone `libtblx` as a sibling directory (`git clone …/libtblx` next to
`tblx`), or strip the directive and resolve from GitHub:

```sh
go mod tidy        # needs libtblx to be public + tagged (v0.1.0)
```

**`go build` complains about `go.sum`**
`go.sum` is generated, not committed from a fresh checkout:
`go mod tidy` once after cloning.

**What Go version do I need?**
1.21+ for the modules; the release workflow uses 1.23.

## The Python binding

**`TblxError: libtblx.so not found`**
Build it once from the `libtblx` root:

```sh
make lib                                   # → libtblx.so + libtblx.h
# or point at an existing build:
TBLX_LIB=/usr/local/lib/libtblx.so python3 my_script.py
```

**`make lib` fails with "c-shared requires cgo"**
`CGO_ENABLED=1` is required for the shared library (the Makefile sets
it). You need a C toolchain — `base-devel` on Arch, `gcc` on Fedora,
Xcode CLT on macOS.

**Is the Python binding "pure Python"?**
The *file* is stdlib-only ctypes, but it deliberately contains **no
format logic** — every operation runs the compiled Go core through
`libtblx.so`. That's the point: one implementation, byte-identical
output everywhere.

## Files & format

**`bad magic` on a file from an older build**
Files written by pre-release alpha builds carry an incompatible
signature. Re-import the source CSV — `tblx import data.csv` — which is
lossless, since only the signature changed.

**Does TBLX support NULLs?**
No — by design, for now. Empty cells become `0`, `0.0` or `""`. If you
need real nulls today, keep a companion `*_present` int column; a
presence-bitmap flag bit is the planned v-next mechanism
([[Spec#11-versioning--the-future]]).

**Why little-endian?**
It's the native order of essentially every machine this runs on, so
numeric columns can be memory-mapped without byte-swapping. The spec
fixes it so implementations never negotiate.

**How big can files get?**
Rows are u64 and strings are u32-prefixed (< 4 GiB each), so the
practical limits are disk and the per-column full read in `ReadColumn`.
Row-group chunking is the known answer for very large files
([[Extending]]).

## Packaging & releases

**How do the GitHub releases build packages?**
`.github/workflows/release.yml` runs on every published release: the
RPM is built inside a `fedora:41` container, the pacman package via
`makepkg` inside an `archlinux` container, plus static binaries for
linux/darwin × amd64/arm64. Assets are attached automatically. Trigger
it without a release from **Actions → release → Run workflow**.

**The release build fails resolving `libtblx`**
Package builds strip the local `replace` and fetch the module from
GitHub — `libtblx` must be **public** and have a **version tag**
(`v0.1.0`) before you release `tblx`.

## The website

**GitHub Pages shows a blank page**
The classic base-path problem: a project site is served from
`/tblxweb/`, so absolute `/assets/…` URLs 404. The deploy workflow
builds with `--base=./`; if deploying manually, do the same:

```sh
npx vite build --base=./
```

**The site build fails importing Go sources**
The source browser embeds both repos at build time. In a standalone
`tblxweb` checkout, add them as submodules:

```sh
git submodule add https://github.com/askmehrun/libtblx libtblx
git submodule add https://github.com/askmehrun/tblx tblx
```
