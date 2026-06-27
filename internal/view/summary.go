// summary.go renders csync's final run-status lines: the post-sync "Sync
// complete!" summary and the "Canceled" notice. Each leads with a glyph echoing
// the prompt's "?" — a green ✓ for success, a red ✗ for a cancel — and a blank
// line to set it off from the output above. lipgloss drops the color when stdout
// is not a terminal; the glyph itself stays.

package view

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// RenderSummary builds the post-sync summary for the selected actions: a leading
// blank line, a green ✓, then a single header line counting the files moved
// (pluralized), ending in a newline so it is ready to print. With nothing selected
// it reports a zero-file header.
func RenderSummary(selected []compare.Action) string {
	noun := "files"
	if len(selected) == 1 {
		noun = "file"
	}
	check := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	return fmt.Sprintf("\n%s Sync complete! (%d %s)\n", check, len(selected), noun)
}

// Canceled returns the notice shown when the user dismisses the picker without
// syncing — a cancel (Ctrl-C/Esc/q) or a confirmed empty selection. A leading
// blank line and a red ✗ set it off as the final word, ready to print.
func Canceled() string {
	cross := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	return "\n" + cross + " Canceled\n"
}
