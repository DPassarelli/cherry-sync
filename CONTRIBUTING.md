# Contributing: pull requests

The actionable rules live in [CLAUDE.md](CLAUDE.md) (the "Pull requests" section), so they stay in Claude's working context. **This document is the reasoning and the worked examples** — read it to understand or challenge a rule; don't restate the rules here, or the two will drift.

## General approach

**All** modifications to code within this project must be made through a pull request. There are no exceptions to this rule, not even for minor documentation changes. The PR workflow contains critical safety checks, which must never be by-passed.

Pull requests exist to help others understand _what_ has been changed and _why_. These descriptions must be succinct but also friendly and easy to grasp. Technical details should only be included when necessary to appreciate one or more significant aspects of _what_ or _why_.

Pull requests should also be as tightly focused in scope as possible. A single PR should not address multiple issues that are only orthogonally related. Ideally, each PR will be for a single issue; the only exceptions for this would be:

* additional issues that are related because of a common underlying problem or architectural decision
* additional issues that would be rendered moot once this PR is accepted

## Titles

[Conventional Commits](https://www.conventionalcommits.org/): `type(scope): summary`, imperative mood, lowercase, no trailing period. The types in use here are: `feat`, `fix`, `refactor`, `docs`, `test`, `ci`, and `chore`. The scope is optional and names the area it touches (`feat(select): …`).

## Bodies

Every PR gets a body — yes, even a rename or a folder move. A change that seems too trivial to describe is usually still hiding a *why* worth one sentence (why now, why this way). Include this information:

* What has been changed or fixed
* Why it was done (briefly)
* How it was done (only if critical to understanding some aspect of the above bullets)

Prose, not headings. Use bullets only when something is genuinely a list.

Do **not**:

- restate the diff line by line, or list the files touched — the diff is right there;
- paste test output or add a `## Testing` section — test-first is a standing invariant (see [TESTING.md](TESTING.md)); mention verification only as a plain sentence when it's part of the *why* (*"verified against rsync 3.4.1, whose protected-args default…"*);
- scaffold the body with `## Summary` / `## Changes` / `## Testing` headings — that ceremony is heavier than anything this repo needs and buries the reasoning under boilerplate.

## Good examples

### Bug fix

```
fix: normalize remote-path operands

A .csync.toml remote written with a `~` home shortcut (for example, `remote = "user@host:~/working"`) listed changes but then failed to transfer any (`rsync` exited with code 12). This is because, by default, `rsync` passes a leading `~` through literally. Therefore, it resolved to `/home/user/~/working`, which does not exist. Leading tildes are now trimmed from the operand to avoid this problem (rsync interprets relative paths in relation to the CWD).

In addition to trimming leading tildes, csync also normalizes the presence of trailing path separators. This is to ensure expected behavior, regardless of whether someone remembers to include them in the operand.

Closes #50
Closes #51
```

### Refactor (no behavior change)

```
refactor: move acceptance tests into sub-folder

Relocate the black-box tests and Gherkin specs under a single acceptance_tests/ directory, to avoid having any `.go` files in the project root.

Adjust the harness for the new location, and fix a coupling the move exposes: `internal/testreport` inferred the module root as the shortest package path seen, which broke once the root package lost its tests. Derive the root from the longest common package prefix instead.

No behavior change.

Closes #68
```

### Anti-patterns

#### Too much information

In the following example, several things should be trimmed:

1. the file-tree listing should be dropped entirely (the good "Refactor" sample above shows none)
2. the *how* of adjusting the harness (`build csync via its module path…`, `point godog at features/`) should be compressed to a brief mention; the good sample keeps only "Adjust the harness for the new location"
3. the trailing list of updated docs (TESTING, README, SMOKE-TEST) should be dropped
4. the coupling explanation ("…garbled every per-package label") should be compressed to a clause, as the good sample does with "which broke once the root package lost its tests"

````
Relocate the black-box test files and the Gherkin specs under a single acceptance_tests/ directory, leaving no .go files in the module root:

```
acceptance_tests/
    features_test.go        (package acceptance_test)
    output_parser_test.go
    features/               (was _features/)
```

Adjust the harness for the new location: build csync via its module path rather than ./cmd/csync, and point godog at features/.

Fix a coupling this exposes: internal/testreport inferred the module root as the shortest package path seen, which was the bare root package only because root had tests. With the suite moved out, that heuristic picked a subpackage and garbled every per-package label. Derive the root from the longest common package prefix instead — correct whether or not root has tests.

No behavior change; docs (TESTING, README, SMOKE-TEST) updated to the new paths.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
````

Note how this compares to the good "Refactor" sample shown above.

#### Too much structure

In this example:

1. we shouldn't restate the diff, list files, or report routine test runs
2. it doesn't say *why* `.git` must float
3. bullets only, no prose

```
## Summary
Excludes nested .git.

## Changes
- Changed /.git/ to .git in gitignore.go
- Updated a comment in compare.go
- Added an acceptance scenario

## Testing
- Ran go test ./... — all green
- gofmt clean
```

#### Habits to avoid

**In a PR description, do not include parenthetical asides set off by emdashes.** Put them inside parentheses or commas instead, or drop them if not really necessary.

## Footers

- `Closes #N` for each issue the change resolves, one per line.
- The `Co-Authored-By:` line.
