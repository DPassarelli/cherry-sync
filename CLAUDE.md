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

### Technical implementation and guidelines

- Let's use Go. I have no prior experience with this language, so I'll be relying heavily on you to help me scaffold the architecture and use proper idioms/patterns.
- Please help me learn as we go along. Explain your choices, and always remember to work in small pieces so that I can follow along.
- We want to maintain a test-driven development approach at all times. Tests should always be focused, and (if possible) follow the pattern described in the document `5 Questions Every Unit Test Must Answer.pdf`. I understand that document is written for JavaScript, and I understand that Go has an existing idiom for testing, but if we can incorporate any of the ideas from that document into our test template, I would greatly appreciate it.
- Please also try (where practical) to adhere to the guidelines documented at https://clig.dev. I understand some of them will be unnecessary, and I don't expect to have 100% implementation, but let's give at least some of them a try! We can create a document to track which ones we've included and update as we go along. 

### Scope for v0.1

A minimal first version that's useful immediately:

- Single direction per invocation (push or pull, specified by argument or inferred from args order like rsync does with `src dest`)
- Dry-run comparison with human-readable output
- Interactive file selection (checkbox-style multi-select)
- Execute transfer for selected files
- Respect `.gitignore` or a custom exclude file

## Author context

- 30 years as a programmer, primarily full-stack web (JS/TS)
- Strong proponent of test-first development, feedback loops, experimentation
- Comfortable with technical depth but prefers problems broken into small pieces
- This project originated from a real workflow need while building a Proxmox home lab
