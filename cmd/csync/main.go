// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer, tui) that do the work.
package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
	"github.com/dpassarelli/cherry-sync/internal/tui"
)

// main parses the command-line arguments, runs the dry-run comparison, asks which
// changes to sync — through the interactive picker on a terminal, or the typed
// prompt otherwise — and transfers the chosen files.
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

	interactive := interactiveTerminal()

	if len(result.Actions) == 0 {
		// Nothing to do — stop before any selection UI. The non-interactive path
		// still leads with the machine-readable "Changes: 0" line it always prints;
		// the interactive path just states it plainly.
		if !interactive {
			fmt.Println("Changes:", 0)
		}
		fmt.Println("No changes to sync.")
		return
	}

	// Pick the selection front-end by whether we're attached to a terminal on both
	// ends. With a real terminal, present the Bubble Tea picker; otherwise (piped,
	// redirected, or under the test harness) print the plain change list and read
	// the typed-grammar response from stdin. The picker renders its own list, so the
	// "Changes:" report is non-interactive-only.
	var selected []compare.Action
	if interactive {
		chosen, accepted, err := tui.RunPicker(result.Actions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// A cancel (Ctrl-C/Esc/q) is distinct from a confirmed empty selection:
		// report it as "Canceled" and stop, rather than transferring nothing and
		// printing "Synced: 0".
		if !accepted {
			fmt.Println("Canceled")
			return
		}
		selected = chosen
	} else {
		fmt.Println("Changes:", len(result.Actions))
		// Number each change from 1 in displayed order. The number is the selection
		// affordance: it's the digit a user types at the prompt to pick that change,
		// and selection.SelectActions indexes result.Actions by the same 1-based value.
		for i, act := range result.Actions {
			fmt.Printf("  %d. %s %s\n", i+1, act.Verb, act.Path)
		}
		// Prompt on stderr so stdout stays a clean, parseable report.
		fmt.Fprint(os.Stderr, "Press Enter to sync all changes: ")
		selected, err = selection.SelectActions(os.Stdin, result.Actions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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

// interactiveTerminal reports whether csync is attached to a real terminal on both
// ends — stdin (to read keys) and stdout (to render). Only then is the Bubble Tea
// picker usable; a piped or redirected run, including the test harness, takes the
// typed-grammar fallback instead.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
