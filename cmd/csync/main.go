// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer, tui) that do the work.
package main

import (
	"fmt"
	"os"
	"strings"

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
	runLog, err := runlog.Create()
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

	// Record which build made this run, up front, so a run abandoned at the prompt
	// still names its binary. A failed record write can't fail the sync — the log is
	// a diagnostic, never a precondition — so the error is deliberately dropped; the
	// unwritable-log case is already surfaced when Create fell back to Discard above.
	_ = runLog.Version(version)

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

	result, err := compare.Run(runner, source, destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
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
	printAbove(view.Excluded(excluded))

	if len(result.Actions) == 0 {
		// Nothing to do — stop before any selection UI. The non-interactive path
		// still leads with the machine-readable "Changes: 0" line it always prints;
		// the interactive path just states it plainly.
		if !interactive {
			fmt.Print(view.ChangeList(nil))
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
		picked, err := view.RunPicker(result.Actions, preamble.String())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		// Nothing chosen — a cancel (Ctrl-C/Esc/q) or a confirmed empty selection,
		// which amount to the same thing: report it and stop before running a no-op
		// transfer that would print "Sync complete! (0 files)".
		if len(picked) == 0 {
			fmt.Print(view.Canceled())
			return 0
		}
		selected = picked
	} else {
		fmt.Print(view.ChangeList(result.Actions))
		// Prompt on stderr so stdout stays a clean, parseable report.
		fmt.Fprint(os.Stderr, "Press Enter to sync all changes: ")
		selected, err = selection.SelectActions(os.Stdin, result.Actions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
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
	err = transfer.Run(source, destination, transferPaths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = transfer.Remove(source, destination, removePaths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Report what moved with the post-sync summary: the past-tense list of changes.
	// csync is human-first in both modes — the non-TTY path is the degraded-but-still-
	// human fallback, not a machine interface — so it gets the same summary, only
	// without color (lipgloss drops ANSI when stdout isn't a terminal).
	fmt.Print(view.RenderSummary(selected))
	return 0
}

// interactiveTerminal reports whether csync is attached to a real terminal on both
// ends — stdin (to read keys) and stdout (to render). Only then is the Bubble Tea
// picker usable; a piped or redirected run, including the test harness, takes the
// typed-grammar fallback instead.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
