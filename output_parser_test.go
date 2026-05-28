package csync_test

import (
	"regexp"
	"strings"
)

// ReportedOutput mirrors the labeled lines csync prints to stdout, plus the
// usage message it may print to stderr. It is the single point of translation
// between rendered output and test assertions — when the rendering changes,
// parseOutput is the only thing that needs to change with it.
type ReportedOutput struct {
	Source      string
	Destination string
	Usage       string
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
		}
	}
	out.Usage = strings.TrimSpace(stderr)
	return out
}
