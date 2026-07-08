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
// blank line, a green ✓, then a single header line counting the changes applied
// (pluralized), ending in a newline so it is ready to print. With nothing selected
// it reports a zero-count header.
//
// When some of the applied changes were removals, the header calls them out
// distinctly — "(3 files total, 2 of which were removed)" — so the count doesn't
// read as if every change was a transfer. With no removals it stays in the plain
// "(3 files)" form.
func RenderSummary(selected []compare.Action) string {
	total := len(selected)
	removed := 0
	for _, a := range selected {
		if a.Verb == "delete" {
			removed++
		}
	}
	check := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	if removed == 0 {
		return fmt.Sprintf("\n%s Sync complete! (%d %s)\n", check, total, pluralFiles(total))
	}
	were := "were"
	if removed == 1 {
		were = "was"
	}
	return fmt.Sprintf("\n%s Sync complete! (%d %s total, %d of which %s removed)\n", check, total, pluralFiles(total), removed, were)
}

// pluralFiles returns "file" for a count of one and "files" otherwise.
func pluralFiles(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

// Canceled returns the notice shown when the user dismisses the picker without
// syncing — a cancel (Ctrl-C/Esc/q) or a confirmed empty selection. A leading
// blank line and a red ✗ set it off as the final word, ready to print.
func Canceled() string {
	cross := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	return "\n" + cross + " Canceled\n"
}
