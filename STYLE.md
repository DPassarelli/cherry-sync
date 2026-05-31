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
