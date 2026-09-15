// changes.go derives what the rest of the run needs from a finished comparison:
// the disclosure of what was held back, and the split of a selection into the two
// passes rsync needs to carry it out.

package main

import (
	"fmt"

	"github.com/dpassarelli/cherry-sync/internal/compare"
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

// splitByVerb separates a selection into the paths to transfer and the paths to
// remove. rsync moves files with one mechanism (--files-from) and removes them with
// another (a --delete filter pass), so each verb has to go to its own call.
func splitByVerb(selected []compare.Action) (transfers, removals []string) {
	for _, act := range selected {
		if act.Verb == "delete" {
			removals = append(removals, act.Path)
		} else {
			transfers = append(transfers, act.Path)
		}
	}
	return transfers, removals
}
