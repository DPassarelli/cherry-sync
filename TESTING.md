# Testing

Cherry-Sync follows a test-first, outside-in development approach. Tests describe behavior before code exists, and behavior — not the unit — is the starting point. This document captures how we work: the loop, the style conventions, the file layout, and the rationale behind each.

The condensed **non-negotiables** live in [CLAUDE.md](CLAUDE.md), so they're always in Claude's working context. This document is the depth behind them — the why and the how — and the home for the detailed conventions (Gherkin shape, `got`/`want`, the facade) that don't fit a checklist. The terse rule statements belong in CLAUDE.md; keep them out of here so the two don't drift.

## Philosophy

- **Test-first.** Every behavior begins as a failing test. The test expresses what a user expects to see; production code is whatever it takes to satisfy that expectation.
- **Outside-in.** We start at the user-observable boundary — what command does the user run, what output do they see? — and drill inward only when a step needs supporting unit-level work. Unit tests are subordinate to behavior tests, not the other way around.
- **Behavior is defined by its conditions.** A behavior isn't understood until you can list the conditions that change its outcome — inputs, directions, orderings, failure modes, edge values. An unlisted condition is an undiscovered behavior or a latent bug. Enumerating conditions is mandatory; the depth scales with uncertainty, and sometimes the honest answer is "only one case" — that's fine, but you have to have asked.
- **Triangulate parameterized rules.** When a behavior is parameterized — select the *N*th change, transfer *K* files, retry *M* times — a single example can be satisfied by a constant that never reads the parameter: `respond with "1"` passes even against code hardcoded to "always the first". *Triangulation* is adding a second example whose expected outcome *differs* (`respond with "2"` → the second change), which forces the parameter to actually drive the result. Two discriminating data points pin a non-degenerate mapping; one does not. To confirm such a test has teeth, briefly mutate the production code to the degenerate constant and check that the new scenario — and only it — goes red.
- **Gherkin as canonical spec.** The `.feature` files are the authoritative description of what csync does. They are written in Given/When/Then form and executed by godog. New behaviors enter the project as Gherkin scenarios before they enter as code.

## The development loop

One round of behavior, from idea to merged code:

1. **Discuss the behavior, then map its conditions.** State it in plain English — what the user does, what they observe. Then enumerate the conditions that change its outcome and brainstorm them widely; the smallest interesting example is a starting point, not the finish line. Verify any assumption about how an external tool (`rsync`, `ssh`) actually behaves by experiment, not memory, before encoding it in a scenario.
2. **Turn the conditions into scenarios.** Each condition with a distinct observable outcome becomes a candidate scenario, written imperative and concrete (exact command, exact expected output). Conditions you won't cover yet become TODO comment blocks at the bottom of the `.feature` file — the visible backlog, chosen deliberately from the brainstorm rather than left out by oversight.
3. **Review the scenarios before writing any code.** Stop here. Read the scenarios back — with the user, and against the design — and confirm they say what's intended, in the right Given/When/Then shape, with the right expected values. This is a gate, not a courtesy: the scenarios are the spec, and revising text is far cheaper than reworking step definitions and production code. Do not wire steps until the scenarios are agreed.
4. **Wire step definitions.** Implement the scenario's steps in `acceptance_tests/features_test.go` so godog can run it. The test will fail — that's the point.
5. **Drill inward as needed.** If a step requires logic that benefits from focused unit coverage (parsing, ordering, state derivation, edge cases), write the unit test in the appropriate `internal/<pkg>/*_test.go` before writing the implementation. Unit tests pin individual conditions one at a time; the scenario already covers the behavior holistically, so don't re-assert the whole scenario at the unit level — isolate the one rule or edge case each test exists to prove.

   **A unit test pins logic, not command assembly.** The thing worth a focused test is a *decision the code makes* — how a line parses, how paths order, how a code classifies. The argument vector handed to an external tool is not such a decision: a test asserting "`-8` is in the slice passed to `rsync`" or "`--` precedes the operands" pins the implementation's shape, not an observable behavior. It's brittle (a correct refactor — `-8` → `--8-bit-output`, reordering flags — turns it red while the tool behaves identically) and redundant (the outside-in scenario that *runs* the real command is what proves the flag does its job: the `café.txt` round-trip proves `-8`; the "treated as a path" scenario proves `--`). A useful litmus: if a unit test could go red without any user-visible behavior changing, it's testing the wrong thing — delete it and lean on the scenario. Two such `rsyncArgs` tests were written and later removed for exactly this reason.
