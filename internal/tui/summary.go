// summary.go renders the post-sync report: a "Sync complete!" header and a list
// of what moved, each file tagged with its change in the past tense and colored
// by verb. csync is human-first in both modes, so the same summary prints on the
// interactive and the non-TTY paths alike — lipgloss simply drops the color when
// stdout isn't a terminal. Rendering only — the visual layer; the one piece of
// content logic, the past-tense mapping, is unit-tested (pastTense).

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// RenderSummary builds the post-sync summary for the selected actions: a header
// counting the files moved, then one row per file showing its path and what
// happened to it, the verb past-tensed and colored like the picker's. The path
// column is padded so the verbs align. With nothing selected it reports just the
// zero-file header and no list. The result ends in a newline, ready to print.
func RenderSummary(selected []compare.Action) string {
	var b strings.Builder

	noun := "files"
	if len(selected) == 1 {
		noun = "file"
	}
	fmt.Fprintf(&b, "Sync complete! (%d %s)\n", len(selected), noun)

	if len(selected) == 0 {
		return b.String()
	}

	// Align the verb column by padding every path to the widest. lipgloss.Width
	// measures display cells, so multi-byte paths line up too.
	width := 0
	for _, a := range selected {
		w := lipgloss.Width("./" + a.Path)
		if w > width {
			width = w
		}
	}

	b.WriteString("\nThe following changes were made:\n")
	for _, a := range selected {
		path := "./" + a.Path
		pad := strings.Repeat(" ", width-lipgloss.Width(path))
		verb := verbStyle(a.Verb).Render(pastTense(a.Verb))
		fmt.Fprintf(&b, "   %s%s   %s\n", path, pad, verb)
	}
	return b.String()
}

// pastTense translates a verb into the past-tense word the summary displays:
// create→created, update→updated, delete→deleted. delete is reserved (csync
// emits no delete actions yet), and an unrecognized verb passes through
// unchanged so it still names itself rather than rendering blank.
func pastTense(verb string) string {
	switch verb {
	case "create":
		return "created"
	case "update":
		return "updated"
	case "delete":
		return "deleted"
	default:
		return verb
	}
}
