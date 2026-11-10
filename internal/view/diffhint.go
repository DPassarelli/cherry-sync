// diffhint.go renders the picker's line about diffs: which files can be diffed, and
// after it, why the last one asked for couldn't be, or that one is on its way.

package view

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// diffHint states the limits on a diff up front, so a file past them is no surprise.
var diffHint = fmt.Sprintf("d diff (text files ≤ %d KiB only)", filediff.SizeLimit/1024)

// diffHintLine is the diff hint followed by any note, bold so a problem stands out,
// or else any progress. It is cut to the terminal's width rather than wrapped, since
// a second row would push the change list down and back each time a note came and
// went.
func (m pickerModel) diffHintLine() string {
	line := "   " + lipgloss.NewStyle().Faint(true).Render(diffHint)
	switch {
	case m.notice != "":
		line += " " + lipgloss.NewStyle().Bold(true).Render(m.notice)
	case m.progress != "":
		line += " " + m.progress
	}
	if m.width <= 0 {
		return line
	}
	return ansi.Truncate(line, m.width, "…")
}
