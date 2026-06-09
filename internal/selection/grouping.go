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

// GroupByDir buckets actions into one Group per directory for display, starting
// a new Group each time a row's directory differs from the previous row's. Input
// is expected as compare produces it — tree-sorted, so same-directory rows are
// contiguous — which yields exactly one Group per directory. Input order is
// preserved verbatim, so the flat numbering a user selects against stays intact.
func GroupByDir(actions []compare.Action) []Group {
	var groups []Group
	for _, a := range actions {
		header := headerFor(a.Path)
		if len(groups) == 0 || groups[len(groups)-1].Dir != header {
			groups = append(groups, Group{Dir: header})
		}
		last := &groups[len(groups)-1]
		last.Actions = append(last.Actions, a)
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
