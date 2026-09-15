# Code style

The actionable style rules live in [CLAUDE.md](CLAUDE.md), so they're always in Claude's working context. **This document is the reasoning behind them** — the why, the scope, and worked examples — not a second copy of the rules. Read it to understand or challenge a rule; don't restate the rules here, or the two will drift. When a rule changes, the statement changes in CLAUDE.md; this file only changes when the *reasoning* does.

The rules cover what `gofmt` and `go vet` don't, and apply to all code in the repository, tests included. For *additional* test-specific conventions (Gherkin shape, `got`/`want` naming, the output-parsing facade), see [TESTING.md](TESTING.md).

The sections below pair with the CLAUDE.md rules of the same topic.

## Assignments inside `if` conditions

Go permits an init statement inside `if`, and it's idiomatic — the stdlib uses it heavily:

```go
if err := doSomething(); err != nil {
    return err
}
```

The CLAUDE.md rule forbids it in favor of the two-line form:

```go
err := doSomething()
if err != nil {
    return err
}
```

**Why:**

- Easier to skim — the assignment and the condition are visually distinct.
- Easier to step through in a debugger; easier to insert a log line between the call and the check.
- The assigned variable is in scope for the rest of the enclosing block — sometimes useful, never harmful when it isn't.

**Scope of the rule:**

- Applies to `if`.
- The comma-ok form keeps its init — `if _, ok := m[key]; !ok`, `if v, ok := x.(T); ok`, `if v, ok := <-ch; ok`. The second result exists only to be the condition, so there is no assignment to lift away from it; splitting the form leaves a dangling line and leaks a dead boolean into the block. This covers only map index, type assertion, and channel receive — an ordinary call returning `(T, bool)` follows the main rule.
- `for` loops keep `for i := 0; i < n; i++` — the init is part of the loop's defining shape.
- Type switches keep `switch v := x.(type)` — the binding is the point.
- Regular switches with init (`switch x := f(); x { ... }`) follow the same spirit; prefer two lines unless the switch is genuinely tighter that way.

## Doc comments: name first, then an active verb

The CLAUDE.md rule starts every doc comment with the identifier it documents — the Go convention, and what `go doc`, pkg.go.dev, and `staticcheck` (ST1020–ST1022) expect:

```go
// Run invokes rsync to compute the diff between source and destination.
func Run(...) { ... }
```

After the name, a present-tense active verb says what the thing does — "Run **invokes**", "Parse **turns**", "parseActions **walks**" — rather than vaguely restating the name.

**Scope of the rule:**

- Functions take a verb: "X **does** …", not "X is the function that …".
- Types and variables that name a thing rather than an action may use "X is a …" / "X represents …" / "X holds …" — a pure data struct has no action to describe, and forcing a verb reads worse. `Action is a single planned change` is correct as written.
- The name must come first either way; that part is non-negotiable because the tooling depends on it.

## Commenting every top-level declaration

The CLAUDE.md rule puts a doc comment on every package-level declaration — `type`, `var`, `const`, `func` — exported or not, in production code and tests alike. Yes, this means the occasional comment that mostly restates the name; we accept that cost in exchange for a uniform rule with no judgment call about which declarations "deserve" one.

A comment should still earn its place where it can — note a non-obvious contract, a side effect, or how this declaration differs from a similar one — rather than echoing the signature. But when the name truly says it all, a one-line comment that says the same thing is fine; it is not a reason to omit the comment.

This rule covers top-level declarations only. Comments *inside* a function body stay by judgment: add them where intent isn't obvious, skip them where the code speaks for itself.

## File header comments

Every production `.go` file opens with a comment stating that file's purpose, so a reader can tell what a file is for — and how a package's files divide the work — without reading the declarations. The CLAUDE.md rule makes this uniform; the Go mechanics decide *how*:

- **A package's doc comment lives on exactly one file.** A comment sitting directly above `package X` (no blank line) is the package doc. Put it on the package's primary/orchestrator file (`compare.go`, `selection.go`); `main` packages describe the command instead (`// Command csync …`), matching `go doc`'s convention for executables.
- **Every other file gets a file comment, separated from `package` by a blank line.** The blank line is what stops Go's tooling from reading it as a *second* package doc — multiple package comments in one package conflict (the tool picks one arbitrarily and `staticcheck` flags it). So:

  ```go
  // parse.go translates rsync's --itemize-changes output into Actions.

  package compare
  ```

  The leading `parse.go` is optional but reads well — it names the file the comment is scoped to, distinguishing it at a glance from the package doc.
