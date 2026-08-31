// summary.go renders csync's final run-status lines: the post-sync "Sync
// complete!" summary, the "Canceled" notice, the timeout that gives up on a
// comparison, and the notice for a transfer whose remote went silent. Each leads with a glyph echoing
// the prompt's "?" — a green ✓ for success, a red ✗ for a cancel — and a blank
// line to set it off from the output above. lipgloss drops the color when stdout
// is not a terminal; the glyph itself stays.

package view

import (
	"fmt"
	"time"

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

// TimedOut returns the notice shown when a comparison runs past the limit and is
// stopped. It wears the same red ✗ as a cancel because the outcome is the same —
// nothing was synced — but says the limit out loud, since unlike a cancel this is
// not something the user did and would otherwise look like an unexplained abort.
// It also names the way out, because csync has no flag to raise the limit and a
// tree this slow to compare is not what it is for.
func TimedOut(limit time.Duration) string {
	cross := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	return fmt.Sprintf("\n%s Gave up comparing after %d seconds.\n  csync is built for quick, interactive syncs; a comparison this slow is better run with rsync directly.\n", cross, int(limit.Seconds()))
}

// Stalled returns the notice shown when a transfer is abandoned because the
// remote stopped sending. It wears the same red ✗ as a cancel and names the limit
// for the same reason TimedOut does — an abort that states no number reads as a
// crash — but says what failed rather than what took too long, because unlike a
// slow comparison this is the remote's doing and not the tree's.
//
// It also says the sync was left half done. A transfer killed partway has already
// moved some of the selection, so a notice implying nothing happened would send
// the user back to a destination in a state neither side agrees on.
func Stalled(limit time.Duration) string {
	cross := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	return fmt.Sprintf("\n%s The remote stopped responding; gave up after %d seconds.\n  Some files may already have been transferred; run csync again to see what is left.\n", cross, int(limit.Seconds()))
}
