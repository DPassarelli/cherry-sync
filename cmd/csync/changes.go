// changes.go derives what the rest of the run needs from a finished comparison:
// the disclosure of what the comparison held back, and the change list in the
// shape the run log records it in.

package main

import (
	"fmt"

	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/runlog"
)

// excludedNotice names what the comparison held back, for the one line that
// discloses it. There is no opt-out flag, so that line is the user's only signal.
// Up to three independent things can be withheld: csync's own .csync.toml (whenever
// present), the .git/ metadata directory (when the local side is a repo), and
// gitignored paths. Each appears only when it applies, and nothing withheld returns
// nothing, so a clean sync stays noise-free.
func excludedNotice(result compare.Result) []string {
	var excluded []string
	if result.CsyncTomlExcluded {
		excluded = append(excluded, ".csync.toml")
	}
	if result.GitDirExcluded {
		excluded = append(excluded, "the .git directory")
	}
	n := len(result.Excluded)
	if n > 0 {
		noun := "paths"
		if n == 1 {
			noun = "path"
		}
		excluded = append(excluded, fmt.Sprintf("%d gitignored %s", n, noun))
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
