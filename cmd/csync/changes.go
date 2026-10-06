// changes.go derives what the rest of the run needs from a finished comparison:
// the disclosure of what the comparison held back, and the change list in the
// shape the run log records it in.

package main

import (
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/runlog"
)

// autoExcluded names what csync withheld on its own account, in the order the
// disclosure lists them: its own .csync.toml, and a .git directory if rsync reported
// holding one back. The gitignored paths are deliberately absent — those are named
// only when they would have moved (see compare.Result.Withheld), since listing the
// rest just repeats the .gitignore the user can already read.
func autoExcluded(result compare.Result) []string {
	var excluded []string
	if result.CsyncTomlExcluded {
		excluded = append(excluded, ".csync.toml")
	}
	if result.GitDirExcluded {
		excluded = append(excluded, ".git/")
	}
	return excluded
}

// logActions adapts the compare package's actions to the run log's own Action type,
// bridging the two so runlog need not depend on compare. It is the one place the shape
// is translated for the classified and selected records.
func logActions(actions []compare.Action) []runlog.Action {
	out := make([]runlog.Action, len(actions))
	for i, a := range actions {
		out[i] = runlog.Action{Verb: a.Verb, Path: a.Path}
	}
	return out
}
