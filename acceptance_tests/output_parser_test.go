package acceptance_test

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
	Source            string
	Destination       string
	ChangeCount       int
	HasChangeCount    bool
	ExcludedCount     int
	HasExcludedCount  bool
	ExcludedGitDir    bool
	ExcludedCsyncToml bool
	Actions           []Action
	SyncCount         int
	HasSyncCount      bool
	RemovedCount      int
	HasRemovedCount   bool
	Message           string
	Version           string
	LogPath           string
	HasLogPath        bool
	NotLogged         string
	Warning           string
	Usage             string
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
	// The leading indent is [^\S\n]+ (horizontal whitespace), not \s+: \s matches
	// newlines, so a \s+ indent would reach across a blank line and swallow the
	// following status line ("No changes to sync.") as a bogus action.
	actionLineRE = regexp.MustCompile(`(?m)^[^\S\n]+(?:(\d+)\.\s+)?(\S+)\s+(.+?)\s*$`)
	// excludingRE captures the parenthetical disclosure of what was held out of the
	// comparison, e.g. "(excluding .csync.toml, the .git directory, and 3 gitignored
	// paths)". The captured group is the inner clause, parsed for its parts below.
	excludingRE = regexp.MustCompile(`(?m)^\(excluding (.+)\)\s*$`)
	// gitignoredCountRE pulls the gitignored-path count out of the excluding clause
	// (e.g. "the .git directory and 3 gitignored paths"). The .git directory is
	// disclosed separately and is not part of this count.
	gitignoredCountRE = regexp.MustCompile(`(\d+) gitignored`)
	// syncCompleteRE pulls the file count out of the post-sync summary header
	// ("Sync complete! (3 files)" / "(1 file)"), the count that used to be the
	// "Synced: N" line.
	syncCompleteRE = regexp.MustCompile(`Sync complete! \((\d+) `)
	// removedRE pulls the removal count out of the summary's distinct-removals
	// clause ("3 files total, 2 of which were removed"). Absent when the sync
	// applied no deletions and the header stays in its plain "(N files)" form.
	removedRE = regexp.MustCompile(`(\d+) of which (?:was|were) removed`)
	// versionLineRE captures the first line `csync --version` prints — either
	// "cherry-sync v<semver>" for a release build or "cherry-sync (dev build)"
	// for an un-injected one. The description and URL follow on their own lines;
	// this matches only the version line.
	versionLineRE = regexp.MustCompile(`(?m)^(cherry-sync (?:v.+|\(dev build\)))\s*$`)
	// logPathRE captures the path of the run log csync wrote. It is matched against
	// both whole streams rather than the report region, so where csync chooses to
	// disclose the path is not something the scenarios depend on: a clean run reports
	// it on stdout below the summary, a failed one on stderr beside the error. The
	// scenario that asserts csync named NO log needs both, or a stray path on the
	// stream it did not read would pass for silence.
	//
	// The path is captured with `*`, not `+`, so a "Log:" line with nothing after it
	// still matches: HasLogPath then reports the line, and LogPath its empty value.
	// Requiring a character would make an empty disclosure indistinguishable from no
	// disclosure, and csync printing "Log: " for a file it never wrote would read as
	// silence.
	logPathRE = regexp.MustCompile(`(?m)^Log:[^\S\n]*(.*?)[^\S\n]*$`)
	// notLoggedRE captures the reason csync gives, as it exits, for having kept no
	// record of the run. It stands in the same place as the Log: line and carries a
	// different label, so a reader — and the scenario asserting csync named no log —
	// cannot mistake one for the other.
	notLoggedRE = regexp.MustCompile(`(?m)^Not logged:[^\S\n]*(.*?)[^\S\n]*$`)
	// warningRE captures a non-fatal diagnostic csync prints to stderr and carries on
	// past — today, only its inability to write a run log.
	warningRE = regexp.MustCompile(`(?m)^warning:\s+(.+?)\s*$`)
)

// operandValue strips the inline "(rewritten from …)" disclosure the header
// appends to a rewritten Source/Destination value, so the reported operand is the
// path csync uses, not the note beside it. A value with no such note is returned
// unchanged.
func operandValue(v string) string {
	before, _, found := strings.Cut(v, " (rewritten from ")
	if found {
		return before
	}
	return v
}

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
		// Split at the start of the summary's line, not at "Sync complete!" itself, so
		// a leading status glyph ("✓ ") goes with the summary rather than dangling in
		// the report where the message scan would mistake it for the status line.
		lineStart := strings.LastIndex(stdout[:idx], "\n") + 1
		report = stdout[:lineStart]
		summary = stdout[lineStart:]
	}
	cm := syncCompleteRE.FindStringSubmatch(summary)
	if cm != nil {
		out.HasSyncCount = true
		n, err := strconv.Atoi(cm[1])
		if err == nil {
			out.SyncCount = n
		}
	}
	rm := removedRE.FindStringSubmatch(summary)
	if rm != nil {
		out.HasRemovedCount = true
		n, err := strconv.Atoi(rm[1])
		if err == nil {
			out.RemovedCount = n
		}
	}

	vm := versionLineRE.FindStringSubmatch(report)
	if vm != nil {
		out.Version = vm[1]
	}

	lm := logPathRE.FindStringSubmatch(stdout)
	if lm == nil {
		lm = logPathRE.FindStringSubmatch(stderr)
	}
	if lm != nil {
		out.HasLogPath = true
		out.LogPath = lm[1]
	}

	nm := notLoggedRE.FindStringSubmatch(stdout)
	if nm == nil {
		nm = notLoggedRE.FindStringSubmatch(stderr)
	}
	if nm != nil {
		out.NotLogged = nm[1]
	}

	wm := warningRE.FindStringSubmatch(stderr)
	if wm != nil {
		out.Warning = wm[1]
	}

	for _, m := range labeledLineRE.FindAllStringSubmatch(report, -1) {
		switch m[1] {
		case "Source":
			out.Source = operandValue(m[2])
		case "Destination":
			out.Destination = operandValue(m[2])
		case "Changes":
			out.HasChangeCount = true
			n, err := strconv.Atoi(m[2])
			if err == nil {
				out.ChangeCount = n
			}
		}
	}

	// The "(excluding …)" aside discloses what was held out of the comparison: the
	// .git directory (when the local side is a repo), csync's own .csync.toml, and/or
	// a gitignored-path count. They are reported independently — .git/ exclusion can
	// show with no gitignored paths at all — so parse them as separate signals rather
	// than a single leading number.
	em := excludingRE.FindStringSubmatch(report)
	if em != nil {
		out.ExcludedGitDir = strings.Contains(em[1], ".git directory")
		out.ExcludedCsyncToml = strings.Contains(em[1], ".csync.toml")
		cm := gitignoredCountRE.FindStringSubmatch(em[1])
		if cm != nil {
			out.HasExcludedCount = true
			n, err := strconv.Atoi(cm[1])
			if err == nil {
				out.ExcludedCount = n
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
		if labeledLineRE.MatchString(line) || actionLineRE.MatchString(line) || excludingRE.MatchString(line) {
			continue
		}
		out.Message = strings.TrimSpace(line)
		break
	}
	out.Usage = strings.TrimSpace(stderr)
	return out
}
