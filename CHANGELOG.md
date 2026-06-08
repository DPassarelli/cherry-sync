# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the project is pre-1.0, the public interface may change in minor releases.

## [Unreleased]

(none)

## [0.2.2-rc1] - 2026-06-07

Dry run of smoke test.

## [0.2.1] - 2026-06-06

### Fixed

- Files that are byte-identical but carry a different modification time are no longer reported as phantom changes. This is common after a git checkout, which restamps every file's mtime and so differs per machine. The comparison now settles each file by content hash (`rsync --checksum`) rather than rsync's default size-and-mtime quick-check, so the change list reflects real content differences — a file whose only difference is its timestamp is correctly treated as unchanged.
- Files whose names contain non-ASCII characters (accents, emoji, CJK, or the narrow no-break space in a macOS screenshot name) now transfer correctly instead of failing. rsync escapes such bytes in its comparison output by default; csync now requests the raw bytes (`rsync -8`) so the selected path matches the real file and the transfer succeeds.

## [0.2.0] - 2026-06-03

### Added

- When the local side is a git repository, files it ignores (per `.gitignore`, `.git/info/exclude`, and global excludes) are left out of the comparison, so the diff shows only files worth moving. The local repository's ignore rules apply in both directions; no git is required on the remote. The `.git` directory itself is always excluded for a repository, so a sync never offers git's internal metadata for transfer. csync discloses what it held back (`Excluded: the .git directory and N gitignored path(s)`) so nothing is withheld silently. The approach — driving rsync's `--exclude-from` with the output of `git ls-files`, letting git itself resolve gitignore semantics — was inspired by [BenteVE/rsync-gitignore](https://github.com/BenteVE/rsync-gitignore).

## [0.1.0] - 2026-06-02

### Added

- Interactive "compare, then sync" workflow that wraps `rsync` over SSH: review what differs between a local and a remote directory before any files move.
- Source and destination given as positional arguments (`csync SOURCE DESTINATION`). Push and pull are both supported, with direction inferred from argument order, the way `rsync` reads `src dest`.
- Dry-run comparison (`rsync --dry-run --itemize-changes`) that renders each difference as a numbered create/update action in a stable, file-tree order.
- Interactive selection at the prompt: Enter or `a` to sync every change, `n` to sync nothing, or a single number to sync just that change.
- Selective transfer of the chosen files via `rsync --files-from` (NUL-delimited).
- Compatibility with both GNU rsync and macOS's openrsync; the `--itemize-changes` output is parsed without assuming implementation-specific field widths.
- Hardened rsync invocation: commands run with no shell, and a `--` separator precedes the path operands so a path beginning with `-` cannot be parsed as an rsync option.

[Unreleased]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.0...0.2.1
[0.2.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dpassarelli/cherry-sync/releases/tag/v0.1.0
