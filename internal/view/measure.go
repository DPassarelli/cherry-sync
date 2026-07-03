// measure.go counts how many terminal rows a block of text occupies at a given
// width, so the picker can hold the preamble main printed above it out of the
// scroll region — including the extra rows a line too wide for the terminal wraps
// onto.

package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// countRows returns the number of on-screen rows s occupies when printed at width
// columns: one row per newline-separated line, plus the extra rows any line wider
// than width wraps onto. Width is measured in display cells (lipgloss.Width strips
// ANSI escapes), so styled text counts by what's visible, not its byte length. A
// non-positive width — the terminal size before the first WindowSizeMsg — falls back
// to one row per line. The empty string is zero rows.
func countRows(s string, width int) int {
	if s == "" {
		return 0
	}
	rows := 0
	for line := range strings.SplitSeq(strings.TrimSuffix(s, "\n"), "\n") {
		w := lipgloss.Width(line)
		if width <= 0 || w == 0 {
			rows++
			continue
		}
		rows += (w + width - 1) / width
	}
	return rows
}
