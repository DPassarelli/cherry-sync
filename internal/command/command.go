// Package command runs external programs (rsync, git) for the rest of csync and
// reports each invocation to a Recorder. It is the single place csync shells out:
// centralizing it means the no-shell rule, the run-log's record of "what csync
// actually invoked", and the cancellation policy are enforced once, not at every
// call site. It never learns who the Recorder is — runlog satisfies the interface,
// tests pass a no-op — so the packages that run commands stay ignorant of the log.
package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// termGrace is how long a cancelled command is given to wind up on its own before
// csync stops waiting on it. rsync answers SIGTERM by unwinding and reporting exit
// code 20, so a cancelled run lands in the log as a deliberate stop rather than as
// a command that simply vanished. The grace is short because it is paid on every
// cancellation, and a command still ignoring SIGTERM two seconds later is not going
// to honor it at all.
const termGrace = 2 * time.Second

// Execution is the record of one external command that ran: what it was, how it
// was invoked, and how it turned out. The argument vector is kept as a slice, not
// a joined string, so a reader (and the run log) can tell where one operand ends
// and the next begins even when a path contains a space. Stderr rides along
// because an exit code alone cannot be diagnosed after the fact — 255 is ssh's
// code passed through rsync, worn identically by a refused key, a changed host key
// and an unreachable host — and the run that most needs explaining is over by the
// time anyone reads about it.
type Execution struct {
	Name     string
	Args     []string
	ExitCode int
	Duration time.Duration
	Stderr   []byte
	Err      error
}

// Error is what a Runner returns for a command that ran and failed: the program's
// own account of the failure, carried with the exit error rather than left in the
// Output for each caller to re-attach. Centralizing it here is what keeps a caller
// from reporting a bare exit code by omission — the failure and its explanation
// are one value, and dropping the explanation now takes deliberate effort.
//
// Both streams are kept because rsync divides its diagnostics between them: the
// itemized listing goes to stdout while errors go to stderr, and a failure part way
// through leaves the reason for it on the far side of that split.
type Error struct {
	Name   string
	Stdout []byte
	Stderr []byte
	Err    error
}

// Error renders the failure as the program described it, behind the program's name
// and the exit status: "rsync: exit status 23: rsync: [sender] change_dir …".
//
// Stderr is what gets shown, because that is where a program explains itself and
// stdout is the work it was doing — the comparison runs rsync at -vv, whose trace
// would otherwise push the one actionable line out of sight. Stdout stands in only
// when stderr is empty, so a program that fails talking solely to stdout is still
// quoted rather than reduced to a bare exit code.
func (e *Error) Error() string {
	said := e.Stderr
	if len(said) == 0 {
		said = e.Stdout
	}
	return fmt.Sprintf("%s: %v: %s", e.Name, e.Err, said)
}

// Unwrap returns the underlying failure, so errors.Is and errors.As still reach the
// *exec.ExitError a caller may want to inspect.
func (e *Error) Unwrap() error { return e.Err }

// Recorder receives each Execution a Runner runs. runlog.Log satisfies it; a test
// passes a no-op. Recording is best-effort — a Runner drops the returned error, so
// a log that cannot be written never fails the command it was describing.
type Recorder interface {
	Record(Execution) error
}

// Output holds what a command wrote, each stream captured on its own so a caller
// can use whichever it needs — the comparison reads stdout, a failed transfer
// reports stderr — without the Runner having to choose between them.
type Output struct {
	Stdout []byte
	Stderr []byte
}

// watcher is the stderr sink for a RunUntil call. It captures the stream as an
// ordinary buffer would and, the first time the text captured so far contains one
// of its phrases, stops the command. Matching against everything accumulated
// rather than against the write in hand is what lets a phrase that straddles two
// writes still count: nothing guarantees a program writes a whole line at once, or
// that a pipe delivers it in one piece.
type watcher struct {
	buf     bytes.Buffer
	says    []string
	stop    context.CancelFunc
	stopped bool
}

