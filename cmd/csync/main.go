// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer) that do the work.
package main

import (
	"fmt"
	"os"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
)

// main parses the command-line arguments, runs the dry-run comparison, prints
// the changes rsync would make, asks which to sync, and transfers the chosen
// files.
func main() {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: csync SOURCE DESTINATION")
		os.Exit(2)
	}

	fmt.Println("Source:", a.Source)
	fmt.Println("Destination:", a.Destination)

	result, err := compare.Run(a.Source, a.Destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Disclose what was held out of the comparison. When the local side is a git
	// repository, csync silently excludes both the .git/ metadata directory and any
	// gitignored paths; with no opt-out flag, this line is the user's only signal.
	// The .git directory is always excluded for a repo (even with nothing
	// gitignored), so its disclosure is gated on GitDirExcluded, and the gitignored
	// count is appended only when there is one. Omitted entirely for a non-repo, so
	// that sync stays noise-free.
	if result.GitDirExcluded {
		line := "Excluded: the .git directory"
		if result.Excluded > 0 {
			noun := "paths"
			if result.Excluded == 1 {
				noun = "path"
			}
			line += fmt.Sprintf(" and %d gitignored %s", result.Excluded, noun)
		}
		fmt.Println(line)
	}

	fmt.Println("Changes:", len(result.Actions))
	// Number each change from 1 in displayed order. The number is the selection
	// affordance: it's the digit a user types at the prompt to pick that change,
	// and selection.SelectActions indexes result.Actions by the same 1-based value.
	for i, act := range result.Actions {
		fmt.Printf("  %d. %s %s\n", i+1, act.Verb, act.Path)
	}

	if len(result.Actions) == 0 {
		fmt.Println("No changes to sync.")
		return
	}

	// Prompt on stderr so stdout stays a clean, parseable report.
	fmt.Fprint(os.Stderr, "Press Enter to sync all changes: ")
	selected, err := selection.SelectActions(os.Stdin, result.Actions)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	paths := make([]string, len(selected))
	for i, act := range selected {
		paths[i] = act.Path
	}
	err = transfer.Run(a.Source, a.Destination, paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Synced:", len(selected))
}
