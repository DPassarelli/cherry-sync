package view

import (
	"fmt"
	"reflect"
	"testing"
)

// makeLines builds n distinguishable lines "L0".."L{n-1}" so a returned slice can
// be asserted exactly, not just by length.
func makeLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	return lines
}

func TestScrollFits(t *testing.T) {
	cases := []struct {
		name           string
		n, height, cur int
	}{
		// windowSize = height-2, so "fits" means n <= height-2.
		{"under", 3, 10, 1},
		{"exactly", 8, 10, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := makeLines(c.n)
			vp := scroll(lines, c.cur, 0, c.height)
			if !reflect.DeepEqual(vp.Lines, lines) {
				t.Errorf("Lines = %v, want all %v", vp.Lines, lines)
			}
			if vp.Offset != 0 {
				t.Errorf("Offset = %d, want 0", vp.Offset)
			}
			if vp.HiddenAbove != 0 || vp.HiddenBelow != 0 {
				t.Errorf("hidden = %d/%d, want 0/0", vp.HiddenAbove, vp.HiddenBelow)
			}
		})
	}
}

func TestScrollWindowSizeIsHeightMinusTwo(t *testing.T) {
	// height 10 reserves two rows for the ▲/▼ indicator lines, leaving 8 for content.
	vp := scroll(makeLines(20), 0, 0, 10)
	if len(vp.Lines) != 8 {
		t.Errorf("len(Lines) = %d, want 8 (height-2)", len(vp.Lines))
	}
}

func TestScrollUnknownHeight(t *testing.T) {
	// Before the first WindowSizeMsg the height is 0: show everything, hide nothing.
	for _, h := range []int{0, -5} {
		lines := makeLines(8)
		vp := scroll(lines, 3, 0, h)
		if !reflect.DeepEqual(vp.Lines, lines) {
			t.Errorf("height %d: Lines = %v, want all", h, vp.Lines)
		}
		if vp.Offset != 0 || vp.HiddenAbove != 0 || vp.HiddenBelow != 0 {
			t.Errorf("height %d: offset/hidden = %d %d/%d, want 0 0/0", h, vp.Offset, vp.HiddenAbove, vp.HiddenBelow)
		}
	}
}

func TestScrollHoldsUntilCursorReachesBottomEdge(t *testing.T) {
	// n=50, height=12 -> windowSize=10. Starting at the top (offset 0) the window
	// must hold still while the cursor moves within it, and only scroll when the
	// cursor steps past the bottom edge (line 10). This is the edge-triggered
	// behavior — no scrolling until an edge is reached.
	const n, height = 50, 12
	lines := makeLines(n)
	for cur := range 10 {
		vp := scroll(lines, cur, 0, height)
		if vp.Offset != 0 {
			t.Errorf("cursor %d: Offset = %d, want 0 (window holds until bottom edge)", cur, vp.Offset)
		}
	}
	vp := scroll(lines, 10, 0, height)
	if vp.Offset != 1 {
		t.Errorf("cursor 10: Offset = %d, want 1 (scroll by one at the edge)", vp.Offset)
	}
}

func TestScrollHoldsUntilCursorReachesTopEdge(t *testing.T) {
	// Symmetric to the bottom edge: with the window parked at offset 20, moving the
	// cursor up within it holds the window, and it scrolls only when the cursor
	// steps above the top edge (line 20).
	const n, height = 50, 12
	lines := makeLines(n)
	for cur := 20; cur < 30; cur++ {
		vp := scroll(lines, cur, 20, height)
		if vp.Offset != 20 {
			t.Errorf("cursor %d: Offset = %d, want 20 (window holds until top edge)", cur, vp.Offset)
		}
	}
	vp := scroll(lines, 19, 20, height)
	if vp.Offset != 19 {
		t.Errorf("cursor 19: Offset = %d, want 19 (scroll by one at the edge)", vp.Offset)
	}
}

