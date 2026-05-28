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
// (create / update / delete) and the path it applies to. The Actions slice
// stays empty until scenarios in compare-directories.feature drill into
// non-zero-change behavior.
type Action struct {
	Verb string
	Path string
}

var labeledLineRE = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z ]*):\s+(.+?)\s*$`)

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
			if n, err := strconv.Atoi(m[2]); err == nil {
				out.ChangeCount = n
			}
		}
	}
	out.Usage = strings.TrimSpace(stderr)
	return out
}
