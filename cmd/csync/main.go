package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
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
// A bare Enter or "a" accepts the default (every change); "n" declines and
// selects nothing; a single 1-based index picks just that change. End-of-input
// with no line at all — a closed or non-interactive stdin — also selects
// nothing, so running csync without answering the prompt never transfers
// anything. Unrecognized input is an error. Multi-select grammars (ranges,
// lists) remain @wip in select-and-sync.feature and land here when drilled in.
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

	response := strings.TrimSpace(line)
	// A bare Enter or "a" accepts the default: every change.
	if response == "" || response == "a" {
		return actions, nil
	}
	// "n" explicitly declines: transfer nothing, exit cleanly.
	if response == "n" {
		return nil, nil
	}
	// A single 1-based index selects just that change from the displayed list.
	// (Multi-select grammars like "1-3" or "1,3", and out-of-range handling,
	// are separate not-yet-drilled scenarios in select-and-sync.feature.)
	if n, convErr := strconv.Atoi(response); convErr == nil && n >= 1 && n <= len(actions) {
		return []compare.Action{actions[n-1]}, nil
	}
	return nil, fmt.Errorf("unrecognized selection: %q", response)
}