func TestScrollClampsAtBottom(t *testing.T) {
	// The offset never runs past the last full window: n=20, windowSize=8 -> maxOffset
	// 12. A stale-deep prevOffset with the cursor at the last line settles at 12 with
	// nothing hidden below.
	vp := scroll(makeLines(20), 19, 99, 10)
	if vp.Offset != 12 {
		t.Errorf("Offset = %d, want 12 (n-windowSize)", vp.Offset)
	}
	if vp.HiddenBelow != 0 {
		t.Errorf("HiddenBelow = %d, want 0", vp.HiddenBelow)
	}
	if vp.HiddenAbove != 12 {
		t.Errorf("HiddenAbove = %d, want 12", vp.HiddenAbove)
	}
}

func TestScrollClampsAtTop(t *testing.T) {
	// Cursor at line 0 pins the window to the top regardless of prevOffset.
	vp := scroll(makeLines(20), 0, 5, 10)
	if vp.Offset != 0 {
		t.Errorf("Offset = %d, want 0", vp.Offset)
	}
	if vp.HiddenAbove != 0 {
		t.Errorf("HiddenAbove = %d, want 0", vp.HiddenAbove)
	}
	if vp.HiddenBelow != 12 {
		t.Errorf("HiddenBelow = %d, want 12", vp.HiddenBelow)
	}
}

func TestScrollClampsStaleOffsetAfterResize(t *testing.T) {
	// The terminal grows: a prevOffset valid for the old (small) window is now past
	// the last full window and must clamp down even though the cursor is already
	// visible. n=50, height=42 -> windowSize=40, maxOffset=10; prevOffset 40 -> 10.
	vp := scroll(makeLines(50), 45, 40, 42)
	if vp.Offset != 10 {
		t.Errorf("Offset = %d, want 10 (clamped to n-windowSize)", vp.Offset)
	}
	if vp.HiddenBelow != 0 {
		t.Errorf("HiddenBelow = %d, want 0", vp.HiddenBelow)
	}
}

func TestScrollTinyHeight(t *testing.T) {
	// height=3 -> windowSize floored at 1; the single visible line must be the
	// cursor's. Starting from offset 0 with the cursor deep in the list, the window
	// scrolls to land exactly on it.
	vp := scroll(makeLines(20), 10, 0, 3)
	if len(vp.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(vp.Lines))
	}
	if vp.Lines[0] != "L10" {
		t.Errorf("Lines[0] = %q, want L10 (the cursor's line)", vp.Lines[0])
	}
}

func TestScrollCursorAlwaysVisibleWalking(t *testing.T) {
	// Thread the offset forward the way Update does — each step feeds the previous
	// offset back in — and walk the cursor all the way down and back up. At every
	// step the cursor must be inside the window and the accounting must balance.
	const n, height = 50, 12
	lines := makeLines(n)
	walk := func(seq []int) {
		offset := 0
		for _, cur := range seq {
			vp := scroll(lines, cur, offset, height)
			offset = vp.Offset
			if cur < vp.HiddenAbove || cur >= vp.HiddenAbove+len(vp.Lines) {
				t.Fatalf("cursor %d not visible: HiddenAbove=%d len=%d", cur, vp.HiddenAbove, len(vp.Lines))
			}
			if got := vp.HiddenAbove + len(vp.Lines) + vp.HiddenBelow; got != n {
				t.Fatalf("cursor %d: accounting = %d, want %d", cur, got, n)
			}
			if vp.Offset != vp.HiddenAbove {
				t.Fatalf("cursor %d: Offset=%d != HiddenAbove=%d", cur, vp.Offset, vp.HiddenAbove)
			}
		}
	}
	down := make([]int, n)
	for i := range n {
		down[i] = i
	}
	up := make([]int, n)
	for i := range n {
		up[i] = n - 1 - i
	}
	walk(down)
	walk(up)
}

func TestScrollIsIdempotentWhenCursorVisible(t *testing.T) {
	// View calls scroll again with the already-settled offset; re-running it with the
	// same cursor must not move the window (otherwise the render would disagree with
	// the state Update stored).
	lines := makeLines(50)
	first := scroll(lines, 25, 0, 12)
	second := scroll(lines, 25, first.Offset, 12)
	if second.Offset != first.Offset {
		t.Errorf("second Offset = %d, want %d (idempotent)", second.Offset, first.Offset)
	}
}
