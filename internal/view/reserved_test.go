package view

import (
	"fmt"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestReservedPreambleShrinksWindow pins that the rows main already printed above the
// picker (banner, header, exclusions) are held out of the scroll region: a picker
// given a 5-row preamble shows exactly 5 fewer content lines than one with none, for
// the same list and terminal height. Without this the picker sizes its frame to the
// whole terminal, and the preamble scrolls off the top.
func TestReservedPreambleShrinksWindow(t *testing.T) {
	var actions []compare.Action
	for i := range 40 {
		actions = append(actions, compare.Action{Verb: "create", Path: fmt.Sprintf("f%02d.txt", i)})
	}

	base := newModel(actions)
	base.width, base.height = 80, 24
	withPre := newModel(actions)
	withPre.width, withPre.height = 80, 24
	withPre.preamble = "r1\nr2\nr3\nr4\nr5\n" // five rows, none wide enough to wrap at width 80

	b := base.visible()
	r := withPre.visible()
	if len(b.Lines) <= len(r.Lines) {
		t.Fatalf("precondition: expected preamble window smaller; base=%d withPre=%d", len(b.Lines), len(r.Lines))
	}
	if got := len(b.Lines) - len(r.Lines); got != 5 {
		t.Errorf("preamble window is %d lines smaller, want 5", got)
	}
}
