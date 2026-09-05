// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer, tui) that do the work.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/command"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/config"
	"github.com/dpassarelli/cherry-sync/internal/license"
	"github.com/dpassarelli/cherry-sync/internal/operand"
	"github.com/dpassarelli/cherry-sync/internal/runlog"
	"github.com/dpassarelli/cherry-sync/internal/selection"
	"github.com/dpassarelli/cherry-sync/internal/transfer"
	"github.com/dpassarelli/cherry-sync/internal/view"
)

// version is csync's release version, injected at build time via
// `-ldflags "-X main.version=…"` (see .goreleaser.yaml). It defaults to "dev"
// for un-injected builds — `go build ./cmd/csync`, `go run`, the test harness —
// which the version line renders as "cherry-sync (dev build)".
var version = "dev"

// compareTimeout is how long a comparison may run before csync stops waiting on
// it (#53). Sixty seconds is the interaction budget: past it the tool is no
// longer doing anything a person is present for, and an unresponsive remote or a
// hung SSH connection would otherwise hang csync indefinitely with no recourse
// short of killing it. It is 59 rather than 60 so the spinner's own count never
// reaches three digits.
const compareTimeout = 59 * time.Second

// defaultStallTimeout is how long a transfer may go without a byte from the
// remote before csync stops waiting on it (#53). Unlike compareTimeout this is a
// silence budget, not an elapsed one: a legitimately large transfer runs as long
// as it needs, and only one that has gone quiet is cut off. Thirty seconds leaves
// room for the receiver to read a very large file while generating its block
// checksums, which is the longest a healthy transfer goes without saying
// anything.
const defaultStallTimeout = 30 * time.Second

// stallTimeoutVar is the environment variable that overrides defaultStallTimeout,
// in whole seconds. It is deliberately undocumented: it exists so the acceptance
// suite can prove the bound without waiting out the real one on every run, not as
// a knob for tuning csync, which has no more business being configurable here
// than the comparison's limit does.
const stallTimeoutVar = "CSYNC_STALL_TIMEOUT"

// stallTimeout returns the silence budget for this run: defaultStallTimeout,
// unless stallTimeoutVar names a positive whole number of seconds. Anything else
// — unset, unparseable, zero, negative — falls back to the default rather than
// failing the run, because a malformed test hook must never be able to leave a
// transfer unbounded, which is the very thing the bound exists to prevent.
func stallTimeout() time.Duration {
	raw := os.Getenv(stallTimeoutVar)
	if raw == "" {
		return defaultStallTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultStallTimeout
	}
	return time.Duration(seconds) * time.Second
}

