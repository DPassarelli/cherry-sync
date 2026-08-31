package view

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestRenderSummary covers how the post-sync header counts the applied changes
// and, when some were removals, calls them out distinctly. With no deletes it
// stays in the plain "(N files)" form; with deletes it adds the "M of which
// were removed" clause, both counts and both verbs pluralized independently.
func TestRenderSummary(t *testing.T) {
	create := compare.Action{Verb: "create", Path: "a"}
	del := compare.Action{Verb: "delete", Path: "b"}

	cases := []struct {
		name     string
		selected []compare.Action
		want     string
	}{
		{"single transfer, no removals", []compare.Action{create}, "Sync complete! (1 file)"},
		{"several transfers, no removals", []compare.Action{create, create, create}, "Sync complete! (3 files)"},
		{"one removal only", []compare.Action{del}, "Sync complete! (1 file total, 1 of which was removed)"},
		{"mixed, one removal", []compare.Action{create, del}, "Sync complete! (2 files total, 1 of which was removed)"},
		{"mixed, several removals", []compare.Action{create, del, del}, "Sync complete! (3 files total, 2 of which were removed)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderSummary(tc.selected)
			if !strings.Contains(got, tc.want) {
				t.Errorf("RenderSummary(%s):\n got %q\n want to contain %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestTimedOut covers the notice that ends a comparison csync gave up on. It has
// to name the limit it hit — an abort that states no number is indistinguishable
// from a crash — and it has to point somewhere, since csync has no flag to raise
// the limit and would otherwise leave the user with no next step.
func TestTimedOut(t *testing.T) {
	for _, limit := range []time.Duration{59 * time.Second, 5 * time.Second, 90 * time.Second} {
		got := TimedOut(limit)
		want := fmt.Sprintf("%d seconds", int(limit.Seconds()))
		if !strings.Contains(got, want) {
			t.Errorf("TimedOut(%s) = %q, want it to name %q", limit, got, want)
		}
		if !strings.Contains(got, "✗") {
			t.Errorf("TimedOut(%s) = %q, want the cancel glyph", limit, got)
		}
		if !strings.Contains(got, "rsync") {
			t.Errorf("TimedOut(%s) = %q, want it to name the way out", limit, got)
		}
	}
}

// TestStalled covers the notice that ends a transfer csync abandoned because the
// remote went quiet. It has to say the remote stopped responding — the one thing
// that distinguishes this from a comparison csync gave up on, which is the other
// notice that ends a run with a limit in it — and it has to name the limit, so an
// abort is legible as a decision rather than a crash.
//
// It also has to warn that the sync was left half done. A transfer killed partway
// leaves the destination in a state neither side agrees on, and a notice that
// implies nothing moved would be a lie the user acts on.
func TestStalled(t *testing.T) {
	for _, limit := range []time.Duration{30 * time.Second, 3 * time.Second} {
		got := Stalled(limit)
		want := fmt.Sprintf("%d seconds", int(limit.Seconds()))
		if !strings.Contains(got, want) {
			t.Errorf("Stalled(%s) = %q, want it to name %q", limit, got, want)
		}
		if !strings.Contains(got, "stopped responding") {
			t.Errorf("Stalled(%s) = %q, want it to say the remote stopped responding", limit, got)
		}
		if !strings.Contains(got, "✗") {
			t.Errorf("Stalled(%s) = %q, want the cancel glyph", limit, got)
		}
		if !strings.Contains(got, "may already have") {
			t.Errorf("Stalled(%s) = %q, want it to warn the sync was left half done", limit, got)
		}
	}
}
