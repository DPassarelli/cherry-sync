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

See [TESTING.md](TESTING.md) for the philosophy, the development loop in full, the Gherkin/unit/facade conventions, and how to run the suite — the reasoning and detail behind the rules below. The non-negotiables are kept here, not duplicated there, because they constrain every change:

- **Test-first.** Every behavior begins as a failing test; write it and watch it fail before writing production code.
- **Enumerate the conditions** that change a behavior's outcome before writing the test — an unlisted condition is an undiscovered behavior or a latent bug.
- **Prove the test has teeth.** Confirm it fails for the right reason before making it pass: mutate the production code to the degenerate version and check that this test — and only it — goes red.
- **Verify external-tool behavior by experiment, not memory.** Check what `rsync`/`ssh` actually emit before encoding it in a test or parser.
- **Agree the scenario before wiring it.** Gherkin scenarios are the spec; review them before writing step definitions or production code.
- **One scenario, one behavior.** Don't bundle expectations into a single scenario.
- **Unit-test logic, not command assembly.** A unit test pins a decision the code makes — parsing, ordering, classification. Never assert the argument vector handed to an external tool (that a specific flag or path is in the slice passed to `rsync`/`ssh`): the outside-in scenario that runs the real command already gates that, and a slice assertion only breaks on a correct refactor. If a unit test's failure couldn't mean a user-visible behavior changed, delete it.
- **Green before refactor; verify before done.** Minimal code to pass, refactor only on green, and run `go test ./...` and `gofmt -l .` before calling it done.

## Style

See [STYLE.md](STYLE.md) for the rationale, scope, and worked examples behind each rule — the "why." The rules themselves are kept here, not duplicated there, because they apply to every change. They cover what `gofmt` and `go vet` don't, and hold for all code including tests:

- **No assignment inside an `if` condition.** Lift the init statement onto its own line above the `if`. (`for` loops and type switches keep their init — see STYLE.md for the exact scope.)
- **Doc-comment every top-level declaration** — `type`, `var`, `const`, `func`, exported or not.
- **Start each doc comment with the identifier name, then an active verb** (`Run invokes…`); data types may use "X is a…" / "holds…" instead.
- **Head every production `.go` file with a purpose comment.** Exactly one file per package carries the `// Package X …` doc (directly above `package`); every other file opens with a blank-line-separated comment naming what that file holds. `main` packages use `// Command <name> …`. Test files are exempt. (See STYLE.md for the Go mechanics.)
- **Don't return long value lists.** A function returning three or more results should bundle them into a named struct (with `error` still returned alongside). Never return a cleanup `func()` for the caller to defer — own the resource's lifetime inside the function. The idiomatic `(T, error)` and comma-ok `(T, bool)` pairs are fine. (See STYLE.md.)

## Security

See [SECURITY.md](SECURITY.md) for the threat model, trust boundaries, and the catalog of concerns. That document is the source of truth on security. The non-negotiable invariants, kept here because they constrain every change:

- **No shell, ever.** Invoke external commands with `exec.Command` and an argument slice — never `sh -c`. Shell metacharacters in a path must reach `rsync` as inert literal bytes.
- **`--` before positional paths handed to rsync**, so a path beginning with `-` can't be parsed as an rsync option (`-e`/`--rsh` = remote-shell execution).
- **Validate every path operand** before use — at minimum reject empty strings (an empty path becomes `/`).
- **NUL-delimit (`--from0`) any `--files-from` list** once the transfer phase exists; newlines in filenames otherwise smuggle extra entries.

## Changelog & documentation

The release process rolls `CHANGELOG.md`'s `[Unreleased]` section into the published GitHub Release notes, so that section is the single source of truth for what shipped and must always be current:

- **Log user-visible changes in the same change that makes them.** Every PR that adds, changes, fixes, or removes something a user would notice gets a Keep a Changelog entry (Added/Changed/Fixed/Removed) under `[Unreleased]` — written then, not reconstructed at release time. An empty `[Unreleased]` must mean nothing user-facing landed, never that the log was skipped.
- **Internal-only changes don't need an entry.** Refactors, test-harness, and CI work that change no observable behavior stay out of the changelog.
- **CHANGELOG owns version history; README must not duplicate it.** Keep per-version status banners, "done" checklists, and hardcoded version numbers out of `README.md`. README describes what the tool is and how to install and use it — content that doesn't go stale every release. Anything of the form "shipped in vX" lives only in CHANGELOG.
- **This file holds durable rules, not plans.** What shipped lives in `CHANGELOG.md`; what's planned lives in README's Roadmap. Don't add a per-version scope or status section here — it goes stale the moment a release ships (the original "Scope for v0.1" did exactly that).
- **Don't reference gitignored / uncommitted files from committed files.** Code comments, feature files, and committed docs must not cite paths that aren't in the repo (anything matched by `.gitignore` — e.g. the local scratchpad, the PDF reference docs). A clone won't have them, so the reference is a dead pointer that leaks a private filename. State the reasoning inline instead, or cite a committed doc. (CLAUDE.md, README, CHANGELOG, STYLE/TESTING/SECURITY are committed and fair to cite.)
- **Don't hard-wrap prose in Markdown.** In committed Markdown — README, CHANGELOG, CLAUDE/TESTING/STYLE/SECURITY, and the prose inside feature files — write each paragraph and list item as one logical line and let the editor soft-wrap; don't insert hard newlines to wrap at ~80 columns, since those breaks render as unwanted line breaks in some viewers. Prose only: keep the deliberate line structure of tables, code blocks, and list-item boundaries.
