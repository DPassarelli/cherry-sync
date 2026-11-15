// withheld.go renders the "Withheld by .gitignore:" section, including the
// summary rows that stand in for a folder of withheld changes.

package view

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// Withheld returns the section naming the changes csync found but will not offer,
// because each path is gitignored, or the empty string when there are none. Each row
// is the path, padded to a common width, then the action csync declined — the column
// shape the picker uses, so the two lists read alike. The rows carry no selection
// number: there is no opt-out for an ignored path, and a number would read as an
// offer. Only changed paths appear; the full ignored set is in the run log, where it
// costs no screen (#59). A summary row is display-only: it says how many files sit
// under a folder, never that the folder itself is ignored, which is why the run log
// still names every file.
func Withheld(actions []compare.Action) string {
	if len(actions) == 0 {
		return ""
	}
	rows := withheldRows(actions)
	width := 0
	for _, row := range rows {
		width = max(width, lipgloss.Width(row.label))
	}
	dim := lipgloss.NewStyle().Faint(true)
	var b strings.Builder
	b.WriteString(dim.Render("Withheld by .gitignore:") + "\n")
	for _, row := range rows {
		pad := strings.Repeat(" ", width-lipgloss.Width(row.label))
		rest := row.actions
		switch {
		case row.removesFolder && row.count > 0:
			rest += "  (with " + fileCount(row.count) + ")"
		case row.count > 0:
			rest = fileCount(row.count) + "  " + rest
		}
		fmt.Fprintf(&b, "  %s\n", dim.Render(row.label+pad+"  "+rest))
	}
	return b.String()
}

// fileCount names n files in the form a row reads them in: "1 file", "17 files".
func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return groupDigits(n) + " files"
}

// groupDigits writes n with a comma between each group of three digits, the form
// people read a large file count in at a glance.
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
