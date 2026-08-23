// Package command runs external programs (rsync, git) for the rest of csync and
// reports each invocation to a Recorder. It is the single place csync shells out:
// centralizing it means the no-shell rule and the run-log's record of "what csync
// actually invoked" are enforced once, not at every call site. It never learns who
// the Recorder is — runlog satisfies the interface, tests pass a no-op — so the
// packages that run commands stay ignorant of the log.
package command

import (
	"bytes"
	"io"
	"os/exec"
	"time"
)

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
func (r *Runner) Run(name string, args []string, stdin io.Reader) (Output, error) {
	// #nosec G204 -- callers pass a fixed program name and an argument vector built
	// without a shell; every variable path operand is guarded by a `--` separator
	// (see compare/transfer) and validated upstream. See SECURITY.md.
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}
