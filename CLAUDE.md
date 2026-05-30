# Cherry-Sync: An Interactive Rsync Wrapper

## Problem

When working across a local machine and a remote dev environment over SSH, the workflow for moving files back and forth is clumsy. Plain `rsync` transfers everything or nothing. Plain `scp` has no incremental mode. What's missing is the middle step: see what's different, then pick what moves and in which direction.

This is a common situation for anyone working with SSH-accessible dev boxes — cloud instances, Proxmox containers, Raspberry Pis, WSL-to-host, etc. This tool originated from a need to improve an AI-assisted development workflow, where Claude runs inside a sandbox isolated from the primary computer.

## What this tool does

An interactive CLI that wraps `rsync` to provide a select-then-sync workflow:

1. Compare local and remote directories (using `rsync --dry-run --itemize-changes`)
2. Show the user a human-readable list of differences (new, modified, deleted — with direction)
3. Let the user choose: sync all, or select individual files
4. Execute the transfer for only the selected files (using `rsync --files-from`)

## Design principles

- **rsync does the heavy lifting.** This tool is a UX layer, not a reimplementation. It parses rsync's output and drives rsync's `--files-from` for selective transfer.
- **SSH is the transport.** No additional daemon or agent on the remote side. If you can `ssh` to it, this tool works.
- **No opinion on direction.** Push and pull are both first-class. Bidirectional diff display (showing which side is newer) is a goal.
- **Minimal dependencies.** `rsync` and `ssh` must be present on both sides. The tool itself should be easy to install.
- **CLI conventions.** Where practical, follows the guidelines at [clig.dev](https://clig.dev) for command-line UX. Adherence is intentional but not exhaustive.

## Key technical details

- `rsync --itemize-changes` produces per-file codes like `>f.st......` that encode what changed and in which direction. Parsing these into human-readable labels ("modified locally", "new on remote", "deleted") is the core translation layer.
- `rsync --files-from=<path>` accepts a text file of relative paths and transfers only those. This is how selective sync works without running rsync once per file.
- Bidirectional comparison requires two dry-run passes (one push, one pull) and merging the results. Conflicts (modified on both sides) need to be flagged clearly.

## Testing

See [TESTING.md](TESTING.md) for the development loop, Gherkin and unit test style conventions, the output-parsing facade, and how to run the suite. That document is the source of truth on testing.

## Style

See [STYLE.md](STYLE.md) for code style rules that aren't enforced by `gofmt` or `go vet`. That document is the source of truth on style.

## Security

See [SECURITY.md](SECURITY.md) for the threat model, trust boundaries, and the catalog of concerns. That document is the source of truth on security. The non-negotiable invariants, kept here because they constrain every change:

- **No shell, ever.** Invoke external commands with `exec.Command` and an argument slice — never `sh -c`. Shell metacharacters in a path must reach `rsync` as inert literal bytes.
- **`--` before positional paths handed to rsync**, so a path beginning with `-` can't be parsed as an rsync option (`-e`/`--rsh` = remote-shell execution).
- **Validate every path operand** before use — at minimum reject empty strings (an empty path becomes `/`).
- **NUL-delimit (`--from0`) any `--files-from` list** once the transfer phase exists; newlines in filenames otherwise smuggle extra entries.

## Scope for v0.1

A minimal first version that's useful immediately:

- Single direction per invocation (push or pull, specified by argument or inferred from args order like rsync does with `src dest`)
- Dry-run comparison with human-readable output
- Interactive file selection (checkbox-style multi-select)
- Execute transfer for selected files
- Respect `.gitignore` or a custom exclude file
