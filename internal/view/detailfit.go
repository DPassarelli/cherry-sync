// detailfit.go trims a row's annotation to the width the terminal has left for it,
// keeping track of which surviving text carries emphasis.

package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// detailGap separates a row's annotation from the verb that precedes it.
const detailGap = "  "

// minDetailWidth is the narrowest an annotation may be squeezed to before it is
// dropped instead. A couple of surviving characters and an ellipsis tell the reader
// nothing except that something was cut, and the columns are better spent on the
// filename.
const minDetailWidth = 12

// fitDetail trims a row's annotation to what the terminal can show, given the
// display width already used by the rest of the row and the total width available.
// A row that overflows would wrap onto a second terminal line while the scroll
// window still counted it as one, so the window would slide out of step with what
// the terminal actually drew — which is why this trims rather than letting the
// terminal decide.
//
// A width of zero means the terminal has not reported its size yet, so nothing is
// trimmed: guessing narrow would flicker the annotation away on the first frame and
// back on the second.
func fitDetail(segments []detailSegment, used, width int) []detailSegment {
	if len(segments) == 0 || width <= 0 {
		return segments
	}
	available := width - used - len(detailGap)
	if available < minDetailWidth {
		return nil
	}
	total := 0
	for _, s := range segments {
		total += lipgloss.Width(s.text)
	}
	if total <= available {
		return segments
	}
	return append(truncateSegments(segments, available-1), detailSegment{text: "…"})
}

// truncateSegments keeps at most max display cells of segments, leaving out any
// segment nothing of it survives.
func truncateSegments(segments []detailSegment, max int) []detailSegment {
	var kept []detailSegment
	for _, s := range segments {
		text := truncateCells(s.text, max)
		if text == "" {
			break
		}
		kept = append(kept, detailSegment{text: text, emphasized: s.emphasized})
		max -= lipgloss.Width(text)
	}
	return kept
}

// truncateCells cuts s to at most max display cells, measuring in cells rather than
// bytes or runes so a multi-byte name or a wide CJK character is not cut mid-glyph
// and does not overflow the row it was measured into.
func truncateCells(s string, max int) string {
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if used+w > max {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}
