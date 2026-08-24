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
// and the next begins even when a path contains a space.
type Execution struct {
	Name     string
	Args     []string
	ExitCode int
	Duration time.Duration
	Err      error
}

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
	// #nosec G204 -- callers pass a fixed program name and an argument vector built
	// without a shell; every variable path operand is guarded by a `--` separator
	// (see compare/transfer) and validated upstream. See SECURITY.md.
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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
	_ = r.rec.Record(Execution{Name: name, Args: args, ExitCode: code, Duration: dur, Err: err})

	// A cancelled command fails with whatever the signal produced ("signal:
	// terminated"), which says nothing about why csync stopped it. Report the
	// context's reason instead, so a caller can tell a genuine rsync failure from a
	// deadline or a user's Ctrl-C.
	if err != nil && ctx.Err() != nil {
		return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, fmt.Errorf("%s: %w", name, ctx.Err())
	}

	return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}
