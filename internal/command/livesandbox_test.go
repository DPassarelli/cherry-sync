//go:build livesandbox

// This file is excluded from the normal build. It needs a real remote to talk
// to, which CI does not have, so it is opt-in:
//
//	CSYNC_TEST_REMOTE=user@host:/tmp/csync-livetest \
//	  go test -tags livesandbox ./internal/command/
//
// It guards an assumption the cancellation policy rests on and that no local
// test can check: that a cancelled rsync tidies up after itself. Run's Cancel
// signals only rsync, never anything rsync spawned, and csync passes neither
// --partial nor --inplace — so both rsync's ssh child and any half-written file
// on the destination are rsync's own to clean up. Adding either flag, or
// switching to a transport that outlives its parent, would break that silently.
package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// liveTarget returns the rsync destination under test and the bare host within
// it, which is what identifies our own ssh processes among everyone else's.
func liveTarget(t *testing.T) (target, host string) {
	t.Helper()
	target = os.Getenv("CSYNC_TEST_REMOTE")
	if target == "" {
		t.Skip("set CSYNC_TEST_REMOTE=user@host:/path to run the live-sandbox tests")
	}
	host = target
	i := strings.Index(host, ":")
	if i >= 0 {
		host = host[:i]
	}
	i = strings.Index(host, "@")
	if i >= 0 {
		host = host[i+1:]
	}
	return target, host
}

// sshCount reports how many local ssh processes are talking to host.
func sshCount(t *testing.T, host string) int {
	t.Helper()
	// pgrep exits 1 when nothing matches, which is not an error here.
	out, _ := exec.Command("pgrep", "-f", "ssh.*"+host).Output()
	return len(strings.Fields(string(out)))
}

// remoteEntries lists what the destination directory currently holds.
func remoteEntries(t *testing.T, target string) []string {
	t.Helper()
	host, dir, found := strings.Cut(target, ":")
	if !found {
		t.Fatalf("CSYNC_TEST_REMOTE=%q has no :path component", target)
	}
	out, err := exec.Command("ssh", "-o", "BatchMode=yes", host, "ls -A "+dir).Output()
	if err != nil {
		t.Fatalf("listing %s: %v", target, err)
	}
	return strings.Fields(string(out))
}

// TestCancelledRemoteTransferLeavesNothingBehind cancels a real transfer partway
// through and checks both halves of the cleanup csync relies on: no ssh left
// running locally, and no partial file left on the destination.
func TestCancelledRemoteTransferLeavesNothingBehind(t *testing.T) {
	target, host := liveTarget(t)

	host2, dir, _ := strings.Cut(target, ":")
	reset := exec.Command("ssh", "-o", "BatchMode=yes", host2, "rm -rf "+dir+" && mkdir -p "+dir)
	err := reset.Run()
	if err != nil {
		t.Fatalf("preparing %s: %v", target, err)
	}

	// Big enough, throttled hard enough, that the transfer is certainly still
	// running when it is cancelled.
	local := t.TempDir()
	err = os.WriteFile(filepath.Join(local, "payload.bin"), make([]byte, 32<<20), 0o600)
	if err != nil {
		t.Fatalf("write payload: %v", err)
	}

	before := sshCount(t, host)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = New(silent{}).Run(ctx, "rsync", []string{
			"--archive",
			"--bwlimit=64", // KB/s
			"--", local + "/", target + "/",
		}, nil)
	}()

	time.Sleep(4 * time.Second)
	during := sshCount(t, host)
	if during <= before {
		cancel()
		<-done
		t.Fatalf("no ssh appeared for %s (before=%d during=%d): the transfer never started, so this test proved nothing", host, before, during)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Run never returned after cancellation")
	}

	// Reaping is asynchronous even once the signal has landed.
	time.Sleep(1 * time.Second)
	after := sshCount(t, host)
	if after > before {
		t.Errorf("ssh survived cancellation: before=%d during=%d after=%d", before, during, after)
	}

	entries := remoteEntries(t, target)
	if len(entries) != 0 {
		t.Errorf("destination still holds %v after cancellation; rsync left a partial file behind", entries)
	}
}

// TestRemoteRsyncVersion records which rsync implementation the sandbox runs.
// It asserts nothing: the two implementations csync meets disagree about
// permissions on new files (issue #102), so a failure elsewhere in this file is
// much easier to read with this in the log.
func TestRemoteRsyncVersion(t *testing.T) {
	target, _ := liveTarget(t)
	host, _, _ := strings.Cut(target, ":")

	out, err := exec.Command("ssh", "-o", "BatchMode=yes", host, "rsync --version").Output()
	if err != nil {
		t.Fatalf("asking %s for its rsync version: %v", host, err)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	t.Logf("remote %s runs %s", host, strings.TrimSpace(first))
}