// main runs csync and exits with the status it reports. It holds no logic of its
// own: os.Exit skips deferred functions, so confining it to this one line is what
// lets run own the resources — the run log above all — and release them on every
// path out.
func main() {
	os.Exit(run())
}

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

	// --help prints the usage summary and exits before any operand resolution. It
	// short-circuits in cli.Parse, so any trailing operands are ignored. The
	// summary goes to stdout — it is requested output, not a diagnostic.
	if a.Mode == cli.Help {
		fmt.Println(view.Usage(version))
		return 0
	}

	// --version reports the build and exits before any operand resolution or
	// comparison. It short-circuits in cli.Parse, so any trailing operands are
	// ignored. The line goes to stdout — it is requested output, not a diagnostic.
	if a.Mode == cli.Version {
		fmt.Println(view.VersionReport(version))
		return 0
	}

	// --license prints the embedded MIT text and exits, so a distributed bare
	// binary carries its own notice (nothing bundled alongside it required). Like
	// --version it short-circuits in cli.Parse, ignoring any trailing operands.
	// The text ends in a newline, so Print — not Println — avoids a trailing blank.
	if a.Mode == cli.License {
		fmt.Print(license.Text())
		return 0
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
			return 1
		}
		switch a.Mode {
		case cli.Push:
			source, destination = ".", cfg.Remote
		case cli.Pull:
			source, destination = cfg.Remote, "."
		}
	}

	// Normalize the operands before anything reads them — the header echo, the
	// compare, and the transfer must all see the same shape, whether the operand
	// came from argv or .csync.toml. The load-bearing case is a remote path with a
	// leading "~": modern rsync passes it literally, so `host:~/x` resolves to
	// `/home/user/~/x` and the transfer fails with exit 12 (#50). Normalize
	// resolves it to a relative path (rsync interprets that against the login home)
	// and reports the rewrite so it can be disclosed below. A `~user` form has no
	// relative equivalent, so Normalize errors here rather than letting rsync fail
	// confusingly mid-transfer.
	srcN, err := operand.Normalize(source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dstN, err := operand.Normalize(destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	source, destination = srcN.Path, dstN.Path

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
	// The header discloses any operand csync rewrote inline: srcN.From/dstN.From
	// carry the original path portion when a remote "~" was resolved, which Header
	// renders as a faint "(rewritten from …)" beside the value, so the change isn't
	// silent. Trailing-slash collapse needs no note — the header already shows the
	// cleaned path.
	printAbove(view.Header(
		view.Endpoint{Path: source, From: srcN.From},
		view.Endpoint{Path: destination, From: dstN.From},
	))

	// The comparison is the long wait: rsync content-hashes every candidate on both
	// ends (--checksum), and until now it ran with nothing on screen, which reads as
	// a hang rather than as work (#62). On a terminal it runs behind a spinner that
	// names the stage it is in; piped, there is nobody to show it to, so it runs
	// plain and reports nothing.
	//
	// The context is what stops it. While the spinner holds the terminal in raw
	// mode a Ctrl-C never becomes a SIGINT, so cancelling here is what passes the
	// user's interruption on to the rsync that is still running.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The comparison is bounded as well as cancellable (#53). csync is an
	// interactive tool: a comparison still running after a minute has stopped being
	// something anyone waits through, and there is deliberately no flag to raise the
	// ceiling — a tree that slow to compare belongs to rsync, not to csync. The bound
	// covers the piped path too, where there is no spinner to show the wait and no
	// Ctrl-C to fall back on.
	compareCtx, compareDone := context.WithTimeout(ctx, compareTimeout)
	defer compareDone()

	var result compare.Result
	if interactive {
		phases := make(chan string, 4)
		report := func(stage string) {
			// Never let a caption block the comparison: if the spinner isn't reading,
			// the work continuing matters more than the label arriving.
			select {
			case phases <- stage:
			default:
			}
		}
		var comparison view.Comparison
		comparison, err = view.RunSpinner(cancel, phases, func() (compare.Result, error) {
			defer close(phases)
			return compare.Run(compareCtx, runner, source, destination, report)
		})
		result = comparison.Result
		if comparison.Cancelled {
			// Drain until the comparison closes the channel, which it does only once
			// rsync has actually exited. Without this csync would return while the
			// signalled process was still winding down, and the escalation that
			// guarantees it dies would go with it.
			for range phases {
			}
			fmt.Print(view.Canceled())
			return 0
		}
	} else {
		result, err = compare.Run(compareCtx, runner, source, destination, nil)
	}
	if err != nil {
		// An expired deadline outranks whatever error rsync's death produced: killed
		// mid-run it reports the signal that killed it, which explains nothing.
		if errors.Is(compareCtx.Err(), context.DeadlineExceeded) {
			fmt.Fprint(os.Stderr, view.TimedOut(compareTimeout))
			return 1
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Record the change list csync classified, before it asks what to sync — so a run
	// abandoned at the prompt still shows what was on offer. The selection is recorded
	// separately once it is made; the two are kept apart because they can differ.
	_ = runLog.Classified(logActions(result.Actions))

	// Record what was held out of the comparison too — the gitignored paths by name, the
	// .git directory, the .csync.toml — so a file that never appears in the change list
	// can still be accounted for. This is the same set the header discloses, named
	// rather than counted.
	_ = runLog.Excluded(result.Excluded, result.GitDirExcluded, result.CsyncTomlExcluded)

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
	if n := len(result.Excluded); n > 0 {
		noun := "paths"
		if n == 1 {
			noun = "path"
		}
		excluded = append(excluded, fmt.Sprintf("%d gitignored %s", n, noun))
	}
	printAbove(view.Excluded(excluded))

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

	// Split the selection into transfers (create/update) and removals (delete):
	// rsync moves files with one mechanism (--files-from) and removes them with
	// another (a --delete filter pass), so each verb goes to its own call.
	var transferPaths, removePaths []string
	for _, act := range selected {
		if act.Verb == "delete" {
			removePaths = append(removePaths, act.Path)
		} else {
			transferPaths = append(transferPaths, act.Path)
		}
	}
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

// logActions adapts the compare package's actions to the run log's own Action type,
// bridging the two so runlog need not depend on compare. It is the one place the shape
// is translated for the classified and selected records.
func logActions(actions []compare.Action) []runlog.Action {
	out := make([]runlog.Action, len(actions))
	for i, a := range actions {
		out[i] = runlog.Action{Verb: a.Verb, Path: a.Path}
	}
	return out
}

// interactiveTerminal reports whether csync is attached to a real terminal on both
// ends — stdin (to read keys) and stdout (to render). Only then is the Bubble Tea
// picker usable; a piped or redirected run, including the test harness, takes the
// typed-grammar fallback instead.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
