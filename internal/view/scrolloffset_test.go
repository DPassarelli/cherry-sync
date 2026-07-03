package view

import (
	"fmt"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestScrollOffsetSnapsToTopAtFirstRow pins the top-of-list behavior: when the
// cursor is on the very first row, the window must sit at offset 0 so the group
// heading above that row is visible and no "▲ more above" is shown. Plain
// edge-triggered scrolling would leave the window one line down (the first row at
// the window's top edge), stranding the heading and lying about content above —
// the ends of the list snap fully into view; only the middle is edge-triggered.
func TestScrollOffsetSnapsToTopAtFirstRow(t *testing.T) {
	var actions []compare.Action
	for i := range 30 {
		actions = append(actions, compare.Action{Verb: "create", Path: fmt.Sprintf("file_%02d.txt", i)})
	}
	m := newModel(actions)
	m.height = 20

	// Scroll well down the list so the offset settles at a nonzero value.
	m.cursor = 25
	m.offset = m.scrollOffset()
	if m.offset == 0 {
		t.Fatalf("precondition: expected a nonzero offset when scrolled down, got 0")
	}

	// Returning to the first row must reveal the top of the list, heading and all.
	m.cursor = 0
	got := m.scrollOffset()
	if got != 0 {
		t.Errorf("scrollOffset at first row = %d, want 0 (top fully revealed)", got)
	}
}
