package operand_test

import (
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/operand"
)

// Behavior: a remote operand whose path begins with the home shortcut "~/" is
// rewritten to the equivalent relative path (rsync resolves a relative remote
// path against the login home) and flagged as rewritten. Without this, modern
// rsync's protected-args default passes the "~" literally and the transfer fails
// with exit 12 — the bug in #50. Mirrors the "~ home shortcut is normalized"
// scenario in saved-targets.feature.
func TestNormalize_RemoteTildeSlash_StrippedAndFlagged(t *testing.T) {
	got, err := operand.Normalize("user@host:~/working")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "user@host:working" {
		t.Errorf("Path: got %q, want %q", got.Path, "user@host:working")
	}
	if !got.Rewrote {
		t.Errorf("Rewrote: got false, want true (a ~ was stripped)")
	}
	if got.From != "~/working" {
		t.Errorf("From: got %q, want %q (the original path portion, for the disclosure)", got.From, "~/working")
	}
}

// Behavior: a bare "~" remote path (the home directory itself, with or without a
// trailing slash) becomes ".", never the empty string — an empty path would
// become the filesystem root once rsync appends its trailing slash. It is still
// flagged as rewritten.
func TestNormalize_RemoteBareTilde_BecomesDot(t *testing.T) {
	for _, in := range []string{"host:~", "host:~/"} {
		got, err := operand.Normalize(in)
		if err != nil {
			t.Fatalf("Normalize(%q): unexpected error: %v", in, err)
		}
		if got.Path != "host:." {
			t.Errorf("Normalize(%q) Path: got %q, want %q", in, got.Path, "host:.")
		}
		if !got.Rewrote {
			t.Errorf("Normalize(%q) Rewrote: got false, want true", in)
		}
	}
}

// Behavior: a "~user" home shortcut is rejected. Another user's home has no
// relative equivalent — only an absolute path reaches it — so csync fails up
// front with a clear error rather than letting rsync fail confusingly mid-
// transfer. The message names the tilde so the user can see what to fix.
func TestNormalize_RemoteTildeUser_Rejected(t *testing.T) {
	_, err := operand.Normalize("user@host:~deploy/project")
	if err == nil {
		t.Fatal("expected error for ~user remote path, got nil")
	}
}

// Behavior: an absolute remote path is left exactly as given and not flagged as
// rewritten — there is nothing to resolve.
func TestNormalize_RemoteAbsolute_Unchanged(t *testing.T) {
	got, err := operand.Normalize("user@host:/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "user@host:/project" {
		t.Errorf("Path: got %q, want %q", got.Path, "user@host:/project")
	}
	if got.Rewrote {
		t.Errorf("Rewrote: got true, want false (absolute path is untouched)")
	}
}

// Behavior: a remote path that is already relative (no leading ~) is left as
// given and not flagged — it already means what the user intends.
func TestNormalize_RemoteRelative_Unchanged(t *testing.T) {
	got, err := operand.Normalize("user@host:working")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "user@host:working" {
		t.Errorf("Path: got %q, want %q", got.Path, "user@host:working")
	}
	if got.Rewrote {
		t.Errorf("Rewrote: got true, want false")
	}
}

// Behavior: a trailing slash on the path portion is collapsed so csync owns the
// operand's shape rather than relying on rsync tolerating a doubled slash once it
// appends its own. This is a display-level cleanup, not the surprising tilde
// change, so it is NOT flagged as rewritten. Applies to remote and local paths.
func TestNormalize_TrailingSlash_Collapsed(t *testing.T) {
	cases := map[string]string{
		"user@host:/project/": "user@host:/project",
		"./local/path/":       "./local/path",
		"./local/path//":      "./local/path",
	}
	for in, want := range cases {
		got, err := operand.Normalize(in)
		if err != nil {
			t.Fatalf("Normalize(%q): unexpected error: %v", in, err)
		}
		if got.Path != want {
			t.Errorf("Normalize(%q) Path: got %q, want %q", in, got.Path, want)
		}
		if got.Rewrote {
			t.Errorf("Normalize(%q) Rewrote: got true, want false (slash collapse is not a rewrite)", in)
		}
	}
}

// Behavior: a local operand beginning with "~" is left untouched — resolving a
// local home shortcut is a separate concern (it would expand against our own
// home, not be stripped) and is out of scope here. The tilde handling keys off
// the operand being remote.
func TestNormalize_LocalTilde_Unchanged(t *testing.T) {
	got, err := operand.Normalize("~/local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "~/local" {
		t.Errorf("Path: got %q, want %q", got.Path, "~/local")
	}
	if got.Rewrote {
		t.Errorf("Rewrote: got true, want false (local tilde is out of scope)")
	}
}

// Behavior: "." (the current directory, used as the local operand for push/pull)
// passes through unchanged and is never emptied by slash collapse.
func TestNormalize_Dot_Unchanged(t *testing.T) {
	got, err := operand.Normalize(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "." {
		t.Errorf("Path: got %q, want %q", got.Path, ".")
	}
}
