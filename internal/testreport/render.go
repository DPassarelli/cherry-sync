// render.go turns a parsed Summary into the terminal report: the per-package
// rows, their status marks and timings, and the totals beneath them.

package testreport

import (
	"fmt"
	"strings"
)

// Render turns a parsed Summary and the environment Meta into the Markdown panel.
func Render(s Summary, m Meta) string {
	// Count failed *tests*. A package-level "fail" usually just accompanies a
	// failed test in it, so it must not be added here — but a package that fails
	// with no failed test recorded is a build/compile error, surfaced on its own.
	failed := s.Scenarios.Failed + s.Units.Failed
	buildFailed := false
	for _, p := range s.Packages {
		if p.Result == "fail" {
			buildFailed = true
		}
	}

	var status string
	switch {
	case failed > 0:
		status = fmt.Sprintf("❌ %d failed", failed)
	case buildFailed:
		status = "❌ build failure (no failed tests recorded)"
	default:
		status = "✅ all green"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s · %s\n\n", m.Label, status, dur(s.Seconds))

	b.WriteString("**Environment**\n")
	fmt.Fprintf(&b, "- rsync: %s\n", orDash(m.Rsync))
	fmt.Fprintf(&b, "- %s · runner %s\n\n", orDash(m.Go), orDash(m.Runner))

	b.WriteString("**What was tested**\n\n")
	b.WriteString("| Kind | Passed | Failed | Skipped |\n|---|--:|--:|--:|\n")
	fmt.Fprintf(&b, "| Gherkin scenarios | %d | %d | %d |\n", s.Scenarios.Passed, s.Scenarios.Failed, s.Scenarios.Skipped)
	fmt.Fprintf(&b, "| Unit tests | %d | %d | %d |\n\n", s.Units.Passed, s.Units.Failed, s.Units.Skipped)

	if len(s.Failures) > 0 {
		b.WriteString("**Failures**\n")
		for _, f := range s.Failures {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	b.WriteString("<details><summary>Per-package detail</summary>\n\n")
	b.WriteString("| Package | Result | Time |\n|---|:--:|--:|\n")
	for _, p := range s.Packages {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", p.Name, packageMark(p.Result), packageTime(p))
	}
	b.WriteString("</details>\n")
	return b.String()
}

// packageMark renders a package's result as a glyph for the detail table.
func packageMark(result string) string {
	switch result {
	case "pass":
		return "✅"
	case "fail":
		return "❌"
	default:
		return "∅ no tests"
	}
}

// packageTime renders a package's elapsed time, or a dash for a no-tests skip.
func packageTime(p PackageResult) string {
	if p.Result == "skip" {
		return "—"
	}
	return dur(p.Elapsed)
}

// dur formats a duration in seconds to one decimal place.
func dur(seconds float64) string { return fmt.Sprintf("%.1fs", seconds) }

// orDash returns the string, or an em dash when it is empty.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
