package csync_test

import "regexp"

// ReportedOutput mirrors the labeled lines csync prints to stdout. It is the
// single point of translation between rendered output and test assertions —
// when the rendering changes, parseOutput is the only thing that needs to
// change with it.
type ReportedOutput struct {
	Source      string
	Destination string
}

var labeledLineRE = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z ]*):\s+(.+?)\s*$`)

func parseStdout(stdout string) ReportedOutput {
	var out ReportedOutput
	for _, m := range labeledLineRE.FindAllStringSubmatch(stdout, -1) {
		switch m[1] {
		case "Source":
			out.Source = m[2]
		case "Destination":
			out.Destination = m[2]
		}
	}
	return out
}
