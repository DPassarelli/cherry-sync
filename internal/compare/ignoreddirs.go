// ignoreddirs.go holds the collapse of withheld changes that sit inside an
// ignored directory, so a directory that exists only on the far side of a pull
// is disclosed the way a push already discloses it: once, by name, rather than
// file by file.

package compare

import (
	"context"
	"slices"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// collapsed holds the outcome of collapseIgnoredDirs: the withheld changes that
// still deserve their own row, and the ignored directories that absorbed the rest.
type collapsed struct {
	withheld []Action
	dirs     []string
}

// collapseIgnoredDirs removes from withheld every change that lies beneath an
// ignored directory, recording that directory in its place. A push never needs
// this, because `git ls-files --directory` sees a local ignored directory and it is
// pre-excluded from the walk. On a pull, a directory that exists only on the remote
// is invisible to that query, so rsync itemizes everything inside it and each file
// is withheld individually; collapsing restores parity with the push.
//
// It only regroups changes already withheld, so it can never cause a path to be
// offered or hidden from the picker; at worst it changes how a withheld change is
// disclosed. Ancestors are queried with a trailing slash because git cannot apply a
// directory-only rule (`build/`) to a path that does not exist locally unless the
// path itself says it is a directory (verified by experiment).
func collapseIgnoredDirs(ctx context.Context, r *command.Runner, dir string, withheld []Action) (collapsed, error) {
	var candidates []string
	for _, a := range withheld {
		for _, d := range ancestorDirs(a.Path) {
			if !slices.Contains(candidates, d) {
				candidates = append(candidates, d)
			}
		}
	}
	if len(candidates) == 0 {
		return collapsed{withheld: withheld}, nil
	}
	ignored, err := checkIgnored(ctx, r, dir, candidates)
	if err != nil {
		return collapsed{}, err
	}
	var c collapsed
	for _, a := range withheld {
		top, ok := topmostIgnored(a.Path, ignored)
		if !ok {
			c.withheld = append(c.withheld, a)
			continue
		}
		if !slices.Contains(c.dirs, top) {
			c.dirs = append(c.dirs, top)
		}
	}
	return c, nil
}

// topmostIgnored returns the shallowest ancestor directory of path that is in
// ignored. The shallowest is the one a user's rule names: git reports every
// directory beneath an ignored one as ignored too, so a deeper match would name
// `build/a/` where the .gitignore says `build/`.
func topmostIgnored(path string, ignored map[string]bool) (string, bool) {
	for _, d := range ancestorDirs(path) {
		if ignored[d] {
			return d, true
		}
	}
	return "", false
}

// ancestorDirs returns the directories that contain path, shallowest first, each
// with a trailing slash: "a/b/c.txt" yields "a/" and "a/b/". A path that names a
// directory itself (ending in "/") is not its own ancestor.
func ancestorDirs(path string) []string {
	var dirs []string
	for i := 0; i < len(path)-1; i++ {
		if path[i] == '/' {
			dirs = append(dirs, path[:i+1])
		}
	}
	return dirs
}
