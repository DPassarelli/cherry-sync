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
