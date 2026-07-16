# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the project is pre-1.0, the public interface may change in minor releases.

## [Unreleased]

### Added

- The run log now records what a run did: which build of csync produced it, the command line as invoked, the source and destination it resolved, and the rsync comparison it ran (argument vector, exit code, duration) — written as the run proceeds, so a run abandoned at the selection prompt still says what it was. (#82)

## [0.9.0] - 2026-07-15

### Added

- `csync --help` (and `-h`) prints a usage summary and exits. Like `--version` and `--license`, it takes precedence over any operands. (#91)
- `csync --license` prints csync's full MIT license text and exits. The text is embedded in the binary, so it travels with a bare executable rather than depending on a file bundled beside it. `csync --version` now closes with a line pointing at it. (#84)
- csync now records every run to a log file. Logs are written to either `$XDG_STATE_HOME/cherry-sync/` or `~/.local/state/cherry-sync/` (when `XDG_STATE_HOME` is unset), and csync names the file it wrote as it exits. Runs that do no work (`--version`, `--license`, a usage error) write nothing. (#82)

### Changed

- An invalid invocation now explains what was wrong and points to `csync --help`, instead of printing a bare `usage: csync SOURCE DESTINATION`. A mistyped verb (`csync pll`) is called out as an unknown command that suggests `push`/`pull`; a lone or missing path says a source and destination are both required; every message ends with a pointer to `--help`. (#91)
- Releases now publish each platform's `csync` as a bare executable (e.g. `cherry-sync_1.0.0_darwin_arm64`) instead of a `tar.gz`. There is nothing left to unpack — the binary carries its own license — so installing is a download, a `chmod +x`, and a move onto your `PATH`. Downloads must be made executable before they will run. (#84)

## [0.8.0] - 2026-07-08

### Added

- csync now detects and applies file deletions. A file present on the destination but missing from the source is shown in red and is selected by default. The post-sync summary reports deletions on their own (i.e., `Sync complete! (3 files total, 2 of which were removed)`). Unlike `rsync`, deletes are always included by csync. _Files whose names contain a leading space or an rsync filter metacharacter (`*`, `?`, `[`) are currently excluded and will not appear in the selection list nor be deleted._ (#52)

## [0.7.0] - 2026-07-05

### Added

- The interactive picker's prompt now states how many files are available to select (`Choose which files to sync (N available):`), so a rendered list waiting for input reads as complete rather than frozen. (#64)
- Every release now verifies the published `linux/arm64` binary by running it on real arm64 hardware before the release goes live, matching the pre-publish checks already run for the other platforms. (#58)

### Fixed

- csync now excludes nested `.git` metadata at any depth, the same way it excludes the top-level `.git/`. (#56)

## [0.6.0] - 2026-07-04

### Added

- csync now reports its own version. `csync --version` prints the version, a one-line description, and the project URL, then exits; it takes precedence over any operands. The version line reads `cherry-sync v1.2.3`, or `cherry-sync (dev build)` for a build made without a stamped version (for example `go build ./cmd/csync`). When csync runs in a terminal, the same version line also appears at the top of the interactive header. (#54)

### Fixed

- A saved remote written with a `~` home shortcut (`remote = "user@host:~/working"`) now syncs instead of listing changes and then failing the transfer with `rsync exit 12`. csync now resolves the shortcut to the equivalent relative path (which rsync interprets against the remote login home), and notes the change inline beside the operand in the header. A `~user` shortcut, which no relative path can reach, is rejected up front with a clear error instead of failing mid-transfer. A trailing slash on a remote is also collapsed so the displayed path stays clean. (#50)

## [0.5.0] - 2026-07-03

### Fixed

- The interactive picker now stays usable when the change list is taller than the terminal. Previously a long list overflowed the screen: the cursor's row, its highlight, and the prompt all scrolled out of view, and the picker looked frozen — arrow keys appeared to do nothing because the cursor was moving off-screen. (#60)

## [0.4.0] - 2026-06-26

### Added

- When csync runs in a terminal, it now shows an interactive picker for choosing which changes to sync. When input or output is redirected (a pipe, a file, or a script), csync falls back to the existing typed prompt, so non-interactive and scripted use is unchanged.
- After a sync, csync now prints a one-line `Sync complete! (N files)` summary, replacing the terse `Synced: N` line.
- When csync runs in a terminal, it now leads with a `cherry-sync` banner above the `Source:` and `Destination:` lines, and those values are emphasized and column-aligned to frame the picker. Redirected output drops the banner and the emphasis, keeping the plain, aligned header it already printed.

### Changed

- The disclosure of what was held out of a comparison now reads as a parenthetical aside — `(excluding .csync.toml, the .git directory, and 5 gitignored paths)` — instead of an `Excluded:` label line.

## [0.3.0] - 2026-06-26

### Added

- The selection prompt now accepts multi-select responses instead of only a single number:
    - a hyphen range like `1-3` picks an inclusive span of changes
    - a comma list like `1,3` picks exactly the changes named
    - these can be combined in one response (`1-2,4`)
    - a change named by more than one member (`1-3,2`) is reduced correctly
    - whitespace between members and around a range's bounds is ignored, so `1 - 2, 4` works the same as `1-2,4`
    - members are bounded to the listed changes; a reversed, out-of-range, or otherwise malformed response is rejected like any other unrecognized entry
- `csync push` and `csync pull` sync against a remote saved in a project-local `.csync.toml` (a `remote = "user@host:/path"` line), so a frequently-used remote no longer has to be retyped.
- csync now holds its own `.csync.toml` out of every comparison — like `.git/`, it is never offered for transfer — and names it on the `Excluded:` line so the omission is visible.

## [0.2.1] - 2026-06-06

### Fixed

- Files that are byte-identical but carry a different modification time are no longer reported as phantom changes.
- Files whose names contain non-ASCII characters (accents, emoji, CJK, or the narrow no-break space in a macOS screenshot name) now transfer correctly instead of failing.

## [0.2.0] - 2026-06-03

### Added

- When the local side is a git repository, files it ignores (per `.gitignore`, `.git/info/exclude`, and global excludes) are left out of the comparison, so the diff shows only files worth moving. The local repository's ignore rules apply in both directions; no git is required on the remote. The `.git` directory itself is always excluded for a repository. csync discloses what it held back (`Excluded: the .git directory and N gitignored path(s)`) so nothing is withheld silently.

The approach taken (that is, driving rsync's `--exclude-from` with the output of `git ls-files`) was inspired by [BenteVE/rsync-gitignore](https://github.com/BenteVE/rsync-gitignore).

## [0.1.0] - 2026-06-02

### Added

- Interactive "compare, then sync" workflow that wraps `rsync` over SSH: review what differs between a local and a remote directory before any files move.
- Source and destination given as positional arguments (`csync SOURCE DESTINATION`). Push and pull are both supported, with direction inferred from argument order, the way `rsync` reads `src dest`.
- Dry-run comparison (`rsync --dry-run --itemize-changes`) that renders each difference as a numbered create/update action in a stable, file-tree order.
- Interactive selection at the prompt: Enter or `a` to sync every change, `n` to sync nothing, or a single number to sync just that change.
- Selective transfer of the chosen files via `rsync --files-from` (NUL-delimited).
- Compatibility with both GNU rsync and macOS's openrsync; the `--itemize-changes` output is parsed without assuming implementation-specific field widths.
- Hardened rsync invocation: commands run with no shell, and a `--` separator precedes the path operands so a path beginning with `-` cannot be parsed as an rsync option.

[Unreleased]: https://github.com/dpassarelli/cherry-sync/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.0...0.2.1
[0.2.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dpassarelli/cherry-sync/releases/tag/v0.1.0