6. **Make it pass.** Write the minimum production code to satisfy the failing tests. No speculative features, no unused fields.
7. **Refactor on green.** Rename, extract, simplify — but only with all tests passing. The suite is the safety net.
8. **Verify.** Run `go test -count=1 ./...` and `gofmt -l .` before committing. The `-count=1` is not optional after a production change — see [Why `-count=1` matters](#running-tests) below for why a plain `go test ./...` can report a stale pass.

## Gherkin style

- **Imperative over declarative.** Prefer `When I run "csync ./project user@host:/project"` and `Then the reported source should be "./project"` over vague phrasing like "the user can see the planned actions." Specific assertions surface design decisions; vague ones erode them. Loosening a too-tight assertion later is easy; tightening a too-loose one means re-deriving what the design originally was.
- **Tables for world state.** When a scenario depends on the state of multiple files on each side, use a Gherkin table. Concrete state is easier to verify and easier to translate into fixtures.
- **One scenario, one behavior.** Don't bundle multiple expectations into a single scenario. If a related expectation matters, write a second scenario.
- **TODO blocks for unstamped scenarios.** When you identify a scenario but aren't ready to drill into it, leave it as a commented block at the bottom of the feature file. The feature file then doubles as a visible per-feature backlog.

## Unit test style

The project-wide rules in [STYLE.md](STYLE.md) apply to test code too; the conventions below are additions specific to tests, not replacements.

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

- **`got` and `want` are the standard names.** Use them in unit tests and in step definitions alike (`want` becomes the step's parameter name; `got` is the locally-extracted value under test). This is the Go stdlib testing idiom — short, symmetric, instantly recognizable to a Go reader. Error messages follow the same idiom: `"X: got %v, want %v"`.
- **Name any intermediate values, too.** In the example, `r` holds the captured `runResult` from the previous `When` step. Each named local is a step in the test's logic; a reader can follow without reverse-engineering inline expressions.
- **Blank line separating act from assert.** A visual cue for arrange / act / assert.
- **No abstraction until it pays for itself.** For two similar assertions, inline duplication is preferable to a helper. Reach for a shared helper only when three or more assertions share the same shape.
- **Describe contracts in prose; don't paste signatures.** This document (and docs generally) captures intent and invariants; the code stays the source of truth for mechanics. Name stable anchors — a function, type, or file — so a reader can find the code, but describe what it takes and returns in words rather than quoting a literal signature. Parameter lists and return types drift on every incidental change; a prose contract doesn't. The exception is when the literal form *is* the subject (e.g. a syntax rule), where a code example is the point.
- **`Test*` functions describe the behavior, not the name.** This is the one sanctioned exception to STYLE.md's "doc comments start with the identifier name" rule. A test function's name is already a descriptive sentence (`TestComparePaths_NumbersCompareByValue`), so leading the comment with it again is pure echo. Instead, lead with what the test pins down, using a `Behavior:` prefix for godog-mirroring unit tests and a `Rule N:` prefix when isolating one rule of a larger behavior:

  ```go
  // Behavior: a single `>f...` update line yields a single update Action.
  func TestParseActions_OneUpdate_ReturnsOneUpdateAction(t *testing.T) { ... }

  // Rule 4: numbers compare by value, not lexically.
  func TestComparePaths_NumbersCompareByValue(t *testing.T) { ... }
  ```

  The exception is scoped to `Test*` functions only. Test *helpers* and *step definitions* (e.g. `assertSortsBefore`, `iRun`) are ordinary functions and follow the name-first rule like any other code.

## The output-parsing facade

Tests assert against csync's output, but they should not parse it inline in every step. The output parser in `acceptance_tests/output_parser_test.go` (`parseOutput`) is the single translation point between rendered output and structured test data.

- **Production emits text; tests parse it.** This keeps the boundary honest. Production code is free to render however reads best; test code is free to assert against semantic fields.
- **One file, one function.** A single function is the entire surface: it takes csync's captured output and returns a `ReportedOutput`. If rendering changes, this is the only place that changes.
- **`_test.go` location.** The parser is test-only scaffolding. Putting it in a `_test.go` file means it's compiled out of the production binary and the boundary stays enforced — production code can never call into the parser.
- **Grow the struct as scenarios drive it.** Only add a field to `ReportedOutput` when a scenario asserts against it. Pre-building fields for output that doesn't exist yet means the facade is lying about what's available.

## File organization

```
acceptance_tests/features/*.feature   Gherkin specs — the behavior catalog
acceptance_tests/features_test.go     godog wiring and step definitions
acceptance_tests/output_parser_test.go parseOutput / ReportedOutput facade
internal/<pkg>/*_test.go              Unit tests next to the code they cover
```

The black-box acceptance suite (godog wiring, step definitions, and the output-parsing facade) lives together under `acceptance_tests/`, keeping the repository root free of source files. Its scenarios and the specs they run sit side by side. Unit tests live in `_test.go` files alongside the package they serve. Neither is ever placed in `internal/` in a way that lets production code import it.

## Running tests

| Command                     | Purpose                                                              |
|-----------------------------|---------------------------------------------------------------------|
| `go test -count=1 ./...`    | Run the full suite (godog scenarios + unit tests); `-count=1` defeats stale caching — see below |
| `./_scripts/test-report.sh`  | Run the suite and print the same dashboard panel CI publishes (needs [`gotestsum`](https://github.com/gotestyourself/gotestsum); see below) |
| `gofmt -l .`                | List files needing formatting (silent = all clean)                  |
| `go vet ./...`              | Runs automatically in lefthook pre-push                             |
| `gosec ./...`               | Static security analysis; runs in lefthook pre-push                |

`_scripts/test-report.sh` is the single source of the dashboard panel shown in the GitHub Actions job summary, so running it locally previews exactly what CI will show: the environment (which rsync implementation, Go, host), a Gherkin-scenario-vs-unit-test breakdown, any failures by name, and per-package detail. It streams gotestsum's live output and then prints the panel as raw Markdown (pipe to a renderer such as `glow` if you want it formatted). The panel is rendered by `cmd/testreport` from gotestsum's `--jsonfile`; that summarizing logic lives in `internal/testreport` and is unit-tested like any other code. The script needs `gotestsum` on `PATH` (or installed in your Go bin dir); install the version CI pins with `go install gotest.tools/gotestsum@v1.13.0`. Extra arguments are forwarded to `go test` (e.g. `./_scripts/test-report.sh -run TestParse`).

godog's default pretty formatter prints feature-file line numbers on failure, which makes it easy to navigate from a failing assertion back to the scenario that triggered it.

**Why `-count=1` matters.** The godog suite builds the `csync` binary in `TestMain` and drives it by exec — the test package imports none of the production packages. Go's test cache keys a package's result on that package's own sources and its imported packages, so **no production-code change invalidates the godog cache**: edit `cmd/csync`, `internal/compare`, `internal/selection`, or anything else the binary uses, and a plain `go test ./...` can still print a green `ok (cached)` from a stale build — only changes to the test files themselves bust it. This bites because the suite execs a binary rather than calling production code in-process, which is otherwise the right design (it tests the real artifact a user runs). Pass `-count=1` (or run `go clean -testcache`) to force a real run whenever you've touched production code; the suite is fast enough that `-count=1` is a fine everyday default.
