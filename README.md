# Cherry-Sync

An interactive rsync wrapper for moving files selectively between a local machine and a remote dev environment over SSH.

[![test](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml/badge.svg)](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml)

## Problem

When working across a local machine and a remote dev environment over SSH, the workflow for moving files back and forth is clumsy. Plain `rsync` transfers everything or nothing. Plain `scp` has no incremental mode. What's missing is the middle step: see what's different, then pick what moves and in which direction.

This is a common situation for anyone working with SSH-accessible dev boxes — cloud instances, Proxmox containers, Raspberry Pis, WSL-to-host, etc.

## What this tool does

An interactive CLI that wraps `rsync` to provide a select-then-sync workflow:

1. Compare local and remote directories (using `rsync --dry-run --itemize-changes`).
2. Show a human-readable list of differences (new, modified, deleted — with direction).
3. Let the user choose: sync all, or select individual files.
4. Execute the transfer for only the selected files (using `rsync --files-from`).

## Design principles

- **rsync does the heavy lifting.** This tool is a UX layer, not a reimplementation. It parses rsync's output and drives rsync's `--files-from` for selective transfer.
- **SSH is the transport.** No additional daemon or agent on the remote side. If you can `ssh` to it, this tool works.
- **No opinion on direction.** Push and pull are both first-class. Bidirectional diff display is a post-v0.1 goal.
- **Minimal dependencies.** `rsync` and `ssh` must be present on both sides. The tool itself should be easy to install.

## Roadmap

Near-term: fixing transfer of non-ASCII filenames (see [Known limitations](#known-limitations)), bounding rsync with a timeout so a stalled transfer can't hang the tool, a multi-select grammar (ranges like `1-3`, lists like `1,3`), honoring `.gitignore` or a custom exclude file, and a `--version` flag.

Further out: bidirectional diff (showing which side is newer), delete detection, and conflict flagging when a file has changed on both sides.

See the [CHANGELOG](CHANGELOG.md) for what has shipped in each release.

## Requirements

- `rsync` and `ssh` on both the local and remote machines
- Go ≥ 1.26.3 — only to build from source (prebuilt binaries need just `rsync`/`ssh`)

## Install

Download the archive for your platform from the [latest release](https://github.com/dpassarelli/cherry-sync/releases/latest) — Linux and macOS, `amd64` and `arm64` — then extract `csync` onto your `PATH`:

```sh
tar -xzf cherry-sync_<version>_darwin_arm64.tar.gz   # match the version and platform you downloaded
sudo mv csync /usr/local/bin/
```

Each release also publishes a `checksums.txt` you can verify the archive against.

### Build from source

```sh
go build ./cmd/csync
```

Produces a `csync` binary at the repo root.

## Usage

```sh
$ ./csync ./local-dir user@host:/remote-dir
Source: ./local-dir
Destination: user@host:/remote-dir
Changes: 2
  1. update README.md
  2. create src/adder.go
Press Enter to sync all changes:
Synced: 2
```

At the prompt: press **Enter** (or `a`) to sync every change, `n` to cancel without transferring anything, or a **number** to sync just that one change. The prompt is written to stderr, so the report on stdout stays clean and parseable.

If the two directories are identical:

```sh
$ ./csync ./local-dir user@host:/remote-dir
Source: ./local-dir
Destination: user@host:/remote-dir
Changes: 0
No changes to sync.
```

Missing or wrong number of arguments prints a usage message on stderr and exits with code 2.

## Known limitations

- **Non-ASCII filenames don't transfer yet.** Names containing non-ASCII bytes — accented characters, emoji, or the narrow no-break space in macOS screenshot names — are escaped in rsync's diff output, and `csync` doesn't yet round-trip the escaped name back to the transfer. The change is listed but the transfer fails. Fix targeted for the next release.
- **No exclude support yet.** `csync` doesn't honor `.gitignore` or a custom exclude file, so pointing it at a repository directory will list `.git/` and build artifacts among the changes. Excludes are planned (see [Roadmap](#roadmap)).

## Development

Run all tests (unit + godog scenarios):

```sh
go test ./...
```

The `features/` directory holds the Gherkin specification and acts as the canonical TODO list — every scenario maps to a desired behavior. See [TESTING.md](TESTING.md) for the testing philosophy and [STYLE.md](STYLE.md) for code style rules that aren't enforced by `gofmt` or `go vet`.

After cloning, activate the git hooks once:

```sh
lefthook install
```

This installs a `commit-msg` hook that enforces Conventional Commits, and a `pre-push` hook that runs `go vet ./...` and `gosec ./...`. See [`lefthook.yml`](lefthook.yml) for details.

## License

MIT — see [LICENSE](LICENSE).
