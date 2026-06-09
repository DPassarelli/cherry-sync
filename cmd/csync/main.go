// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer) that do the work.
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
// selects nothing; otherwise the line is a selection list — comma-separated
// members, each a single 1-based index ("3") or an inclusive range ("1-3") —
// resolved in order. End-of-input with no line at all — a closed or
// non-interactive stdin — also selects nothing, so running csync without
// answering the prompt never transfers anything. Any unrecognized, malformed,
// or out-of-range response is an error.
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
	// Otherwise the response is a selection list: comma-separated members, each a
	// single 1-based index ("3") or an inclusive range ("1-3"). Members resolve
	// left to right; a single index selects its one row, a range its whole span.
	// Any member that is malformed, reversed (3-1), or names a row outside 1..len
	// rejects the entire response — partial selection on a typo would silently
	// sync the wrong set. A row named more than once (e.g. an overlapping range
	// and index) is selected only once, in first-named order.
	var selected []compare.Action
	seen := make(map[int]bool)
	// add selects the 0-based row once, skipping it if already named.
	add := func(i int) {
		if !seen[i] {
			seen[i] = true
			selected = append(selected, actions[i])
		}
	}
	for member := range strings.SplitSeq(response, ",") {
		// Tolerate whitespace between members and around a range's operands, so
		// "1 - 2, 4" reads the same as "1-2,4". (The whole response is already
		// trimmed above; this handles the spaces a user leaves inside the list.)
		member = strings.TrimSpace(member)
		lo, hi, isRange := strings.Cut(member, "-")
		if isRange {
			loN, errLo := strconv.Atoi(strings.TrimSpace(lo))
			hiN, errHi := strconv.Atoi(strings.TrimSpace(hi))
			if errLo != nil || errHi != nil || loN < 1 || loN > hiN || hiN > len(actions) {
				return nil, fmt.Errorf("unrecognized selection: %q", response)
			}
			for i := loN; i <= hiN; i++ {
				add(i - 1)
			}
			continue
		}
		m, convErr := strconv.Atoi(member)
		if convErr != nil || m < 1 || m > len(actions) {
			return nil, fmt.Errorf("unrecognized selection: %q", response)
		}
		add(m - 1)
	}
	return selected, nil
}
