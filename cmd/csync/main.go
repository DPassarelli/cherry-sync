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
	"github.com/dpassarelli/cherry-sync/internal/config"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
	"github.com/dpassarelli/cherry-sync/internal/view"
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

	// Detect the terminal once: it both selects the selection front-end (picker vs.
	// typed prompt) and gates the decorative banner, which only an interactive run
	// shows — piped output stays clean.
	interactive := interactiveTerminal()

	if interactive {
		fmt.Print(view.Banner())
	}
	fmt.Print(view.Header(source, destination))

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
	fmt.Print(view.Excluded(excluded))

	if len(result.Actions) == 0 {
		// Nothing to do — stop before any selection UI. The non-interactive path
		// still leads with the machine-readable "Changes: 0" line it always prints;
		// the interactive path just states it plainly.
		if !interactive {
			fmt.Print(view.ChangeList(nil))
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
		chosen, accepted, err := view.RunPicker(result.Actions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// A cancel (Ctrl-C/Esc/q) is distinct from a confirmed empty selection:
		// report it as "Canceled" and stop, rather than transferring nothing and
		// printing the "Sync complete! (0 files)" summary.
		if !accepted {
			fmt.Println("Canceled")
			return
		}
		selected = chosen
	} else {
		fmt.Print(view.ChangeList(result.Actions))
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
	err = transfer.Run(source, destination, paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Report what moved with the post-sync summary: the past-tense list of changes.
	// csync is human-first in both modes — the non-TTY path is the degraded-but-still-
	// human fallback, not a machine interface — so it gets the same summary, only
	// without color (lipgloss drops ANSI when stdout isn't a terminal).
	fmt.Print(view.RenderSummary(selected))
}

// interactiveTerminal reports whether csync is attached to a real terminal on both
// ends — stdin (to read keys) and stdout (to render). Only then is the Bubble Tea
// picker usable; a piped or redirected run, including the test harness, takes the
// typed-grammar fallback instead.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
