// Package selection holds the pure decision logic behind csync's file picker:
// which changes are selected, and how they are grouped for display. It has no
// rendering and holds no global state, so the interactive (Bubble Tea) and
// non-interactive (typed-grammar) front-ends share one tested core.
package selection

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// SelectActions reads one line of input and returns the actions the user chose.
// A bare Enter or "a" accepts the default (every change); "n" declines and
// selects nothing; otherwise the line is a selection list — comma-separated
// members, each a single 1-based index ("3") or an inclusive range ("1-3") —
// resolved in order. End-of-input with no line at all — a closed or
// non-interactive stdin — also selects nothing, so running csync without
// answering the prompt never transfers anything. Any unrecognized, malformed,
// or out-of-range response is an error.
func SelectActions(r io.Reader, actions []compare.Action) ([]compare.Action, error) {
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
