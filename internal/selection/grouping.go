// grouping.go buckets compare's actions by directory for display, turning the
// flat, tree-sorted action list into the per-directory sections the file picker
// renders. Grouping only — it never changes which actions are selected.

package selection

import (
	"path"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// Group is a run of actions that share a directory. Dir is the display header
// for that directory relative to the transfer root ("./" for the root itself,
// "./sub" for a subdirectory).
type Group struct {
	Dir     string
	Actions []compare.Action
}

// GroupByDir buckets actions into one Group per distinct directory: directories
// appear in first-appearance order, and within each, actions keep their input
// order. It coalesces — a directory's files are gathered into a single Group even
// when they aren't contiguous in the input. compare's sort interleaves a
// directory's files with subdirectories (dot-before-non-dot and
// file-before-subdirectory ordering can split a directory's rows apart), so a
// run-based grouping would emit the directory under two headings; coalescing
// avoids that. Coalescing can move a later same-directory row earlier, so the
// picker adopts this grouped order as the order it displays and the cursor moves
// through (see newModel).
func GroupByDir(actions []compare.Action) []Group {
	var groups []Group
	at := make(map[string]int) // directory header -> its index in groups
	for _, a := range actions {
		header := headerFor(a.Path)
		i, ok := at[header]
		if !ok {
			i = len(groups)
			at[header] = i
			groups = append(groups, Group{Dir: header})
		}
		groups[i].Actions = append(groups[i].Actions, a)
	}
	return groups
}

// headerFor returns the display header for a file's directory: "./" when the file
// sits at the transfer root, otherwise "./" joined to its parent path.
func headerFor(p string) string {
	dir := path.Dir(p)
	if dir == "." {
		return "./"
	}
	return "./" + dir
}
