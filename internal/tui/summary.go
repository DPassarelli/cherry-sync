// summary.go renders the post-sync report: a one-line "Sync complete!" header
// counting the files moved. The interactive picker already shows the selected
// set, so the summary no longer re-lists each file; the same terse line prints on
// the non-TTY path too, where the pre-transfer "Changes:" list already records
// what was offered. Rendering only — the visual layer.

package tui

import (
	"fmt"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// RenderSummary builds the post-sync summary for the selected actions: a single
// header line counting the files moved (pluralized), ending in a newline so it is
// ready to print. With nothing selected it reports a zero-file header.
func RenderSummary(selected []compare.Action) string {
	noun := "files"
	if len(selected) == 1 {
		noun = "file"
	}
	return fmt.Sprintf("Sync complete! (%d %s)\n", len(selected), noun)
}
