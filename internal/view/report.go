// report.go renders the plain-text report lines shared with the non-TTY path: the
// "(excluding …)" disclosure and the numbered change list. These carry no styling —
// the change-list numbers double as the selection affordance for the typed-grammar
// prompt — so they read the same in a terminal or piped.

package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// joinAnd renders a list as English prose: "a", "a and b", or "a, b, and c". It
// composes the Excluded disclosure line, which can name one to three withheld
// things.
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
	}
}

// Excluded returns the disclosure of what was held out of the comparison as a
// dimmed parenthetical aside — "(excluding a, b, and c)" — or the empty string
// when nothing was, so the caller can print it unconditionally and a clean sync
// stays noise-free. It is deliberately not aligned with the source/destination
// header: the disclosure is a separate note, not part of that two-line block. The
// faint styling drops to plain text when stdout is not a terminal.
func Excluded(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	dim := lipgloss.NewStyle().Faint(true)
	return dim.Render("(excluding "+joinAnd(parts)+")") + "\n"
}

// LogPath returns the disclosure of where this run's log was written, as a dimmed
// "Log: <path>" line. csync always says where it logged, on the runs that fail as
// much as on the ones that succeed — those are the runs worth reading, and a
// record the user cannot find is no use to them. The caller prints it last, on the
// way out: nobody reads the path until something has already gone wrong, so it
// earns no room above the interactive picker, which holds every preceding line out
// of its scroll region. Like the Excluded disclosure it is faint rather than
// aligned with anything — an aside the eye can skip. The styling drops to plain
// text when stdout is not a terminal.
func LogPath(path string) string {
	dim := lipgloss.NewStyle().Faint(true)
	return dim.Render("Log: "+path) + "\n"
}

// ChangeList returns the non-TTY change report: a "Changes: N" count followed by
// the actions numbered from 1 in displayed order. The number is the selection
// affordance — the digit a user types at the typed-grammar prompt to pick that
// change — and selection.SelectActions indexes the same actions by that 1-based
// value. With no actions it reports just "Changes: 0".
func ChangeList(actions []compare.Action) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Changes: %d\n", len(actions))
	for i, act := range actions {
		fmt.Fprintf(&b, "  %d. %s %s\n", i+1, act.Verb, act.Path)
	}
	return b.String()
}
