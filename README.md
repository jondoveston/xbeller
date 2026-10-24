# xbeller

List and edit bookmarks in [XBEL](https://en.wikipedia.org/wiki/XBEL) files
(the standard XML bookmark format) using the file commands you already know:
`ls`, `cat`, `mkdir`, `cp`, `mv`, `rm`.

The binary is installed as both `xbeller` and the short alias `xb`.

```console
$ xb bookmarks.xbel ls -l /Work
b  2024-01-02  Spec      https://example.com/spec
d  -           Archive/  12 items

$ xb bookmarks.xbel mkdir -p /Reading/2026
$ xb bookmarks.xbel add /Reading/2026 "Go blog" https://go.dev/blog --desc "news"
$ xb bookmarks.xbel mv /Work/Spec /Reading/2026
$ xb bookmarks.xbel cat /Reading/2026 -o yaml
$ xb bookmarks.xbel rm -r /Work/Archive
```

## Install

Homebrew (macOS on Apple Silicon, Linux on x86-64 and arm64):

```sh
brew install jondoveston/tap/xbeller
```

Nix:

```sh
nix run github:jondoveston/xbeller -- bookmarks.xbel ls
nix profile install github:jondoveston/xbeller
```

From source (needs the Go version in `go.mod`):

```sh
go install github.com/jondoveston/xbeller@latest
```

## Usage

```
xbeller <file> <command> [args]
```

The XBEL file is always the **first argument**. Every command is stateless:
there is no "current folder", so there is no `cd`. Folders and bookmarks are
always named by their full path from the top of the file.

| Command                       | What it does                                          |
| ----------------------------- | ----------------------------------------------------- |
| `ls [-l\|-u] [-R] [path]`     | list a folder (`-l` details, `-u` URLs only, `-R` recursive) |
| `cat [-o json\|yaml\|xml] [path]` | print a bookmark or folder, including everything below it |
| `add <folder> <title> <url>`  | add a bookmark (`-d` sets a description)              |
| `mkdir [-p] <path>...`        | create folders (`-p` creates parents, no error if present) |
| `cp [-r] <source>... <dest>`  | copy (`-r` required for folders)                      |
| `mv <source>... <dest>`       | move, or rename if `dest` is a new name               |
| `rm [-r] [--host GLOB] <path>...` | remove (`-r` required for folders; `--host` removes matching bookmarks) |

Flags that work with every command: `-n/--dry-run` (print what would change,
write nothing), `--backup` (keep the original as `<file>.bak`), `-v/--verbose`
(print each change), `--version`.

`cp` and `mv` follow the Linux rules: if `dest` is an existing folder the
sources go inside it; otherwise `dest` is a new name inside an existing folder
(one source only). Several sources need an existing folder.

### Paths

| Path         | Meaning                                                    |
| ------------ | ---------------------------------------------------------- |
| `/`          | the top of the file                                        |
| `/Work/Spec` | the item titled `Spec` in the folder titled `Work`         |
| `Name[2]`    | the 2nd item titled `Name` (XBEL allows repeated titles)   |
| `A\/B`       | an item whose title contains a slash                       |
| `Name\[2]`   | an item whose title really ends in `[2]`                   |

If a title is ambiguous the command stops and tells you. `ls` prints names in
this form, adding `[n]` where titles repeat, so you can paste them into other
commands. Quote paths in your shell: `[` is a glob character in zsh.

### Listing and removing by URL

`ls -u` prints only the URL of each bookmark, one per line (with `-R`, one flat
list for the whole subtree), ready for `sort`, `xargs` or a `while read` loop.
Two options, both repeatable and independent of each other, need `-u`:

- `--host GLOB` lists only bookmarks whose host matches.
- `--no-query GLOB` removes the query string and fragment (`?...`, `#...`) from
  URLs whose host matches. `'*'` matches every host.

Patterns are globs (`*`, `?`, `[abc]`) matched against the host alone, ignoring
case and port. `example.com` does not match `www.example.com`; give
`'*.example.com'` as well. `*` also matches dots.

`rm --host GLOB` removes the bookmarks that `ls -u --host GLOB` lists, so you
can export a list and then remove it. Each path is a folder to search (one
level, subfolders are left alone) or a single bookmark; no folder is removed.
It cannot be combined with `-r`, and nothing is written if nothing matches.

```console
$ xb bookmarks.xbel ls -u --host example.com --host '*.example.com' --no-query '*' /Done >> done.txt
$ xb bookmarks.xbel -n rm --host example.com --host '*.example.com' /Done   # preview
$ xb bookmarks.xbel --backup rm --host example.com --host '*.example.com' /Done
```

### Behaviour worth knowing

- Edits are all-or-nothing: if any path in a command is wrong, nothing is
  changed. The file is replaced atomically (temp file + rename), keeping its
  permissions, and symlinks are followed.
- `mv` keeps `id` attributes. `cp` gives copies new ids. New bookmarks and
  folders get an `id` only if the file already uses ids, and an `added`
  timestamp. If every id in the file is an integer, new ids continue the
  sequence.
- Unknown elements, `<info>` blocks, comments inside the file, attributes and
  the DOCTYPE are preserved. The first edit re-indents the whole file (two
  spaces), except for [Floccus files](#floccus); commands that change nothing
  do not rewrite the file. Comments outside the `<xbel>` element are dropped.
- Characters that XML 1.0 forbids (control characters such as backspace, which
  show up in titles scraped from web pages) are removed when the file is read,
  so such files can be listed and edited. Commands that write the file say how
  many were removed, because the written file is then well-formed XML.
- Only UTF-8 files are supported.

### Floccus

[Floccus](https://floccus.org) can sync bookmarks through an XBEL file, and is
picky about what it reads back. xbeller recognises its files by the comment
near the top, `<!--- highestId :53547: for Floccus bookmark sync browser
extension -->`, and then:

- gives every new bookmark and folder an integer id, continuing from the
  larger of that counter and the biggest id in the file, and updates the
  counter. Floccus refuses files with a missing or non-integer id, and would
  reuse ids if the counter fell behind.
- does not add `added` attributes, which Floccus does not keep.
- writes the file in Floccus's own layout (top-level items unindented, `&apos;`
  and `&quot;` entities in text, no newline after `</xbel>`), so editing a
  file only shows the changes you made in `diff`.

`cp` renumbers everything it copies; `mv` keeps ids; deleted ids are never
reused. Floccus only stores titles, URLs and ids, so anything else (such as
descriptions) is kept by xbeller but dropped by Floccus the next time it syncs.

## Development

Everything is driven by [go-task](https://taskfile.dev); `nix develop` gives
you Go, task, golangci-lint and gopls at the versions CI uses.

```sh
task            # build ./xbeller
task test       # go test -race -cover ./...
task lint
task run -- bookmarks.xbel ls
task --list
```

The Go version is whatever the stable nixpkgs channel in `flake.nix` provides
(currently nixos-26.05); keep the `go` line in `go.mod` at or below it.

### Nix builds

Nix builds everything, including the release binaries. Go cross-compiles with
`CGO_ENABLED=0`, so one machine can build every target:

```sh
task nix:build     # native build, with tests
task nix:cross     # linux x86-64/arm64 and darwin arm64
task nix:release   # tarballs + SHA256SUMS + the Homebrew formula in ./result
task nix:check     # nix flake check
```

Nix only sees files tracked by git, so `git add` new files before building.
After changing `go.mod`/`go.sum`, run `task nix:vendor-hash` to refresh
`vendorHash` in `flake.nix`.

### Releasing

1. Change `VERSION` and commit.
2. `task release:tag` creates and pushes `v<VERSION>`.
3. The `release` workflow builds `.#release` with Nix, publishes the tarballs
   and `SHA256SUMS` as a GitHub release, and pushes the generated formula to
   `jondoveston/homebrew-tap` (`Formula/xbeller.rb`).

The workflow needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret: a fine-grained token
limited to the `homebrew-tap` repository with **Contents: Read and write**.
