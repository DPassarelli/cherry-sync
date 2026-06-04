package testreport

import (
	"strings"
	"testing"
)

// sampleStream is a hand-built go test -json stream exercising every
// classification branch: a plain scenario, a scenario whose title contains a
// slash, a skipped scenario, a failed scenario, the TestFeatures harness parent
// (must not be counted), a unit test with table-driven subtests (the subtests
// must not inflate the unit count), a failed unit test, package-level pass/skip
// rows, and non-finished noise lines that must be ignored.
const sampleStream = `
{"Action":"run","Package":"m","Test":"TestFeatures"}
{"Action":"output","Package":"m","Test":"TestFeatures","Output":"=== RUN\n"}
{"Action":"pass","Package":"m","Test":"TestFeatures/Alpha_scenario"}
{"Action":"pass","Package":"m","Test":"TestFeatures/Ignored_via_.git/info/exclude"}
{"Action":"skip","Package":"m","Test":"TestFeatures/A_work_in_progress"}
{"Action":"fail","Package":"m","Test":"TestFeatures/Broken_behaviour"}
{"Action":"fail","Package":"m","Test":"TestFeatures"}
{"Action":"pass","Package":"m/internal/cli","Test":"TestParse"}
{"Action":"pass","Package":"m/internal/cli","Test":"TestParse/empty_source"}
{"Action":"pass","Package":"m/internal/cli","Test":"TestParse/empty_dest"}
{"Action":"fail","Package":"m/internal/compare","Test":"TestRsyncArgs"}
{"Action":"fail","Package":"m","Elapsed":1.5}
{"Action":"pass","Package":"m/internal/cli","Elapsed":0.2}
{"Action":"fail","Package":"m/internal/compare","Elapsed":0.1}
{"Action":"skip","Package":"m/cmd/csync","Elapsed":0}
`

func parseSample(t *testing.T) Summary {
	t.Helper()
	s, err := Parse(strings.NewReader(sampleStream))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	return s
}

// TestParse_ScenarioTally counts each TestFeatures subtest once — including a
// title containing a slash — and never counts the TestFeatures parent.
func TestParse_ScenarioTally(t *testing.T) {
	s := parseSample(t)
	want := Outcome{Passed: 2, Failed: 1, Skipped: 1}
	if s.Scenarios != want {
		t.Errorf("scenarios = %+v, want %+v", s.Scenarios, want)
	}
}

// TestParse_UnitTally counts unit test functions, not their table subtests.
func TestParse_UnitTally(t *testing.T) {
	s := parseSample(t)
	want := Outcome{Passed: 1, Failed: 1, Skipped: 0}
	if s.Units != want {
		t.Errorf("units = %+v, want %+v", s.Units, want)
	}
}

// TestParse_DurationSumsPackageElapsed totals the package-level wall times.
func TestParse_DurationSumsPackageElapsed(t *testing.T) {
	s := parseSample(t)
	if s.Seconds < 1.79 || s.Seconds > 1.81 {
		t.Errorf("Seconds = %v, want ~1.8", s.Seconds)
	}
}

// TestParse_ShortensPackageNames trims the module root to "." and a leading
// "root/" off the rest, sorted with "." first.
func TestParse_ShortensPackageNames(t *testing.T) {
	s := parseSample(t)
	var got []string
	for _, p := range s.Packages {
		got = append(got, p.Name)
	}
	want := []string{".", "cmd/csync", "internal/cli", "internal/compare"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("package names = %v, want %v", got, want)
	}
}

// TestParse_RecordsSkipPackagesAsNoTests marks a package with no test files.
func TestParse_RecordsSkipPackagesAsNoTests(t *testing.T) {
	s := parseSample(t)
	for _, p := range s.Packages {
		if p.Name == "cmd/csync" && p.Result != "skip" {
			t.Errorf("cmd/csync result = %q, want skip", p.Result)
		}
	}
}

// TestParse_CollectsFailureNames lists the failing scenario and unit, prettified.
func TestParse_CollectsFailureNames(t *testing.T) {
	s := parseSample(t)
	joined := strings.Join(s.Failures, "\n")
	if !strings.Contains(joined, "Broken behaviour") {
		t.Errorf("failures missing the broken scenario: %v", s.Failures)
	}
	if !strings.Contains(joined, "TestRsyncArgs") {
		t.Errorf("failures missing the broken unit: %v", s.Failures)
	}
}

// TestRender_HeadingShowsFailureStatus reports the count of failed *tests* in
// the heading — the package-level fail rows that accompany a failed test must
// not be double-counted (the sample has 1 failed scenario + 1 failed unit).
func TestRender_HeadingShowsFailureStatus(t *testing.T) {
	s := parseSample(t)
	out := Render(s, Meta{Label: "linux-x64-gnu"})
	head := firstLine(out)
	if !strings.Contains(head, "linux-x64-gnu") || !strings.Contains(head, "2 failed") {
		t.Errorf("heading = %q, want label and \"2 failed\"", head)
	}
}

// TestRender_BuildFailure flags a package that failed without any failed test
// (a compile/build error) distinctly, rather than reporting it as "0 failed".
func TestRender_BuildFailure(t *testing.T) {
	s := Summary{
		Packages: []PackageResult{{Name: ".", Result: "fail"}},
		NoTests:  true,
	}
	out := Render(s, Meta{Label: "x"})
	head := firstLine(out)
	if strings.Contains(head, "✅") || strings.Contains(head, "0 failed") {
		t.Errorf("heading = %q, want a build-failure marker", head)
	}
}

// TestRender_AllGreenHeading shows the green marker when nothing failed.
func TestRender_AllGreenHeading(t *testing.T) {
	s := Summary{Scenarios: Outcome{Passed: 30}, Units: Outcome{Passed: 20}}
	out := Render(s, Meta{Label: "macos15-arm64"})
	if !strings.Contains(firstLine(out), "✅") {
		t.Errorf("heading = %q, want green marker", firstLine(out))
	}
}

// TestRender_IncludesCountsAndEnvironment surfaces the tallies and the rsync line.
func TestRender_IncludesCountsAndEnvironment(t *testing.T) {
	s := parseSample(t)
	out := Render(s, Meta{Label: "x", Rsync: "openrsync: protocol version 29", Go: "go1.26.3", Runner: "macOS arm64"})
	for _, want := range []string{
		"Gherkin scenarios",
		"Unit tests",
		"openrsync: protocol version 29",
		"go1.26.3",
		"macOS arm64",
		"cmd/csync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
