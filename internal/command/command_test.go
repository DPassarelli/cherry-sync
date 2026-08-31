package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// silent is a Recorder that keeps nothing, so these tests exercise Run without a
// run log.
type silent struct{}

func (silent) Record(Execution) error { return nil }

// script writes an executable shell script and returns its path.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prog.sh")
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700)
	if err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// TestCancelSendsTermBeforeKill pins the choice of signal: a cancelled command
// gets SIGTERM, which rsync answers by unwinding and reporting a real status.
// Sending SIGKILL straight away would make every cancelled run indistinguishable
// from a crash in the log.
func TestCancelSendsTermBeforeKill(t *testing.T) {
	// Reports the signal it received, which it can only do if that signal was
	// catchable — SIGKILL would leave stdout empty. The wait is a loop of short
	// sleeps rather than one long one because a shell defers a trap until the
	// foreground command it is running finishes, which a single `sleep 300` would
	// postpone past the end of the test.
	prog := script(t, "trap 'echo GOT_TERM; exit 0' TERM\nwhile :; do sleep 0.1; done\n")

	ctx, cancel := context.WithCancel(context.Background())

	type outcome struct {
		out Output
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := New(silent{}).Run(ctx, prog, nil, nil)
		done <- outcome{out, err}
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case got := <-done:
		if !strings.Contains(string(got.out.Stdout), "GOT_TERM") {
			t.Errorf("stdout = %q, want it to show the command caught SIGTERM", got.out.Stdout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run never returned after cancellation")
	}
}

// TestCancelEscalatesToKill pins the escalation: a command that ignores SIGTERM
// is given termGrace to exit on its own and is then killed outright, so a stuck
// transfer cannot hold csync open indefinitely.
func TestCancelEscalatesToKill(t *testing.T) {
	prog := script(t, "trap '' TERM\nsleep 300\n")

	ctx, cancel := context.WithCancel(context.Background())
	r := New(silent{})

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, _ = r.Run(ctx, prog, nil, nil)
		done <- time.Since(start)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case elapsed := <-done:
		// It must not die instantly — that would mean SIGTERM was skipped, and a
		// cancelled rsync would never report its own exit status.
		if elapsed < termGrace {
			t.Errorf("command died in %v, before the %v SIGTERM grace elapsed", elapsed, termGrace)
		}
		if elapsed > termGrace+3*time.Second {
			t.Errorf("command took %v to die; escalation to SIGKILL did not happen", elapsed)
		}
	case <-time.After(termGrace + 10*time.Second):
		t.Fatal("Run never returned; a SIGTERM-ignoring command was never killed")
	}
}

// TestCancelReportsContextReason checks that a cancelled command fails with the
// context's reason rather than "signal: terminated", which says nothing about why
// csync stopped it.
func TestCancelReportsContextReason(t *testing.T) {
	prog := script(t, "sleep 300\n")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, err := New(silent{}).Run(ctx, prog, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want an error wrapping context.DeadlineExceeded", err)
	}
}

// TestSuccessfulCommandIsUnaffected guards the common path: the cancellation
// machinery must not change what an ordinary command returns.
func TestSuccessfulCommandIsUnaffected(t *testing.T) {
	prog := script(t, "echo out\necho err >&2\n")

	out, err := New(silent{}).Run(context.Background(), prog, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	gotOut := strings.TrimSpace(string(out.Stdout))
	if gotOut != "out" {
		t.Errorf("stdout = %q, want %q", gotOut, "out")
	}
	gotErr := strings.TrimSpace(string(out.Stderr))
	if gotErr != "err" {
		t.Errorf("stderr = %q, want %q", gotErr, "err")
	}
}

// stallGuard bounds how long these tests wait for RunUntil to return. It is
// generous against the SIGTERM grace a stop has to pay, and short against the
// `sleep 300` the scripts would run for if the phrase were never noticed, so a
// command that is not stopped fails the test rather than the whole suite.
const stallGuard = termGrace + 10*time.Second

// completion is what one RunUntil call produced.
type completion struct {
	out Output
	err error
}

// finish runs body as a script through RunUntil and returns its completion,
// failing the test if RunUntil has not returned within stallGuard — the hang the
// phrase watching exists to prevent.
func finish(t *testing.T, stderrSays []string, body string) completion {
	t.Helper()
	prog := script(t, body)

	done := make(chan completion, 1)
	go func() {
		out, err := New(silent{}).RunUntil(context.Background(), prog, nil, nil, stderrSays)
		done <- completion{out: out, err: err}
	}()

	select {
	case got := <-done:
		return got
	case <-time.After(stallGuard):
		t.Fatalf("RunUntil did not return within %v", stallGuard)
		return completion{}
	}
}

// TestStderrPhraseStopsTheCommand pins the bound csync's stalled-transfer
// guarantee rests on. openrsync reports "poll: timeout" the moment its --timeout
// elapses and then blocks in waitpid on the silent remote shell, so waiting for it
// to exit hangs the run; csync must stop a command that has already said it is
// giving up. The stderr it wrote has to survive the stop, because that text is
// what the caller classifies the failure from.
func TestStderrPhraseStopsTheCommand(t *testing.T) {
	got := finish(t, []string{"poll: timeout"}, "echo 'rsync: error: poll: timeout' >&2\nsleep 300\n")

	if got.err == nil {
		t.Error("RunUntil succeeded, want the stopped command to report a failure")
	}
	// The caller's own context never ended, so blaming it would tell a user their
	// run was cancelled when in fact the remote went quiet.
	if errors.Is(got.err, context.Canceled) {
		t.Errorf("got %v, want an error that does not blame the caller's context", got.err)
	}
	if !strings.Contains(string(got.out.Stderr), "poll: timeout") {
		t.Errorf("stderr = %q, want it to still carry the phrase that stopped the command", got.out.Stderr)
	}
}

// TestPhraseSplitAcrossWritesStopsTheCommand covers the phrase arriving in pieces.
// Nothing guarantees a command writes a whole line in one write, or that a pipe
// delivers it in one read, so matching only what each write carries would miss a
// phrase straddling two of them — and miss it precisely when the transfer is
// already in trouble.
func TestPhraseSplitAcrossWritesStopsTheCommand(t *testing.T) {
	body := "printf 'rsync: error: poll: ' >&2\nsleep 0.5\nprintf 'timeout\\n' >&2\nsleep 300\n"

	got := finish(t, []string{"poll: timeout"}, body)

	if got.err == nil {
		t.Error("RunUntil succeeded, want the stopped command to report a failure")
	}
}

// TestUnmatchedOutputRunsToCompletion guards the common path: a command that
// never says it is giving up must run to its own end. A watcher that stopped
// commands early would cut off healthy transfers, which is a worse failure than
// the hang it was added to prevent.
func TestUnmatchedOutputRunsToCompletion(t *testing.T) {
	body := "echo 'rsync: some other trouble' >&2\nsleep 0.5\necho DONE\n"

	got := finish(t, []string{"poll: timeout"}, body)

	if got.err != nil {
		t.Errorf("RunUntil: %v, want the command to run to completion", got.err)
	}
	if !strings.Contains(string(got.out.Stdout), "DONE") {
		t.Errorf("stdout = %q, want the command to have reached its end", got.out.Stdout)
	}
}
