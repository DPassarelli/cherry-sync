// Steps for the interactive selection prompt: starting csync and holding it at
// the prompt, answering it, and asserting what happens either side of the answer.

package acceptance_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// theChangedFileIsDeletedBeforeIAnswer removes the local file the scenario changed,
// while csync sits at the prompt with the comparison already done. The transfer is
// then told to send a file that is no longer there and stops with a partial-transfer
// error, which is a failure on the far side of the prompt rather than before it.
//
// Deleting rather than chmod-ing, for the reason thatCsyncCannotWriteItsLog gives:
// root ignores permission bits, so a setup built on them stops testing anything the
// day this suite runs as root. Nothing gets to read a file that is gone.
func theChangedFileIsDeletedBeforeIAnswer(ctx context.Context) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return fmt.Errorf("local path not set; missing Background step?")
	}
	changed, _ := ctx.Value(changedFileKey{}).(string)
	if changed == "" {
		return fmt.Errorf("no file was changed locally, so there is none to delete")
	}
	err := os.Remove(filepath.Join(local, changed))
	if err != nil {
		return fmt.Errorf("deleting the changed file: %w", err)
	}
	return nil
}

// iHaveStartedCsyncButNotYetAnsweredThePrompt starts csync and returns once it is
// blocked on stdin at the selection prompt, leaving it there for the steps that
// follow. The operands are not in the Gherkin: the scenarios that use this step
// are about the run log, not about how csync is invoked, so the command line is an
// implementation detail of the step rather than a fact of the scenario.
//
// Waiting for the prompt to appear on stderr is what makes the observation that
// follows deterministic. csync writes it after the comparison and before any
// transfer, then blocks — so the process is suspended, not merely slow, and a step
// that reads the log now cannot be racing a csync that is still writing it.
func iHaveStartedCsyncButNotYetAnsweredThePrompt(ctx context.Context) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	remote := resolvedRemote(ctx)
	if remote == "" {
		return ctx, fmt.Errorf("remote path not set; missing a step that creates one?")
	}

	cmd := exec.Command(csyncBinary, local, remote)
	cmd.Env = csyncEnv(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ctx, fmt.Errorf("stdin pipe: %w", err)
	}
	run := &runningCsync{cmd: cmd, stdin: stdin, stdout: &syncBuffer{}, stderr: &syncBuffer{}, done: make(chan error, 1)}
	cmd.Stdout = run.stdout
	cmd.Stderr = run.stderr
	err = cmd.Start()
	if err != nil {
		return ctx, fmt.Errorf("start csync: %w", err)
	}
	go func() { run.done <- cmd.Wait() }()
	// Stash before waiting: if the prompt never comes, the After hook still has the
	// child to kill.
	ctx = context.WithValue(ctx, startedKey{}, run)

	deadline := time.Now().Add(promptWait)
	for !strings.Contains(run.stderr.String(), selectionPrompt) {
		select {
		case waitErr := <-run.done:
			// csync exited rather than asking. Report that, not the timeout it would
			// otherwise become: a scenario whose csync died has a different problem
			// from one whose csync hung, and the two should not read alike.
			run.reaped = true
			return ctx, fmt.Errorf("csync exited (%v) before reaching the selection prompt\nstdout:\n%s\nstderr:\n%s",
				waitErr, run.stdout.String(), run.stderr.String())
		default:
		}
		if time.Now().After(deadline) {
			return ctx, fmt.Errorf("csync never reached the selection prompt within %s\nstdout:\n%s\nstderr:\n%s",
				promptWait, run.stdout.String(), run.stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ctx, nil
}

// iLocateTheLogFile finds the run log under the scenario's XDG_STATE_HOME and
// stashes its path. It backs both `When I look for the log file` and `Given I have
// taken note of where the log file is`.
//
// It searches rather than being told, because the scenarios must not name the
// path: csync has not yet disclosed it (that happens as csync exits) and where it
// belongs is pinned by the location scenario alone. Insisting on exactly one match
// is what makes the search an honest stand-in for the disclosure — with two logs
// under the state directory, "the log file" would not mean anything.
func iLocateTheLogFile(ctx context.Context) (context.Context, error) {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, foundLogKey{}, log), nil
}

// theLogFileShouldAlreadyHaveContent asserts the log is not empty at the moment
// csync is blocked at the prompt. Content, not existence: csync opens the file as
// it starts, so an empty one proves only that it was created. Bytes on disk here
// prove the records were written as the run proceeded — a log assembled in memory
// and flushed on the way out would be empty at this moment, and would be lost
// entirely on the abnormal exits the log exists to survive.
func theLogFileShouldAlreadyHaveContent(ctx context.Context) error {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		return fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("run log at %q: %w", path, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("run log at %q is empty while csync waits at the prompt; nothing has been written to disk yet", path)
	}
	return nil
}

// iAnswerThePrompt sends a bare Enter to the waiting csync — accept every change —
// and waits for it to finish, stashing its streams and exit code where the ordinary
// Then steps read them.
func iAnswerThePrompt(ctx context.Context) (context.Context, error) {
	run, _ := ctx.Value(startedKey{}).(*runningCsync)
	if run == nil {
		return ctx, fmt.Errorf("csync was not started; missing a step that starts it?")
	}
	_, err := io.WriteString(run.stdin, "\n")
	if err != nil {
		return ctx, fmt.Errorf("answering the prompt: %w", err)
	}
	err = run.stdin.Close()
	if err != nil {
		return ctx, fmt.Errorf("closing stdin: %w", err)
	}

	exitCode := 0
	err = <-run.done
	run.reaped = true
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return ctx, fmt.Errorf("waiting for csync: %w", err)
		}
		exitCode = exitErr.ExitCode()
	}
	result := runResult{Stdout: run.stdout.String(), Stderr: run.stderr.String(), ExitCode: exitCode}
	return context.WithValue(ctx, outputKey{}, result), nil
}

// csyncShouldExitNormally asserts csync ran to completion rather than falling over
// partway. It is `exit code 0` said in the vocabulary of a scenario that cares only
// that the run finished, since a csync that died has nothing to report about a log.
func csyncShouldExitNormally(ctx context.Context) error {
	return csyncShouldReturnExitCode(ctx, 0)
}

// theReportedLogPathShouldBeTheOneIFoundEarlier ties the log csync discloses on the
// way out to the file it was seen filling in mid-run. Without it the two halves are
// each honest and jointly useless: csync could write one file and name another.
func theReportedLogPathShouldBeTheOneIFoundEarlier(ctx context.Context) error {
	want, _ := ctx.Value(foundLogKey{}).(string)
	if want == "" {
		return fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).LogPath
	if got != want {
		return fmt.Errorf("csync reported log path %q, but the log it was writing is %q", got, want)
	}
	return nil
}