// Write captures p and stops the command if the stream now says one of the
// phrases. It is called only from the goroutine os/exec copies the pipe with, so
// the buffer has a single writer; the Runner reads it after Wait has returned.
func (w *watcher) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if w.stopped || len(w.says) == 0 {
		return n, err
	}
	for _, phrase := range w.says {
		if bytes.Contains(w.buf.Bytes(), []byte(phrase)) {
			w.stopped = true
			w.stop()
			break
		}
	}
	return n, err
}

// Runner runs external commands and reports each to its Recorder.
type Runner struct {
	rec Recorder
}

// New returns a Runner that reports every command it runs to rec.
func New(rec Recorder) *Runner {
	return &Runner{rec: rec}
}

// Run executes name with args — never through a shell, so shell metacharacters in
// a path reach the program as inert bytes — feeding stdin (nil for none), and
// returns what it wrote. It times the call, reports the Execution, and returns the
// program's error unwrapped so the caller can frame it. The Execution is reported
// whether the command succeeded or failed: a run worth troubleshooting is usually
// one that failed, so its record must not be the one that goes missing.
//
// Cancelling ctx stops the command with SIGTERM rather than Go's default SIGKILL,
// escalating only if it is ignored. Only the command itself is signalled, not any
// process it spawned: rsync's ssh child exits on its own when rsync goes away
// (verified against rsync 3.4.1 for both SIGTERM and SIGKILL), and signalling the
// process group instead would take rsync out of the terminal's foreground group,
// where a Ctrl-C on a non-interactive run would no longer reach it.
func (r *Runner) Run(ctx context.Context, name string, args []string, stdin io.Reader) (Output, error) {
	return r.RunUntil(ctx, name, args, stdin, nil)
}

// RunUntil runs a command as Run does, but stops waiting on it as soon as its
// stderr says one of stderrSays. It exists because a command can announce that it
// has given up and then fail to exit: openrsync reports "poll: timeout" the moment
// its --timeout elapses, then blocks in waitpid on a remote shell that is never
// coming back (verified on macOS 15), so a stalled transfer would hang csync for
// as long as the remote stayed silent. Stopping the command follows the same
// SIGTERM-then-kill path a cancellation does.
//
// The stderr that triggered the stop is returned with the rest, so the caller can
// still say what went wrong, and the caller's own context is left untouched — a
// command stopped this way must not be reported as the user's cancellation.
func (r *Runner) RunUntil(ctx context.Context, name string, args []string, stdin io.Reader, stderrSays []string) (Output, error) {
	// A context of csync's own, so stopping the command on a phrase cannot be
	// mistaken for the caller cancelling the run.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	// #nosec G204 -- callers pass a fixed program name and an argument vector built
	// without a shell; every variable path operand is guarded by a `--` separator
	// (see compare/transfer) and validated upstream. See SECURITY.md.
	cmd := exec.CommandContext(runCtx, name, args...)
	var stdout bytes.Buffer
	stderr := &watcher{says: stderrSays, stop: stop}
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}

	// WaitDelay is what makes the SIGTERM above safe to prefer. It bounds two ways
	// a cancelled command can otherwise hang csync indefinitely: a process that
	// ignores the signal (which it then kills outright), and a process that exits
	// leaving a child of its own holding the inherited output pipes, which Wait
	// would otherwise read until an EOF that never comes.
	cmd.WaitDelay = termGrace

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	// ProcessState is nil only when the program never started (not found, not
	// executable); -1 stands for "did not run" and is distinct from any real exit.
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	out := Output{Stdout: stdout.Bytes(), Stderr: stderr.buf.Bytes()}
	_ = r.rec.Record(Execution{Name: name, Args: args, ExitCode: code, Duration: dur, Stderr: out.Stderr, Err: err})

	// A cancelled command fails with whatever the signal produced ("signal:
	// terminated"), which says nothing about why csync stopped it. Report the
	// context's reason instead, so a caller can tell a genuine rsync failure from a
	// deadline or a user's Ctrl-C.
	if err != nil && ctx.Err() != nil {
		return out, fmt.Errorf("%s: %w", name, ctx.Err())
	}
	if err != nil {
		return out, &Error{Name: name, Stdout: out.Stdout, Stderr: out.Stderr, Err: err}
	}

	return out, nil
}
