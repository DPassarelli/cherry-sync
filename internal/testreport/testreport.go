// Package testreport summarizes a `go test -json` event stream (as written by
// gotestsum's --jsonfile) into a compact Markdown panel for the GitHub Actions
// job summary and the local preview. It separates Gherkin scenarios (godog
// subtests of TestFeatures) from ordinary unit tests, so a reader sees what was
// exercised and to what degree without drilling into the run log.
package testreport

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

// scenarioPrefix is the subtest-name prefix godog gives every scenario it runs
// under TestFeatures; everything below it is a Gherkin scenario, not a unit test.
const scenarioPrefix = "TestFeatures/"

// event is the subset of a `go test -json` TestEvent that this package reads.
type event struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
}

// Parse reads a `go test -json` stream and classifies each finished test.
func Parse(r io.Reader) (Summary, error) {
	var s Summary
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // test Output lines can be large
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e event
		err := json.Unmarshal([]byte(line), &e)
		if err != nil {
			continue // tolerate the occasional non-event line
		}
		if e.Action != "pass" && e.Action != "fail" && e.Action != "skip" {
			continue // ignore run/output/start/pause/cont events
		}
		if e.Test == "" {
			s.Packages = append(s.Packages, PackageResult{Name: e.Package, Result: e.Action, Elapsed: e.Elapsed})
			s.Seconds += e.Elapsed
			continue
		}
		key := e.Package + "\x00" + e.Test
		if seen[key] {
			continue
		}
		seen[key] = true
		switch {
		case e.Test == "TestFeatures" || e.Test == "TestMain":
			// godog harness / test main — not a behaviour of its own.
		case strings.HasPrefix(e.Test, scenarioPrefix):
			tally(&s.Scenarios, e.Action)
			if e.Action == "fail" {
				s.Failures = append(s.Failures, "scenario: "+prettyScenario(e.Test))
			}
		case strings.Contains(e.Test, "/"):
			// A table-driven subtest of a unit test — counted at the function level.
		default:
			tally(&s.Units, e.Action)
			if e.Action == "fail" {
				s.Failures = append(s.Failures, "unit: "+e.Test+" ("+shortPkg(e.Package, "")+")")
			}
		}
	}
	err := sc.Err()
	if err != nil {
		return s, err
	}
	shortenPackages(&s)
	s.NoTests = s.Scenarios.total()+s.Units.total() == 0
	return s, nil
}

// tally records one finished test's action in the Outcome.
func tally(o *Outcome, action string) {
	switch action {
	case "pass":
		o.Passed++
	case "fail":
		o.Failed++
	case "skip":
		o.Skipped++
	}
}

// prettyScenario turns a godog subtest name back into a readable scenario title.
func prettyScenario(test string) string {
	return strings.ReplaceAll(strings.TrimPrefix(test, scenarioPrefix), "_", " ")
}

// shortenPackages replaces each package path with one relative to the module
// root (the shortest path seen), the root itself becoming ".", and sorts the
// rows with "." first then alphabetically. It also rewrites the module prefix
// already embedded in any unit failure entries.
func shortenPackages(s *Summary) {
	root := moduleRoot(s.Packages)
	for i := range s.Packages {
		s.Packages[i].Name = shortPkg(s.Packages[i].Name, root)
	}
	for i := range s.Failures {
		s.Failures[i] = strings.Replace(s.Failures[i], "("+root+"/", "(", 1)
	}
	sort.Slice(s.Packages, func(i, j int) bool {
		a, b := s.Packages[i].Name, s.Packages[j].Name
		if a == "." {
			return b != "."
		}
		if b == "." {
			return false
		}
		return a < b
	})
}

// moduleRoot infers the module's root import path from the packages seen: the
// longest prefix shared by every package path, cut back to a path-segment
// boundary. Deriving it this way — rather than assuming the shortest path seen is
// the root — keeps the report correct whether or not the root package itself has
// tests. Once the acceptance suite moved out of the module root (issue #57), no
// bare-root package appears in a run, so "shortest path" would wrongly pick a
// subpackage as the root and garble every label.
func moduleRoot(pkgs []PackageResult) string {
	if len(pkgs) == 0 {
		return ""
	}
	prefix := pkgs[0].Name
	for _, p := range pkgs[1:] {
		prefix = commonPrefix(prefix, p.Name)
	}
	slash := strings.LastIndex(prefix, "/")
	if slash >= 0 {
		prefix = prefix[:slash]
	}
	return prefix
}

// commonPrefix returns the longest common leading run of bytes of a and b.
func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// shortPkg trims the module root off a package path, yielding "." for the root.
func shortPkg(pkg, root string) string {
	if root != "" {
		if pkg == root {
			return "."
		}
		return strings.TrimPrefix(pkg, root+"/")
	}
	i := strings.LastIndex(pkg, "/")
	if i >= 0 {
		return pkg[i+1:]
	}
	return pkg
}
