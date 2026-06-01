package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/compare"
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

	fmt.Println("Changes:", len(result.Actions))
	for _, act := range result.Actions {
		fmt.Printf("  %s %s\n", act.Verb, act.Path)
	}

	if len(result.Actions) == 0 {
		fmt.Println("No changes to sync.")
		return
	}

	// Prompt on stderr so stdout stays a clean, parseable report.
	fmt.Fprint(os.Stderr, "Press Enter to sync all changes: ")
	selected, err := selectActions(os.Stdin, result.Actions)
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

// selectActions reads one line of input and returns the actions the user chose.
// A bare Enter (empty line) accepts the default: every change. End-of-input
// with no line at all — a closed or non-interactive stdin — selects nothing, so
// running csync without answering the prompt never transfers anything. Other
// responses (a single index, "n", etc.) are drafted as @wip scenarios in
// select-and-sync.feature and land here as each is drilled in.
func selectActions(r io.Reader, actions []compare.Action) ([]compare.Action, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	// Nothing was read at all (closed/non-interactive stdin, or Ctrl-D before
	// typing): select nothing. This is distinct from the empty *line* below —
	// both trim to "", but only a bare Enter means "accept the default". The
	// `line == ""` guard lets a partial line with no trailing newline (e.g.
	// "a"+Ctrl-D) still fall through to the switch rather than be discarded.
	if err != nil && line == "" {
		return nil, nil
	}
	switch strings.TrimSpace(line) {
	case "", "a": // bare Enter or "a": accept the default — every change
		return actions, nil
	default:
		return nil, fmt.Errorf("unrecognized selection: %q", strings.TrimSpace(line))
	}
}
