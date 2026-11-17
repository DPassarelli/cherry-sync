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
// the files beneath it, actions the verbs among them). itemized marks actions that
// carry a count per verb, which between them already state the file count.
type withheldRow struct {
	label    string
	count    int
	actions  string
	itemized bool
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
		rows = append(rows, foldedRow(key, files[key], plan.deleted[key]))
	}
	return rows
}

// foldedRow builds the row for folder from the files folded into it. A deleted
// folder reads as a deletion even when it holds no files at all.
func foldedRow(folder string, files []compare.Action, deleted bool) withheldRow {
	if deleted {
		return withheldRow{label: folder, count: len(files), actions: "delete"}
	}
	tally := verbTally(files)
	return withheldRow{label: folder, count: len(files), actions: describeVerbs(tally), itemized: len(tally) > 1}
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

// verbTally counts how many of actions carry each verb.
func verbTally(actions []compare.Action) map[string]int {
	tally := map[string]int{}
	for _, a := range actions {
		tally[a.Verb]++
	}
	return tally
}

// describeVerbs writes a folded row's actions: the bare verb when they all agree,
// or each verb's count otherwise, most frequent first and ties by name, so the same
// set of changes always reads the same way.
func describeVerbs(tally map[string]int) string {
	verbs := make([]string, 0, len(tally))
	for v := range tally {
		verbs = append(verbs, v)
	}
	if len(verbs) == 1 {
		return verbs[0]
	}
	slices.SortFunc(verbs, func(a, b string) int {
		return cmp.Or(cmp.Compare(tally[b], tally[a]), cmp.Compare(a, b))
	})
	parts := make([]string, len(verbs))
	for i, v := range verbs {
		parts[i] = groupDigits(tally[v]) + " " + v
	}
	return strings.Join(parts, " · ")
}
