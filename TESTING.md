# Testing

Cherry-Sync follows a test-first, outside-in development approach. Tests
describe behavior before code exists, and behavior — not the unit — is the
starting point. This document captures how we work: the loop, the style
conventions, the file layout, and the rationale behind each.

## Philosophy

- **Test-first.** Every behavior begins as a failing test. The test expresses
  what a user expects to see; production code is whatever it takes to satisfy
  that expectation.
- **Outside-in.** We start at the user-observable boundary — what command does
  the user run, what output do they see? — and drill inward only when a step
  needs supporting unit-level work. Unit tests are subordinate to behavior
  tests, not the other way around.
- **Gherkin as canonical spec.** The `.feature` files are the authoritative
  description of what csync does. They are written in Given/When/Then form and
  executed by godog. New behaviors enter the project as Gherkin scenarios
  before they enter as code.

## The development loop

One round of behavior, from idea to merged code:

1. **Discuss the behavior in plain English.** What does the user do? What do
   they observe? What's the smallest interesting example?
2. **Sketch a Gherkin scenario.** Use imperative `When`/`Then` with the exact
   command and the exact expected output. Concrete beats abstract.
3. **Stash deferred scenarios.** If the discussion surfaces related scenarios
   that aren't ready to drill into, add them as TODO comment blocks at the
   bottom of the relevant `.feature` file. They become the visible backlog.
4. **Wire step definitions.** Implement the scenario's steps in
   `features_test.go` so godog can run it. The test will fail — that's the
   point.
5. **Drill inward as needed.** If a step requires logic that benefits from
   focused unit coverage (parsing, state derivation, edge cases), write the
   unit test in the appropriate `internal/<pkg>/*_test.go` before writing the
   implementation.
6. **Make it pass.** Write the minimum production code to satisfy the failing
   tests. No speculative features, no unused fields.
7. **Refactor on green.** Rename, extract, simplify — but only with all tests
   passing. The suite is the safety net.
8. **Verify.** Run `go test ./...` and `gofmt -l .` before committing.

## Gherkin style

- **Imperative over declarative.** Prefer `When I run "csync ./project
  user@host:/project"` and `Then the reported source should be "./project"`
  over vague phrasing like "the user can see the planned actions." Specific
  assertions surface design decisions; vague ones erode them. Loosening a
  too-tight assertion later is easy; tightening a too-loose one means
  re-deriving what the design originally was.
- **Tables for world state.** When a scenario depends on the state of multiple
  files on each side, use a Gherkin table. Concrete state is easier to verify
  and easier to translate into fixtures.
- **One scenario, one behavior.** Don't bundle multiple expectations into a
  single scenario. If a related expectation matters, write a second scenario.
- **TODO blocks for unstamped scenarios.** When you identify a scenario but
  aren't ready to drill into it, leave it as a commented block at the bottom
  of the feature file. The feature file then doubles as a visible per-feature
  backlog.

## Unit test style

Tests must be readable line by line. The shape we use:

```go
func theReportedSourceShouldBe(ctx context.Context, expected string) error {
    raw := captured(ctx)
    actual := parseStdout(raw).Source

    if actual != expected {
        return fmt.Errorf("expected Source %q, got %q in output:\n%s", expected, actual, raw)
    }
    return nil
}
```

Conventions:

- **Named locals for the moving parts.** `raw`, `parsed`, `actual`, `expected`
  — each names a step in the test's logic. A reader can follow the test
  without reverse-engineering inline expressions.
- **Blank line separating act from assert.** A visual cue for arrange / act /
  assert.
- **No abstraction until it pays for itself.** For two similar assertions,
  inline duplication is preferable to a helper. Reach for a shared helper only
  when three or more assertions share the same shape.

## The output-parsing facade

Tests assert against csync's stdout, but they should not parse it inline in
every step. The `parseStdout` function in `output_parser_test.go` is the
single translation point between rendered output and structured test data.

- **Production emits text; tests parse it.** This keeps the boundary honest.
  Production code is free to render however reads best; test code is free to
  assert against semantic fields.
- **One file, one function.** `parseStdout(stdout string) ReportedOutput` is
  the entire surface. If rendering changes, this is the only place that
  changes.
- **`_test.go` location.** The parser is test-only scaffolding. Putting it in
  a `_test.go` file means it's compiled out of the production binary and the
  boundary stays enforced — production code can never call into the parser.
- **Grow the struct as scenarios drive it.** Only add a field to
  `ReportedOutput` when a scenario asserts against it. Pre-building fields for
  output that doesn't exist yet means the facade is lying about what's
  available.

## File organization

```
features/*.feature              Gherkin specs — the behavior catalog
features_test.go                godog wiring and step definitions
output_parser_test.go           parseStdout / ReportedOutput facade
internal/<pkg>/*_test.go        Unit tests next to the code they cover
```

Test-only helpers live in `_test.go` files at the repo root or alongside the
package they serve. They are never placed in `internal/` — production code
must not be able to import them.

## Running tests

| Command           | Purpose                                              |
|-------------------|------------------------------------------------------|
| `go test ./...`   | Run the full suite (godog scenarios + unit tests)    |
| `gofmt -l .`      | List files needing formatting (silent = all clean)   |
| `go vet ./...`    | Runs automatically in lefthook pre-push              |

godog's default pretty formatter prints feature-file line numbers on failure,
which makes it easy to navigate from a failing assertion back to the scenario
that triggered it.
