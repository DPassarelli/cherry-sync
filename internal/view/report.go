// report.go renders the plain-text report lines shared with the non-TTY path: the
// "(excluding …)" disclosure and the numbered change list. These carry no styling —
// the change-list numbers double as the selection affordance for the typed-grammar
// prompt — so they read the same in a terminal or piped.

package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// Excluded returns the section naming what csync withholds on its own account —
// its .csync.toml, and any .git it found — as a heading over one dimmed path per
// line, or the empty string when there is nothing to name. The paths are named
// rather than counted: there are only ever two, and a name tells the user which
// file to stop looking for. It carries no surrounding blank lines, so the caller
// can set it directly against the withheld section that follows.
func Excluded(names []string) string {
	if len(names) == 0 {
		return ""
	}
	dim := lipgloss.NewStyle().Faint(true)
	var b strings.Builder
	b.WriteString(dim.Render("Automatically excluding:") + "\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  %s\n", dim.Render(n))
	}
	return b.String()
}

// Withheld returns the section naming the changes csync found but will not offer,
// because each path is gitignored, or the empty string when there are none. Each row
// is the path, padded to a common width, then the action csync declined — the column
// shape the picker uses, so the two lists read alike. The rows carry no selection
// number: there is no opt-out for an ignored path, and a number would read as an
// offer. Only changed paths appear; the full ignored set is in the run log, where it
// costs no screen (#59).
func Withheld(actions []compare.Action) string {
	if len(actions) == 0 {
		return ""
	}
	width := 0
	for _, act := range actions {
		w := lipgloss.Width(act.Path)
		if w > width {
			width = w
		}
	}
	dim := lipgloss.NewStyle().Faint(true)
	var b strings.Builder
	b.WriteString(dim.Render("Withheld by .gitignore:") + "\n")
	for _, act := range actions {
		pad := strings.Repeat(" ", width-lipgloss.Width(act.Path))
		fmt.Fprintf(&b, "  %s\n", dim.Render(act.Path+pad+"  "+act.Verb))
	}
	return b.String()
}

// LogPath returns the disclosure of where this run's log was written, as a dimmed
// "Log written to <path>" line. csync always says where it logged, on the runs that fail as
// much as on the ones that succeed — those are the runs worth reading, and a
// record the user cannot find is no use to them. The caller prints it last, on the
// way out: nobody reads the path until something has already gone wrong, so it
// earns no room above the interactive picker, which holds every preceding line out
// of its scroll region. Like the Excluded disclosure it is faint rather than
// aligned with anything — an aside the eye can skip. The styling drops to plain
// text when stdout is not a terminal.
func LogPath(path string) string {
	dim := lipgloss.NewStyle().Faint(true)
	return dim.Render("Log written to "+path) + "\n"
}

// NotLogged returns the footer for a run that kept no record: a dimmed "Not logged:
// <reason>" line standing where a logged run names its file. csync warns about this
// before it asks what to sync, in time for the user to stop; it says so again here
// because a long change list scrolls that warning away, and the end of the run is
// where someone who wanted the record will come looking for it. A distinct label,
// not an empty "Log written to", so nothing points at a file that was never created.
func NotLogged(reason string) string {
	dim := lipgloss.NewStyle().Faint(true)
	return dim.Render("Not logged: "+reason) + "\n"
}

// ChangeList returns the non-TTY change report: a "Changes: N" count followed by
// the actions numbered from 1 in displayed order. The number is the selection
// affordance — the digit a user types at the typed-grammar prompt to pick that
// change — and selection.SelectActions indexes the same actions by that 1-based
// value. With no actions it reports just "Changes: 0".
//
// Every annotated row carries its comparison here, where the picker shows only the
// row under the cursor. There is no cursor in a piped report and nothing to move,
// so withholding the text would leave it unreachable; a report is read rather than
// scanned, which is also why the extra width costs nothing. The parentheses close
// the annotation off from the path, which may itself contain spaces.
func ChangeList(actions []compare.Action, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Changes: %d\n", len(actions))
	for i, act := range actions {
		detail := actionDetail(act, now)
		if detail == "" {
			fmt.Fprintf(&b, "  %d. %s %s\n", i+1, act.Verb, act.Path)
			continue
		}
		fmt.Fprintf(&b, "  %d. %s %s  (%s)\n", i+1, act.Verb, act.Path, detail)
	}
	return b.String()
}
