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
	Source           string
	Destination      string
	ChangeCount      int
	HasChangeCount   bool
	ExcludedCount    int
	HasExcludedCount bool
	ExcludedGitDir   bool
	Actions          []Action
	SyncCount        int
	HasSyncCount     bool
	Message          string
	Usage            string
}

// Action is a single planned change in csync's reported output: a verb
// (e.g. update), the path it applies to, and Index — the 1-based selection
// number shown next to the change (the digit a user types to pick it). Index is
// 0 when the rendered line carries no number. Production's Action type lives in
// internal/compare; this one is the test-side view of the rendered text.
type Action struct {
	Index int
	Verb  string
	Path  string
}

// labeledLineRE and actionLineRE match the two line shapes csync prints:
// labeledLineRE captures `Label: value` summary lines (Source, Destination,
// Changes); actionLineRE captures the indented action lines, with an optional
// leading `N.` selection number ahead of the `verb path` pair.
var (
	labeledLineRE = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z ]*):\s+(.+?)\s*$`)
	actionLineRE  = regexp.MustCompile(`(?m)^\s+(?:(\d+)\.\s+)?(\S+)\s+(.+?)\s*$`)
	// gitignoredCountRE pulls the gitignored-path count out of the Excluded line's
	// value (e.g. "the .git directory and 3 gitignored paths"). The .git directory
	// is disclosed separately and is not part of this count.
	gitignoredCountRE = regexp.MustCompile(`(\d+) gitignored`)
	// syncCompleteRE pulls the file count out of the post-sync summary header
	// ("Sync complete! (3 files)" / "(1 file)"), the count that used to be the
	// "Synced: N" line.
	syncCompleteRE = regexp.MustCompile(`Sync complete! \((\d+) `)
)

// parseOutput translates csync's rendered stdout and stderr into a structured
// ReportedOutput. It is the single parsing facade the test assertions read
// through, so a rendering change only has to be absorbed here.
func parseOutput(stdout, stderr string) ReportedOutput {
	var out ReportedOutput

	// The post-sync summary ("Sync complete! ...") is human prose whose indented
	// rows share the shape of the pre-sync action list, so split the two apart:
	// parse labels, actions, and the status message from the report region (before
	// the summary), and the synced file count from the summary region. Without the
	// split, a summary row like "   ./README.md   updated" would be mis-read as a
	// planned action.
	report := stdout
	summary := ""
	idx := strings.Index(stdout, "Sync complete!")
	if idx >= 0 {
		report = stdout[:idx]
		summary = stdout[idx:]
	}
	cm := syncCompleteRE.FindStringSubmatch(summary)
	if cm != nil {
		out.HasSyncCount = true
		n, err := strconv.Atoi(cm[1])
		if err == nil {
			out.SyncCount = n
		}
	}

	for _, m := range labeledLineRE.FindAllStringSubmatch(report, -1) {
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
		case "Excluded":
			// The value discloses what was held out of the comparison: the .git
			// directory (when the local side is a repo) and/or a gitignored-path
			// count, e.g. "the .git directory and 3 gitignored paths". The two are
			// reported independently — .git/ exclusion can show with no gitignored
			// paths at all — so parse them as separate signals rather than a single
			// leading number.
			out.ExcludedGitDir = strings.Contains(m[2], ".git directory")
			cm := gitignoredCountRE.FindStringSubmatch(m[2])
			if cm != nil {
				out.HasExcludedCount = true
				n, err := strconv.Atoi(cm[1])
				if err == nil {
					out.ExcludedCount = n
				}
			}
		}
	}
	for _, m := range actionLineRE.FindAllStringSubmatch(report, -1) {
		actionIdx := 0
		if m[1] != "" {
			actionIdx, _ = strconv.Atoi(m[1])
		}
		out.Actions = append(out.Actions, Action{Index: actionIdx, Verb: m[2], Path: m[3]})
	}
	// Message is the first free-text line: non-empty, and neither a `Label:
	// value` summary line nor an indented action line. csync emits one for
	// human-facing status like "No changes to sync."
	for line := range strings.SplitSeq(report, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if labeledLineRE.MatchString(line) || actionLineRE.MatchString(line) {
			continue
		}
		out.Message = strings.TrimSpace(line)
		break
	}
	out.Usage = strings.TrimSpace(stderr)
	return out
}
