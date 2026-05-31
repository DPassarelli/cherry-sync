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
- **Behavior is defined by its conditions.** A behavior isn't understood until
  you can list the conditions that change its outcome — inputs, directions,
  orderings, failure modes, edge values. An unlisted condition is an
  undiscovered behavior or a latent bug. Enumerating conditions is mandatory;
  the depth scales with uncertainty, and sometimes the honest answer is "only
  one case" — that's fine, but you have to have asked.
- **Gherkin as canonical spec.** The `.feature` files are the authoritative
  description of what csync does. They are written in Given/When/Then form and
  executed by godog. New behaviors enter the project as Gherkin scenarios
  before they enter as code.

## The development loop

One round of behavior, from idea to merged code:

1. **Discuss the behavior, then map its conditions.** State it in plain English
   — what the user does, what they observe. Then enumerate the conditions that
   change its outcome and brainstorm them widely; the smallest interesting
   example is a starting point, not the finish line. Verify any assumption
   about how an external tool (`rsync`, `ssh`) actually behaves by experiment,
   not memory, before encoding it in a scenario.
2. **Turn the conditions into scenarios.** Each condition with a distinct
   observable outcome becomes a candidate scenario, written imperative and
   concrete (exact command, exact expected output). Conditions you won't cover
   yet become TODO comment blocks at the bottom of the `.feature` file — the
   visible backlog, chosen deliberately from the brainstorm rather than left
   out by oversight.
3. **Review the scenarios before writing any code.** Stop here. Read the
   scenarios back — with the user, and against the design — and confirm they
   say what's intended, in the right Given/When/Then shape, with the right
   expected values. This is a gate, not a courtesy: the scenarios are the spec,
   and revising text is far cheaper than reworking step definitions and
   production code. Do not wire steps until the scenarios are agreed.
4. **Wire step definitions.** Implement the scenario's steps in
   `features_test.go` so godog can run it. The test will fail — that's the
   point.
5. **Drill inward as needed.** If a step requires logic that benefits from
   focused unit coverage (parsing, ordering, state derivation, edge cases),
   write the unit test in the appropriate `internal/<pkg>/*_test.go` before
   writing the implementation. Unit tests pin individual conditions one at a
   time; the scenario already covers the behavior holistically, so don't
   re-assert the whole scenario at the unit level — isolate the one rule or
   edge case each test exists to prove.
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

The project-wide rules in [STYLE.md](STYLE.md) apply to test code too; the
conventions below are additions specific to tests, not replacements.

Tests must be readable line by line. The shape we use:

```go
func theReportedSourceShouldBe(ctx context.Context, want string) error {
    r := captured(ctx)
    got := parseOutput(r.Stdout, r.Stderr).Source

    if got != want {
        return fmt.Errorf("Source: got %q, want %q in output:\n%s", got, want, r.Stdout)
    }
    return nil
}
```

Conventions:

- **`got` and `want` are the standard names.** Use them in unit tests and in
  step definitions alike (`want` becomes the step's parameter name; `got` is
  the locally-extracted value under test). This is the Go stdlib testing
  idiom — short, symmetric, instantly recognizable to a Go reader. Error
  messages follow the same idiom: `"X: got %v, want %v"`.
- **Name any intermediate values, too.** In the example, `r` holds the
  captured `runResult` from the previous `When` step. Each named local is a
  step in the test's logic; a reader can follow without reverse-engineering
  inline expressions.
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
| `gosec ./...`     | Static security analysis; runs in lefthook pre-push  |

godog's default pretty formatter prints feature-file line numbers on failure,
which makes it easy to navigate from a failing assertion back to the scenario
that triggered it.
