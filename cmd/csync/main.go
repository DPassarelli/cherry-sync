// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer) that do the work.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/config"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
)

// joinAnd renders a list as English prose: "a", "a and b", or "a, b and c". It
// composes the Excluded disclosure line, which can name one to three withheld
// things.
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

// main parses the command-line arguments, runs the dry-run comparison, prints
// the changes rsync would make, asks which to sync, and transfers the chosen
// files.
func main() {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: csync SOURCE DESTINATION")
		os.Exit(2)
	}

	// With a saved-target verb the operands aren't on the command line: resolve
	// them from the project's .csync.toml in the current directory. Push sends the
	// project (".") to the saved remote; pull brings the saved remote down to the
	// project. The direction is the only difference — which side is source.
	source, destination := a.Source, a.Destination
	if a.Mode != cli.Explicit {
		cfg, err := config.Load(".")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		switch a.Mode {
		case cli.Push:
			source, destination = ".", cfg.Remote
		case cli.Pull:
			source, destination = cfg.Remote, "."
		}
	}

	fmt.Println("Source:", source)
	fmt.Println("Destination:", destination)

	result, err := compare.Run(source, destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Disclose what was held out of the comparison — with no opt-out flag, this
	// line is the user's only signal. Up to three independent things can be
	// withheld: csync's own .csync.toml (whenever present), the .git/ metadata
	// directory (when the local side is a repo), and gitignored paths. Each shows
	// only when it applies, joined into one English list; omitted entirely when
	// nothing was withheld, so a clean sync stays noise-free.
	var excluded []string
	if result.CsyncTomlExcluded {
		excluded = append(excluded, ".csync.toml")
	}
	if result.GitDirExcluded {
		excluded = append(excluded, "the .git directory")
	}
	if result.Excluded > 0 {
		noun := "paths"
		if result.Excluded == 1 {
			noun = "path"
		}
		excluded = append(excluded, fmt.Sprintf("%d gitignored %s", result.Excluded, noun))
	}
	if len(excluded) > 0 {
		fmt.Println("Excluded:", joinAnd(excluded))
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
	err = transfer.Run(source, destination, paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Synced:", len(selected))
}
