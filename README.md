<div align="center">

<img src="csync-logo.gif" alt="cherry-sync logo" />

An interactive `rsync` wrapper (CLI-based) for moving files selectively between a local machine and a remote dev environment over SSH. Cherry-pick your sync!

[![test](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml/badge.svg)](https://github.com/dpassarelli/cherry-sync/actions/workflows/test.yml)

</div>

## Problem

When working across a local machine and a remote dev environment over SSH, the workflow for moving files back and forth is a little clumsy. You either have to memorize `rsync` parameters or just move everything and sort through the details later. Plain `scp` has no incremental mode. What's missing is the middle step: see what's different, then pick what moves, and in which direction.

This is a common situation for anyone working with SSH-accessible dev boxes — cloud instances, Proxmox containers, Raspberry Pis, WSL-to-host, etc. [At least one macOS app](https://github.com/rsyncOSX/RsyncUI) exists for this purpose, but nothing that is cross-platform, runs directly on the command line, and has a rich terminal UX ⌨️ 🤓

## What this tool does

An interactive CLI that wraps `rsync` to provide a select-then-sync workflow:

1. Compare local and remote directories (using `rsync --dry-run --itemize-changes`).
2. Automatically exclude `.git/` and csync's own `.csync.toml`, and honor `.gitignore`/`.git/info/exclude`
3. Show a human-readable list of differences (new, modified, deleted — with direction).
4. Let the user choose: sync all, none, or individual files.
5. Execute the transfer for only the selected files (using `rsync --files-from`) and report the outcome.

## Design principles

- **rsync does the heavy lifting.** This tool is a UX layer, not a reimplementation. It parses rsync's output and drives rsync's `--files-from` for selective transfer.
- **SSH is the transport.** No additional daemon or agent on the remote side. If you can `ssh` to it, this tool works.
- **No opinion on direction.** Push and pull are both first-class. Bidirectional diff display is a future goal.
- **Minimal dependencies.** `rsync` and `ssh` must be present on both sides. The tool itself should be easy to install.

## Requirements

- `rsync` and `ssh` on both the local and remote machines
- Go ≥ 1.26.3 — only to build from source (prebuilt binaries need just `rsync`/`ssh`)

## Install

Download the archive for your platform from the [latest release](https://github.com/dpassarelli/cherry-sync/releases/latest), then extract `csync` onto your `PATH`:

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
csync ./local-dir user@host:/remote-dir
```

csync compares the two directories and lets you choose what to transfer. In a terminal you get an interactive picker — arrow keys to move, space to toggle a file, `a` for all/none, Enter to sync, `ctrl-c` to cancel. When input or output is redirected (a pipe, a file, CI), csync falls back to a typed prompt on stderr — so the report on stdout stays uncluttered — where you press **Enter** (or `a`) to sync every change, `n` or `ctrl-c` to cancel, or pick a subset by number: a single number, a range like `1-3`, a comma list like `1,3`, or any combination (`1-2,4`); whitespace around the numbers is ignored.

If the two directories are identical, csync reports there is nothing to sync and exits cleanly. A missing or wrong number of arguments prints a usage message on stderr and exits with code 2.

### Saved targets

To avoid retyping a remote you sync with often, save it in a `.csync.toml` in the project directory:

```toml
remote = "user@host:/remote-dir"
```

Then, from that directory, `csync push` sends the project to the saved remote and `csync pull` brings it down:

```sh
cd ./local-dir
csync push
```

The `push`/`pull` verbs take no other arguments. csync never offers the `.csync.toml` itself for transfer — like `.git/`, it is held out of the comparison and reported as excluded. If the file is missing, malformed, or sets no `remote`, csync says so and exits non-zero rather than guessing. An explicit `csync SOURCE DESTINATION` is unaffected and does not read `.csync.toml` for its target.

## Roadmap

Near-term enhancements are tracked as [open `enhancement` issues](https://github.com/DPassarelli/cherry-sync/issues?q=is%3Aissue%20state%3Aopen%20label%3Aenhancement) on GitHub.

Further out: bidirectional diff (showing which side is newer) and conflict flagging when a file has changed on both sides.

See the [CHANGELOG](CHANGELOG.md) for what has shipped in each release.

## Known limitations

- **Submodules' nested `.git` is not excluded.** Only the top-level `.git/` directory is held out of a sync. A repository containing submodules carries nested `.git` directories (or `.git` files) deeper in the tree, and those are still offered for transfer. If you sync a superproject, expect that metadata in the diff and deselect it.

## Development

**Disclaimer:** I am a real person with many years of software engineering experience. I personally came up with this idea on my own, and I am the one driving the product design, monitoring the development process, writing the commit messages, and approving releases; however, I am heavily relying on [Claude](https://www.anthropic.com/product/claude-code) to write code and analyze security issues on this project. The evidence of this is sprinkled throughout.

The development process is mainly described in [TESTING](TESTING.md), with additional concerns covered in [STYLE](STYLE.md) and [SECURITY](SECURITY.md). Automated testing and a thorough CI workflow have been in place since the beginning in order to assure quality and reliability. The Gherkin specifications found under `_features/` are the canonical definition of expected behavior and usage for this application.

### Run tests:

```sh
go test -count=1 ./...
```

The `-count=1` is deliberate: the suite builds and execs the `csync` binary rather than importing it, so Go's test cache doesn't notice production-code changes and a plain `go test ./...` can report a stale pass. See [TESTING](TESTING.md#running-tests) for the details.

## License

MIT — see [LICENSE](LICENSE).
