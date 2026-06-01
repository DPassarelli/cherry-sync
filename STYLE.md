# Code style

Project-wide conventions that aren't enforced by `gofmt` or `go vet`. We track
them here so they survive across sessions and reviewers. Style rules join the
list as they come up; nothing here is meant to be exhaustive.

This document is authoritative for **all** code in the repository, tests
included. A rule here applies to test files exactly as it applies to production
code unless it explicitly says otherwise.

For *additional* test-specific style (Gherkin shape, `got`/`want` naming, the
output-parsing facade), see [TESTING.md](TESTING.md). Those conventions extend
the rules here; they never override them.

## Avoid assignments inside `if` conditions

Go permits an init statement inside `if`:

```go
if err := doSomething(); err != nil {
    return err
}
```

This is idiomatic Go and the stdlib uses it heavily — but we don't. Prefer:

```go
err := doSomething()
if err != nil {
    return err
}
```

**Why:**

- Easier to skim — the assignment and the condition are visually distinct.
- Easier to step through in a debugger; easier to insert a log line between
  the call and the check.
- The assigned variable is in scope for the rest of the enclosing block —
  sometimes useful, never harmful when it isn't.

**Scope of the rule:**

- Applies to `if`.
- `for` loops keep `for i := 0; i < n; i++` — the init is part of the loop's
  defining shape.
- Type switches keep `switch v := x.(type)` — the binding is the point.
- Regular switches with init (`switch x := f(); x { ... }`) follow the same
  spirit; prefer two lines unless the switch is genuinely tighter that way.

## Doc comments start with the name, then an active verb

Begin every doc comment with the identifier it documents — the Go convention,
and what `go doc`, pkg.go.dev, and `staticcheck` (ST1020–ST1022) expect:

```go
// Run invokes rsync to compute the diff between source and destination.
func Run(...) { ... }
```

After the name, prefer a present-tense active verb that says what the thing
does — "Run **invokes**", "Parse **turns**", "parseActions **walks**". Avoid a
vague restatement of the name.

**Scope of the rule:**

- Functions take a verb: "X **does** …", not "X is the function that …".
- Types and variables that name a thing rather than an action may use
  "X is a …" / "X represents …" / "X holds …" — a pure data struct has no
  action to describe, and forcing a verb reads worse. `Action is a single
  planned change` is correct as written.
- The name must come first either way; that part is non-negotiable because the
  tooling depends on it.

## Comment every top-level declaration

Every package-level declaration — `type`, `var`, `const`, `func` — carries a
doc comment, exported or not, in production code and tests alike. Yes, this
means the occasional comment that mostly restates the name; we accept that
cost in exchange for a uniform rule with no judgment call about which
declarations "deserve" one.

A comment should still earn its place where it can — note a non-obvious
contract, a side effect, or how this declaration differs from a similar one —
rather than echoing the signature. But when the name truly says it all, a
one-line comment that says the same thing is fine; it is not a reason to omit
the comment.

This rule covers top-level declarations only. Comments *inside* a function
body stay by judgment: add them where intent isn't obvious, skip them where
the code speaks for itself.
