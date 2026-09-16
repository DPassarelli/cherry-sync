// transfer.go prepares the transfer phase and accounts for it: the split of a
// selection into the two passes rsync needs, and the report of a pass that failed.

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
	"github.com/dpassarelli/cherry-sync/internal/view"
)

// reportTransferFailure prints the right account of a failed transfer and returns
// the exit status for it. A stall gets csync's own notice: rsync's version of it
// is an io-timeout line and a numeric code, which names the symptom and not the
// problem. Every other failure is still reported as rsync described it, since
// rsync is the one that knows what went wrong.
func reportTransferFailure(err error, stall time.Duration) int {
	if errors.Is(err, transfer.ErrStalled) {
		fmt.Fprint(os.Stderr, view.Stalled(stall))
		return 1
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
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
