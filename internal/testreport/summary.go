// summary.go holds the shapes a parsed test run is reported in: the per-package
// results, the counts rolled up across them, and the run metadata printed alongside.

package testreport

// Outcome holds the pass/fail/skip tally for one category of tests.
type Outcome struct {
	Passed  int
	Failed  int
	Skipped int
}

// total reports how many tests the Outcome accounts for.
func (o Outcome) total() int { return o.Passed + o.Failed + o.Skipped }

// PackageResult is a single package's result row for the per-package detail.
type PackageResult struct {
	Name    string
	Result  string // "pass", "fail", or "skip" (skip = no test files)
	Elapsed float64
}

// Summary is the parsed view of a test run: the scenario and unit tallies, the
// names of any failures, the per-package results, and the total test time.
type Summary struct {
	Scenarios Outcome
	Units     Outcome
	Failures  []string
	Packages  []PackageResult
	Seconds   float64
	NoTests   bool
}

// Meta carries the environment facts the script gathers (the test run itself
// can't know them) for the panel's heading and Environment block.
type Meta struct {
	Label  string
	Rsync  string
	Go     string
	Runner string
}
