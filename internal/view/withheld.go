// withheld.go renders the "Withheld by .gitignore:" section, including the
// summary rows that stand in for a folder with many withheld changes.

package view

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// summaryThreshold is the most withheld changes a top-level folder can hold and
// still be listed file by file. Past it, the folder collapses to one summary row:
// an ignored directory that exists only on the far side of a pull can run to
// thousands of rows and bury the changes actually on offer.
const summaryThreshold = 10

// withheldRow is one line of the withheld section: either a single change (count
// 0, label the path, actions its verb) or a summary of a top-level folder (label
// the folder, count its changes, actions the verbs among them).
type withheldRow struct {
	label   string
	count   int
	actions string
}

// Withheld returns the section naming the changes csync found but will not offer,
// because each path is gitignored, or the empty string when there are none. Each row
// is the path, padded to a common width, then the action csync declined — the column
// shape the picker uses, so the two lists read alike. The rows carry no selection
// number: there is no opt-out for an ignored path, and a number would read as an
// offer. Only changed paths appear; the full ignored set is in the run log, where it
// costs no screen (#59). A summary row is display-only: it says how many changes sit
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
		if row.count > 0 {
			rest = groupDigits(row.count) + " files  " + rest
		}
		fmt.Fprintf(&b, "  %s\n", dim.Render(row.label+pad+"  "+rest))
	}
	return b.String()
}

// withheldRows lays out the withheld changes as rows, folding each top-level folder
// with more than summaryThreshold changes into one summary row placed where its
// first change stood. Grouping by the top-level folder alone keeps the rule
// predictable: the user can tell from a path which summary it would land in.
func withheldRows(actions []compare.Action) []withheldRow {
	groups := map[string][]compare.Action{}
	for _, a := range actions {
		top, ok := topFolder(a.Path)
		if ok {
			groups[top] = append(groups[top], a)
		}
	}
	var rows []withheldRow
	summarized := map[string]bool{}
	for _, a := range actions {
		top, ok := topFolder(a.Path)
		if !ok || len(groups[top]) <= summaryThreshold {
			rows = append(rows, withheldRow{label: a.Path, actions: a.Verb})
			continue
		}
		if summarized[top] {
			continue
		}
		summarized[top] = true
		rows = append(rows, withheldRow{label: top, count: len(groups[top]), actions: verbCounts(groups[top])})
	}
	return rows
}

// topFolder returns the first segment of path with its trailing slash ("build/"
// for "build/a/out.bin"), or false for a file at the transfer root.
func topFolder(path string) (string, bool) {
	i := strings.IndexByte(path, '/')
	if i < 0 || i == len(path)-1 {
		return "", false
	}
	return path[:i+1], true
}

// verbCounts describes the verbs among a summarized folder's changes: the bare verb
// when they all agree, or each verb's count otherwise, most frequent first and ties
// by name, so the same set of changes always reads the same way.
func verbCounts(actions []compare.Action) string {
	counts := map[string]int{}
	for _, a := range actions {
		counts[a.Verb]++
	}
	if len(counts) == 1 {
		return actions[0].Verb
	}
	verbs := make([]string, 0, len(counts))
	for v := range counts {
		verbs = append(verbs, v)
	}
	slices.SortFunc(verbs, func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})
	parts := make([]string, len(verbs))
	for i, v := range verbs {
		parts[i] = groupDigits(counts[v]) + " " + v
	}
	return strings.Join(parts, " · ")
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
