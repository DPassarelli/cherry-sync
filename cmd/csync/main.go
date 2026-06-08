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
	// and selectActions indexes result.Actions by the same 1-based value.
	for i, act := range result.Actions {
		fmt.Printf("  %d. %s %s\n", i+1, act.Verb, act.Path)
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
	// An index outside 1..len falls through to the error below, rejected like any
	// unrecognized response.
	n, convErr := strconv.Atoi(response)
	if convErr == nil && n >= 1 && n <= len(actions) {
		return []compare.Action{actions[n-1]}, nil
	}
	// A hyphen range like "1-3" selects an inclusive span of rows. A reversed
	// (3-1), out-of-range, or otherwise malformed range fails one of the guards
	// and falls through to the error below.
	lo, hi, found := strings.Cut(response, "-")
	if found {
		loN, errLo := strconv.Atoi(lo)
		hiN, errHi := strconv.Atoi(hi)
		if errLo == nil && errHi == nil && loN >= 1 && loN <= hiN && hiN <= len(actions) {
			return append([]compare.Action(nil), actions[loN-1:hiN]...), nil
		}
	}
	// A comma list like "1,3" selects exactly the rows named. Each member must be
	// a single 1-based index; if any member is out of range or not a bare index
	// (e.g. a range member as in "1-2,4"), the whole response is rejected. The
	// combined range-and-list grammar, dedup of overlapping members, and explicit
	// malformed-input rejection are drilled in by later scenarios.
	if strings.Contains(response, ",") {
		parts := strings.Split(response, ",")
		selected := make([]compare.Action, 0, len(parts))
		valid := true
		for _, p := range parts {
			m, err := strconv.Atoi(p)
			if err != nil || m < 1 || m > len(actions) {
				valid = false
				break
			}
			selected = append(selected, actions[m-1])
		}
		if valid {
			return selected, nil
		}
	}
	return nil, fmt.Errorf("unrecognized selection: %q", response)
}
