// withheld_rows.go decides how the withheld changes fold into rows: which stand
// alone, and which fold together under a deleted folder or a folder summary.

package view

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// summaryThreshold is the most withheld files a top-level folder can hold and
// still be listed file by file. Past it, the folder collapses to one summary row:
// an ignored directory that exists only on the far side of a pull can run to
// thousands of rows and bury the changes actually on offer.
const summaryThreshold = 10

// withheldRow is one line of the withheld section: either a single change (count
// 0, label the path, actions its verb) or a folded folder (label the folder, count
// the files beneath it, actions the verbs among them). removesFolder marks a fold
// whose folder is itself being deleted rather than merely holding changes.
type withheldRow struct {
	label         string
	count         int
	actions       string
	removesFolder bool
}

// rowPlan holds what is known about the whole set of withheld changes before any
// row is laid out, since whether a change folds depends on its neighbours.
type rowPlan struct {
	deleted  map[string]bool
	topFiles map[string]int
}

// withheldRows lays out the withheld changes as rows. A deleted folder folds into
// one row with everything beneath it, owned by the outermost folder being deleted,
// because rsync names the folder beside its contents and listing both says the same
// thing twice (#139). A top-level folder with more than summaryThreshold files folds
// into one summary row instead, absorbing any deleted folder inside it. Each folded
// row stands where its first change stood. Only files are counted: the folder
// entries rsync reports for a deletion are not files a user would recognize.
// Grouping by the top-level folder alone keeps the rule predictable: the user can
// tell from a path which summary it would land in.
func withheldRows(actions []compare.Action) []withheldRow {
	plan := newRowPlan(actions)
	keys := make([]string, len(actions))
	files := map[string][]compare.Action{}
	for i, a := range actions {
		keys[i] = plan.rowKey(a.Path)
		if keys[i] != "" && !isFolder(a.Path) {
			files[keys[i]] = append(files[keys[i]], a)
		}
	}
	var rows []withheldRow
	laidOut := map[string]bool{}
	for i, a := range actions {
		key := keys[i]
		if key == "" {
			rows = append(rows, withheldRow{label: a.Path, actions: a.Verb})
			continue
		}
		if laidOut[key] {
			continue
		}
		laidOut[key] = true
		row := withheldRow{label: key, count: len(files[key]), removesFolder: plan.deleted[key]}
		row.actions = "delete"
		if !row.removesFolder {
			row.actions = verbCounts(files[key])
		}
		rows = append(rows, row)
	}
	return rows
}

// newRowPlan gathers the deleted folders and each top-level folder's file count.
func newRowPlan(actions []compare.Action) rowPlan {
	plan := rowPlan{deleted: map[string]bool{}, topFiles: map[string]int{}}
	for _, a := range actions {
		if isFolder(a.Path) {
			plan.deleted[a.Path] = a.Verb == "delete"
			continue
		}
		top, ok := topFolder(a.Path)
		if ok {
			plan.topFiles[top]++
		}
	}
	return plan
}

// rowKey names the folder whose row the change at path folds into, or returns the
// empty string when it stands alone. A summarized top-level folder takes priority
// over a deleted folder inside it; a deleted top-level folder is never summarized,
// since it already reads as one change.
func (p rowPlan) rowKey(path string) string {
	owner := p.deletedOwner(path)
	top, ok := topFolder(path)
	if ok && owner != top && p.topFiles[top] > summaryThreshold {
		return top
	}
	return owner
}

// deletedOwner returns the outermost deleted folder that is path or contains it,
// or the empty string when no such folder is being deleted.
func (p rowPlan) deletedOwner(path string) string {
	for i := range len(path) {
		if path[i] == '/' && p.deleted[path[:i+1]] {
			return path[:i+1]
		}
	}
	return ""
}

// isFolder reports whether path names a folder, which rsync marks with a trailing
// slash.
func isFolder(path string) bool {
	return strings.HasSuffix(path, "/")
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
