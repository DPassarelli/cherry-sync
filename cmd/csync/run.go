// run.go holds csync's orchestration: the single pass from parsed arguments
// through comparison and selection to the transfer, and the account it gives of
// a transfer that failed.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/command"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/runlog"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
	"github.com/dpassarelli/cherry-sync/internal/view"
)

// run parses the command-line arguments, runs the dry-run comparison, asks which
// changes to sync — through the interactive picker on a terminal, or the typed
// prompt otherwise — transfers the chosen files, and returns the process exit
// status. Every failure leaves through a `return` so the deferred disclosure of
// the run log's path happens on the runs that fail as much as on the ones that
// succeed; those are the runs worth reading.
func run() (code int) {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		// State the specific problem cli.Parse diagnosed, then point at --help for
		// the full usage rather than dumping it here — a lost user gets a one-line
		// reason and a next step. The reason goes to stderr; it is a diagnostic.
		fmt.Fprintf(os.Stderr, "ERROR: %s\nRun 'csync --help' for usage.\n", err)
		return 2
	}

	infoCode, handled := infoMode(a)
	if handled {
		return infoCode
	}

	ops, err := resolveOperands(a)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	source, destination := ops.Source, ops.Destination

	// Open this run's log before any work happens. --version, --license, and a
	// usage error have all returned by now: they run no rsync, so they leave
	// nothing to troubleshoot and must not litter the state directory.
	//
	// A log that cannot be opened warns and is replaced by one that records nothing.
	// The record is a diagnostic, never a precondition: a state directory gone
	// read-only says nothing about whether the files should move, and a tool that
	// refuses to work because it cannot keep a diary is a tool nobody keeps. It
	// warns rather than declining silently, because a user hunting the record of a
	// destructive run must not be left wondering whether csync skipped it or they
	// misremembered where it goes.
	runLog, err := runlog.Create(version)
	notLogged := ""
	if err != nil {
		notLogged = err.Error()
		fmt.Fprintf(os.Stderr, "warning: this run will not be logged (%v)\n", err)
		runLog = runlog.Discard()
	}

	// Every run ends by accounting for its record: where it was written, or why it
	// was not. Deferring that is the point of the os.Exit(run()) shape — the
	// accounting cannot be forgotten at a new exit, and it costs the interactive
	// picker no row of the terminal it holds above its scroll region. A failed run
	// says so on stderr, beside the error the log would have explained; a clean one
	// on stdout with the rest of the report.
	//
	// The unlogged run repeats here what the warning above already said, because a
	// long change list scrolls that warning off the top and this is where someone
	// who wanted the record will look. It carries its own label rather than an empty
	// "Log written to", so nothing sends the user after a file that was never created.
	defer func() {
		out := os.Stdout
		if code != 0 {
			out = os.Stderr
		}
		path := runLog.Path()
		if path == "" {
			fmt.Fprint(out, view.NotLogged(notLogged))
			return
		}
		fmt.Fprint(out, view.LogPath(path))
	}()

	// Registered last, so it runs first: the log is closed before csync points at
	// it. Failing to close cannot fail the sync — every record is already on disk,
	// each written with its own syscall rather than buffered — so the descriptor is
	// all that is being given back here.
	defer func() { _ = runLog.Close() }()

	// Record the literal invocation, up front, so the log opens with the command as
	// run — what the user actually typed, the raw args before any resolution.
	// filepath.Base trims the arg0 path down to "csync". A failed record write can't
	// fail the sync — the log is a diagnostic, never a precondition — so the error is
	// deliberately dropped; the unwritable-log case is already surfaced when Create
	// fell back to Discard above.
	_ = runLog.Invocation(filepath.Base(os.Args[0]), os.Args[1:])

	// Then the operands: what csync compared and which way the sync went, the frame
	// the rest of the log hangs on. These are the normalized paths the header echoes
	// and the comparison uses, so the log agrees with what the user saw — and under a
	// saved-target push/pull they are the resolved paths the invocation above does not
	// show. (The version heads the log already — Create wrote it.)
	_ = runLog.Operands(source, destination)

	// Every external command csync runs goes through this runner, which reports each
	// invocation to the run log. A discarding log is a valid recorder that keeps
	// nothing, so there is no separate unlogged path to maintain here.
	runner := command.New(runLog)

	// Detect the terminal once: it both selects the selection front-end (picker vs.
	// typed prompt) and gates the decorative banner, which only an interactive run
	// shows — piped output stays clean.
	interactive := interactiveTerminal()

	// Accumulate everything printed above the picker so it can hold those rows out of
	// its scroll region and they don't scroll off the top. The picker measures the
	// rows itself at the live terminal width (wrapping a long remote path counts for
	// every row it takes); the typed-prompt path ignores the preamble.
	var preamble strings.Builder
	printAbove := func(s string) {
		fmt.Print(s)
		preamble.WriteString(s)
	}

	if interactive {
		printAbove(view.Banner(version))
	}
	// The header discloses any operand csync rewrote inline: the From fields carry
	// the original path portion when a remote "~" was resolved, which Header renders
	// as a faint "(rewritten from …)" beside the value, so the change isn't silent.
	// Trailing-slash collapse needs no note — the header already shows the cleaned path.
	printAbove(view.Header(
		view.Endpoint{Path: source, From: ops.SourceFrom},
		view.Endpoint{Path: destination, From: ops.DestinationFrom},
	))

	// This context is what a Ctrl-C travels down. While the spinner holds the
	// terminal in raw mode a Ctrl-C never becomes a SIGINT, so cancelling here is
	// what passes the user's interruption on to the rsync that is still running.
	// The transfer below runs under the same context, so it is interruptible too.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmp, err := runComparison(ctx, cancel, runner, source, destination, interactive)
	// A cancelled comparison outranks any error it ended with: the user asked it to
	// stop, so stopping is the outcome, not a failure to report.
	if cmp.Cancelled {
		fmt.Print(view.Canceled())
		return 0
	}
	if err != nil {
		// An expired deadline outranks whatever error rsync's death produced: killed
		// mid-run it reports the signal that killed it, which explains nothing.
		if cmp.TimedOut {
			fmt.Fprint(os.Stderr, view.TimedOut(compareTimeout))
			return 1
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	result := cmp.Result

	// Record the change list csync classified, before it asks what to sync — so a run
	// abandoned at the prompt still shows what was on offer. The selection is recorded
	// separately once it is made; the two are kept apart because they can differ.
	_ = runLog.Classified(logActions(result.Actions))

	// Record what was held out of the comparison too — the gitignored paths by name, the
	// .git directory, the .csync.toml — so a file that never appears in the change list
	// can still be accounted for. This is the same set the header discloses, named
	// rather than counted.
	_ = runLog.Excluded(result.Excluded, result.GitDirExcluded, result.CsyncTomlExcluded)

	printAbove(view.Excluded(excludedNotice(result)))
	// A gitignored path that WOULD have moved is the only exclusion a user gets
	// surprised by: the file changed, so its absence from the change list looks like
	// csync missing it rather than honoring .gitignore. Name those changes here, above
	// the list, so the absence is accounted for where it is noticed.
	printAbove(view.Withheld(result.Withheld))

	if len(result.Actions) == 0 {
		// Nothing to do — stop before any selection UI. The non-interactive path
		// still leads with the machine-readable "Changes: 0" line it always prints;
		// the interactive path just states it plainly.
		if !interactive {
			fmt.Print(view.ChangeList(nil, time.Now()))
		}
		fmt.Println("\nNo changes to sync.")
		return 0
	}

	// Pick the selection front-end by whether we're attached to a terminal on both
	// ends. With a real terminal, present the Bubble Tea picker; otherwise (piped,
	// redirected, or under the test harness) print the plain change list and read
	// the typed-grammar response from stdin. The picker renders its own list, so the
	// "Changes:" report is non-interactive-only.
	var selected []compare.Action
	if interactive {
		var picked []compare.Action
		if len(result.Actions) > view.LargeSetLimit {
			// Past this many changes csync stops offering a choice (#61). A list this
			// long is not something anyone reviews a row at a time, so rather than
			// render one, it states its design limit and offers the whole set or
			// nothing. Enter here means every change, which is why the gate accepts
			// nothing else.
			proceed, gateErr := view.RunLargeSetGate()
			if gateErr != nil {
				fmt.Fprintln(os.Stderr, gateErr)
				return 1
			}
			if proceed {
				picked = result.Actions
			}
		} else {
			chosen, pickErr := view.RunPicker(result.Actions, preamble.String())
			if pickErr != nil {
				fmt.Fprintln(os.Stderr, pickErr)
				return 1
			}
			picked = chosen
		}
		// Record what the user chose, distinct from what was classified above, before
		// the empty-selection stop below — so even a cancelled run records that nothing
		// was taken. Declining the large-set gate lands here too, as an empty selection.
		_ = runLog.Selected(logActions(picked))
		// Nothing chosen — a cancel (Ctrl-C/Esc/q) or a confirmed empty selection,
		// which amount to the same thing: report it and stop before running a no-op
		// transfer that would print "Sync complete! (0 files)".
		if len(picked) == 0 {
			fmt.Print(view.Canceled())
			return 0
		}
		selected = picked
	} else {
		fmt.Print(view.ChangeList(result.Actions, time.Now()))
		// Prompt on stderr so stdout stays a clean, parseable report.
		fmt.Fprint(os.Stderr, "Press Enter to sync all changes: ")
		selected, err = selection.SelectActions(os.Stdin, result.Actions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		// The selection, recorded as its counterpart to the classification above.
		_ = runLog.Selected(logActions(selected))
	}

	transferPaths, removePaths := splitByVerb(selected)
	// Transfers first, removals last: the additive pass is recoverable, so if it
	// fails we exit before deleting anything on the destination.
	stall := stallTimeout()
	err = transfer.Run(ctx, runner, source, destination, transferPaths, stall)
	if err != nil {
		return reportTransferFailure(err, stall)
	}
	err = transfer.Remove(ctx, runner, source, destination, removePaths, stall)
	if err != nil {
		return reportTransferFailure(err, stall)
	}

	// Report what moved with the post-sync summary: the past-tense list of changes.
	// csync is human-first in both modes — the non-TTY path is the degraded-but-still-
	// human fallback, not a machine interface — so it gets the same summary, only
	// without color (lipgloss drops ANSI when stdout isn't a terminal).
	fmt.Print(view.RenderSummary(selected))
	return 0
}
