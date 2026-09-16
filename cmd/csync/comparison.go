// comparison.go runs the compare phase and reports how it ended, leaving the
// orchestration to decide what each ending is worth as an exit status.

package main

import (
	"context"
	"errors"

	"github.com/dpassarelli/cherry-sync/internal/command"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/view"
)

// comparison is how a compare phase ended: the changes it found, and whether it
// stopped on the user's interruption or on the deadline rather than by finishing.
// Both are reported as data rather than acted on here, so the caller keeps the
// decision about what to say and which status to exit with.
type comparison struct {
	Result    compare.Result
	Cancelled bool
	TimedOut  bool
}

// runComparison compares source against destination, behind a spinner when there
// is a terminal to show one on. This is the long wait — rsync content-hashes every
// candidate on both ends (--checksum) — and running it with nothing on screen reads
// as a hang rather than as work (#62). Piped, there is nobody to show a spinner to,
// so it runs plain and reports nothing.
//
// The comparison is bounded as well as cancellable (#53). csync is an interactive
// tool: a comparison still running after a minute has stopped being something anyone
// waits through, and there is deliberately no flag to raise the ceiling — a tree that
// slow to compare belongs to rsync, not to csync. The bound covers the piped path
// too, where there is no spinner to show the wait and no Ctrl-C to fall back on.
func runComparison(ctx context.Context, cancel context.CancelFunc, runner *command.Runner, source, destination string, interactive bool) (comparison, error) {
	compareCtx, compareDone := context.WithTimeout(ctx, compareTimeout)
	defer compareDone()

	// Read on the way out, before the deferred compareDone fires: once it does,
	// Err() reports Canceled and an expired deadline is no longer distinguishable
	// from an ordinary cancel.
	timedOut := func() bool { return errors.Is(compareCtx.Err(), context.DeadlineExceeded) }

	if !interactive {
		result, err := compare.Run(compareCtx, runner, source, destination, nil)
		return comparison{Result: result, TimedOut: timedOut()}, err
	}

	phases := make(chan string, 4)
	report := func(stage string) {
		// Never let a caption block the comparison: if the spinner isn't reading,
		// the work continuing matters more than the label arriving.
		select {
		case phases <- stage:
		default:
		}
	}
	spun, err := view.RunSpinner(cancel, phases, func() (compare.Result, error) {
		defer close(phases)
		return compare.Run(compareCtx, runner, source, destination, report)
	})
	if spun.Cancelled {
		// Drain until the comparison closes the channel, which it does only once
		// rsync has actually exited. Without this csync would return while the
		// signalled process was still winding down, and the escalation that
		// guarantees it dies would go with it.
		for range phases {
		}
	}
	return comparison{Result: spun.Result, Cancelled: spun.Cancelled, TimedOut: timedOut()}, err
}
