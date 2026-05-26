# Cherry-Sync: An Interactive Rsync Wrapper

## Problem

When working across a local machine and a remote dev environment over SSH, the workflow for moving files back and forth is clumsy. Plain `rsync` transfers everything or nothing. Plain `scp` has no incremental mode. What's missing is the middle step: see what's different, then pick what moves and in which direction.

This is a common situation for anyone working with SSH-accessible dev boxes — cloud instances, Proxmox containers, Raspberry Pis, WSL-to-host, etc.

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

## Key technical details

- `rsync --itemize-changes` produces per-file codes like `>f.st......` that encode what changed and in which direction. Parsing these into human-readable labels ("modified locally", "new on remote", "deleted") is the core translation layer.
- `rsync --files-from=<path>` accepts a text file of relative paths and transfers only those. This is how selective sync works without running rsync once per file.
- Bidirectional comparison requires two dry-run passes (one push, one pull) and merging the results. Conflicts (modified on both sides) need to be flagged clearly.

## Open questions to resolve early

### Implementation language

**Option A: Node.js CLI**
- Pro: The author (dpassarelli) has 30 years of JS experience. Rich TUI libraries (`inquirer`, `prompts`, `chalk`). Easy to publish via `npm`. Cross-platform.
- Con: Requires Node.js runtime on the local machine. Heavier dependency footprint.

**Option B: Shell script (bash/zsh)**
- Pro: Zero dependencies beyond rsync/ssh. Runs anywhere. Fast to prototype.
- Con: TUI interaction is painful in pure shell. Argument parsing, error handling, and cross-platform support get ugly fast. Hard to test.

**Option C: Go or Rust**
- Pro: Single static binary, no runtime. Fast. Good CLI libraries (cobra, clap).
- Con: Less familiar to the author. Slower iteration in early stages.

Recommendation: start with Node.js for speed of iteration and TUI quality, with the understanding that a compiled rewrite is possible later if the dependency bothers people.

### Scope for v0.1

A minimal first version that's useful immediately:

- Single direction per invocation (push or pull, specified by argument or inferred from args order like rsync does with `src dest`)
- Dry-run comparison with human-readable output
- Interactive file selection (checkbox-style multi-select)
- Execute transfer for selected files
- Respect `.gitignore` or a custom exclude file

## Development workflow

- Test-first. The itemize-changes parser is pure logic and very testable. Start there.
- The tool will be developed inside a Claude Code sandbox (Debian LXC container on Proxmox) and synced to/from a Mac workstation — meaning we'll be dogfooding immediately.

## Author context

- 30 years as a programmer, primarily full-stack web (JS/TS)
- Strong proponent of test-first development, feedback loops, experimentation
- Comfortable with technical depth but prefers problems broken into small pieces
- This project originated from a real workflow need while building a Proxmox home lab
