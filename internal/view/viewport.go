// viewport.go computes the scrolling window over the picker's rendered lines so a
// change list taller than the terminal stays navigable: it slices the lines to the
// rows that fit and reports how many are hidden above and below, which the picker
// turns into scroll indicators.

package view

// viewport is the visible slice of a taller rendered list together with the offset
// of its first line and how many lines are hidden above and below it — enough for
// the caller to draw scroll indicators (▲/▼), know which rows to paint, and store
// the settled offset for the next frame.
type viewport struct {
	Lines       []string
	Offset      int
	HiddenAbove int
	HiddenBelow int
}

// scroll returns the viewport over lines that keeps the cursor's line on screen
// within height rows, moving the previous offset the minimum needed: the window
// holds still while the cursor travels inside it and scrolls only when the cursor
// reaches an edge (edge-triggered, not centered). prevOffset is the offset from the
// last frame — threading it back in is what makes the window hold its position.
// When the list fits — or height is not yet known (<= 0, the first frame before a
// WindowSizeMsg) — every line is returned at offset 0 with nothing hidden. On
// overflow it reserves two of the height rows for the indicator lines and windows
// the rest, clamping the offset to the list's ends so the cursor stays inside the
// returned slice.
func scroll(lines []string, cursorLine, prevOffset, height int) viewport {
	n := len(lines)
	windowSize := max(height-2, 1)
	if height <= 0 || n <= windowSize {
		return viewport{Lines: lines}
	}
	offset := min(prevOffset, cursorLine)
	if cursorLine >= offset+windowSize {
		offset = cursorLine - windowSize + 1
	}
	maxOffset := n - windowSize
	offset = min(offset, maxOffset)
	offset = max(offset, 0)
	return viewport{
		Lines:       lines[offset : offset+windowSize],
		Offset:      offset,
		HiddenAbove: offset,
		HiddenBelow: n - offset - windowSize,
	}
}
