# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the project is pre-1.0, the public interface may change in minor releases.

## [Unreleased]

## [0.5.0] - 2026-07-03

### Fixed

- The interactive picker now stays usable when the change list is taller than the terminal. Previously a long list overflowed the screen: the cursor's row, its highlight, and the prompt all scrolled out of view, and the picker looked frozen — arrow keys appeared to do nothing because the cursor was moving off-screen. The list now scrolls to keep the cursor visible — holding still while the cursor moves within view and scrolling only once it reaches the top or bottom edge — with `▲ more above` / `▼ more below` indicators showing when there are changes off-screen in either direction. The banner and the source/destination header also stay on screen above the picker instead of scrolling off the top. (#60)

## [0.4.0] - 2026-06-26

### Added

- When csync runs in a terminal, it now shows an interactive picker for choosing which changes to sync: an arrow-key (or `j`/`k`) cursor over a `[x]`/`[ ]` checkbox list, grouped by directory and color-coded by change type (green for new files, yellow for modified). Unchecked files are dimmed so the selected set stands out at a glance. Use `space` to toggle a file, `a` to toggle all on or off, `Enter` to sync the checked files, and `Ctrl-C`/`Esc`/`q` to cancel without transferring anything. When input or output is redirected (a pipe, a file, or a script), csync falls back to the existing typed prompt, so non-interactive and scripted use is unchanged.
- After a sync, csync now prints a one-line `Sync complete! (N files)` summary, replacing the terse `Synced: N` line. The interactive picker already shows which files were chosen, so the summary reports just the count rather than re-listing every file.
- When csync runs in a terminal, it now leads with a `cherry-sync` banner above the `Source:` and `Destination:` lines, and those values are emphasized and column-aligned to frame the picker. Redirected output drops the banner and the emphasis, keeping the plain, aligned header it already printed.

### Changed

- The disclosure of what was held out of a comparison now reads as a parenthetical aside — `(excluding .csync.toml, the .git directory, and 5 gitignored paths)` — instead of an `Excluded:` label line.

## [0.3.0] - 2026-06-26

### Added

- The selection prompt now accepts multi-select responses instead of only a single number: a hyphen range like `1-3` picks an inclusive span of changes, a comma list like `1,3` picks exactly the changes named, and the two combine in one response (`1-2,4`). A change named by more than one member (`1-3,2`) is synced once, not twice. Whitespace between members and around a range's bounds is ignored, so `1 - 2, 4` works the same as `1-2,4`. Members are bounded to the listed changes; a reversed, out-of-range, or otherwise malformed response is rejected like any other unrecognized entry.
- `csync push` and `csync pull` sync against a remote saved in a project-local `.csync.toml` (a `remote = "user@host:/path"` line), so a frequently-used remote no longer has to be retyped. Run them from the project directory: `push` sends the project to the saved remote, `pull` brings the saved remote down to the project.
- csync now holds its own `.csync.toml` out of every comparison — like `.git/`, it is never offered for transfer — and names it on the `Excluded:` line so the omission is visible.

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

[Unreleased]: https://github.com/dpassarelli/cherry-sync/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/dpassarelli/cherry-sync/compare/v0.2.0...0.2.1
[0.2.0]: https://github.com/dpassarelli/cherry-sync/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dpassarelli/cherry-sync/releases/tag/v0.1.0
