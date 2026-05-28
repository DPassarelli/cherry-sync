package csync_test

import (
	"regexp"
	"strconv"
	"strings"
)

// ReportedOutput mirrors the labeled lines csync prints to stdout, plus the
// usage message it may print to stderr. It is the single point of translation
// between rendered output and test assertions — when the rendering changes,
// parseOutput is the only thing that needs to change with it.
type ReportedOutput struct {
	Source         string
	Destination    string
	ChangeCount    int
	HasChangeCount bool
	Actions        []Action
	Usage          string
}

// Action is a single planned change in csync's reported output: a verb
// (e.g. update) and the path it applies to. Production's Action type lives
// in internal/compare; this one is the test-side view of the rendered text.
type Action struct {
	Verb string
	Path string
}

var (
	labeledLineRE = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z ]*):\s+(.+?)\s*$`)
	actionLineRE  = regexp.MustCompile(`(?m)^\s+(\S+)\s+(.+?)\s*$`)
)

func parseOutput(stdout, stderr string) ReportedOutput {
	var out ReportedOutput
	for _, m := range labeledLineRE.FindAllStringSubmatch(stdout, -1) {
		switch m[1] {
		case "Source":
			out.Source = m[2]
		case "Destination":
			out.Destination = m[2]
		case "Changes":
			out.HasChangeCount = true
			n, err := strconv.Atoi(m[2])
			if err == nil {
				out.ChangeCount = n
			}
		}
	}
	for _, m := range actionLineRE.FindAllStringSubmatch(stdout, -1) {
		out.Actions = append(out.Actions, Action{Verb: m[1], Path: m[2]})
	}
	out.Usage = strings.TrimSpace(stderr)
	return out
}
