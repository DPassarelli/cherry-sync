# Cherry-Sync

An interactive rsync wrapper for moving files selectively between a local machine and a remote dev environment over SSH.

[![test](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml/badge.svg)](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml)

> **Status: pre-v0.1.** CLI scaffolding only — the tool does not yet perform any rsync operations. See [Status](#status) below for a feature-by-feature breakdown.

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

## Status

| Current                                                  | Planned for v0.1                                       |
| -------------------------------------------------------- | ------------------------------------------------------ |
| Two-positional-arg CLI (echoes `source` and `destination`) | Dry-run diff via `rsync --dry-run --itemize-changes`   |
| Test + lint CI on PRs (`go vet`, `go test`)              | Human-readable change list (new / modified / deleted)  |
| Conventional Commits enforced on PR titles               | Interactive multi-select of files to transfer          |
| Lefthook git hooks (`commit-msg`, `pre-push`)            | Selective transfer via `rsync --files-from`            |
|                                                          | Honor `.gitignore` or a custom exclude file            |

Beyond v0.1: bidirectional diff (showing which side is newer) and conflict flagging when a file has changed on both sides.

## Requirements

- Go ≥ 1.26 (to build from source)
- `rsync` and `ssh` on both the local and remote machines (once v0.1 lands)

## Build

```sh
go build ./cmd/csync
```

Produces a `csync` binary at the repo root.

## Usage

The CLI currently accepts two positional args and prints them back; no synchronization is performed yet.

```sh
$ ./csync ./local-dir user@host:/remote-dir
Source: ./local-dir
Destination: user@host:/remote-dir
```

## Development

Run all tests (unit + godog scenarios):

```sh
go test ./...
```

The `features/` directory holds the Gherkin specification and acts as the canonical TODO list — every scenario maps to a desired behavior. See [CLAUDE.md](CLAUDE.md) for the testing philosophy.

After cloning, activate the git hooks once:

```sh
lefthook install
```

This installs a `commit-msg` hook that enforces Conventional Commits, and a `pre-push` hook that runs `go vet ./...`. See [`lefthook.yml`](lefthook.yml) for details.

## License

MIT — see [LICENSE](LICENSE).