- **Test files are exempt.** A `_test.go` file's purpose is given by its name (it tests the same-named production file) and the package it's in; a header there would be noise. This keeps the rule aimed where it earns its place — splitting a package's production surface into self-describing files.

The payoff shows up when a package grows past one file: `compare`'s `Run` orchestration, itemize `parse`, display `order`, and `gitignore` exclusion each announce themselves, so the package reads as four narrow concerns rather than one long file.

## Returning many values

Go lets a function return any number of values, but a long return list is a design smell. The CLAUDE.md rule caps it: three or more results bundle into a named struct, and a cleanup `func()` is never a return value.

**Why a struct past two values:**

- The call site is positional — `path, count, ok, cleanup, err := f()` — and nothing but order distinguishes the `string` from the `bool` from the `func()`. Reorder the returns and every caller silently binds the wrong names. A struct names each field at the call site (`exc.patterns`, `exc.inWorkTree`), so a reader doesn't reverse-engineer position, and adding a field doesn't churn every caller.
- A long signature is usually a function doing several jobs. Naming the bundle (`exclusions`) makes the seam explicit and often reveals that the jobs should split.

**Why never a cleanup `func()`:**

- Returning `cleanup func()` leaks a resource's lifecycle to the caller and depends on them remembering `defer cleanup()` — a forgotten defer is a leak the function could have prevented. Own the resource inside the function; if a resource genuinely must outlive the call, return a value with a `Close` method (the `io.Closer` convention) rather than a bare func, so the obligation is typed and discoverable.

**The worked example.** `compare`'s exclusion step used to be `excludeFile(source, destination) (string, int, bool, func(), error)` — five returns, one a temp-file cleanup func the caller had to defer. The func existed only because the patterns were written to a temp file for rsync's `--exclude-from`. Passing them as repeated `--exclude=` args instead removed the file (and the cleanup, and three I/O error paths), collapsing the signature to `localExclusions(source, destination) (exclusions, error)` — an idiomatic value-plus-error, the data named in a struct.

**Exceptions:**

- `(T, error)` — the everyday Go pattern; two values, always fine.
- comma-ok `(T, bool)` — `v, ok := m[k]`, `v, ok := x.(T)`; idiomatic and clear.
- A small, well-known positional tuple where order is self-evident — e.g. `strings.Cut`'s `(before, after, found string, bool)`. Three values, but the names are obvious from the operation; a struct would read worse. Reach for this sparingly: when in doubt, the struct is the safer default.

## File length

A file is the unit a reader opens. Past roughly 150 lines of code the eye stops holding the whole thing at once, and navigation shifts from structure to search — you stop knowing where something is and start grepping for it. The CLAUDE.md rule puts the threshold there and asks for a look, not a split.

**It is not a gate.** Nothing in CI counts lines, and no PR is blocked by the number. Crossing it is a prompt to ask "is there a seam here?" — sometimes the honest answer is no, and the file stays long. What the rule forbids is not a long file but an *unexamined* one.

**Test files are exempt**, for the same reason the file-header rule exempts them: length there is not the same signal.

- A table-driven test's cases are one behavior's enumeration. Splitting them across files hides the enumeration, which is the thing a reader most needs to see whole.
- A step-definition file is a vocabulary. `acceptance_tests` is the worked example: `features_test.go` reached 1,865 SLOC and genuinely needed splitting, but it needed it for *cohesion* — eleven unrelated concerns in one file — not because of a count. Splitting it to fit under 100 would have taken about nineteen files and would have separated step definitions from the helpers only they use. It split into eleven, the largest at 283, and that is the right answer.

The lesson generalizes: the number starts the conversation, the seam ends it. A split that serves the number and not the reader is the failure this rule is trying to prevent.

## Duplicated files

A file should exist once. Two copies mean two things to update and a silent bug the day someone updates one.

**The exemption is a tool constraint, not a convenience.** `go:embed` cannot reach a parent directory, so `internal/license` cannot embed the repository-root `LICENSE` directly. A byte-identical copy therefore lives beside `license.go`. There is no way to express this in Go without the copy.

**A forced copy carries two obligations:**

- **A test that fails on drift.** `license_test.go` compares the embedded text against the root file and fails if they diverge, so the root `LICENSE` stays the single source of truth and the copy cannot quietly rot.
- **A comment naming the constraint.** The package doc on `internal/license` says why the copy exists and which file is canonical. Without it, the next reader sees redundancy and deletes the wrong one.

**Why not the alternatives.** A symlink breaks on Windows checkouts and confuses GitHub's license detection. Generating the copy at build time adds a step to every build and the copy still has to exist on disk at compile time — the duplication moves, it does not go away. The committed copy plus a drift test is the cheapest arrangement that keeps the invariant provable.
