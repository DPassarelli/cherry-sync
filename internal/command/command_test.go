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
