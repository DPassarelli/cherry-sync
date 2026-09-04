// detail.go renders the trailing annotation the change list carries for a changed
// file: what differs about it. The values come from compare's Action; the wording
// is here, so rsync's vocabulary never reaches the screen.

package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// detailGap separates a row's annotation from the verb that precedes it.
const detailGap = "  "

// minDetailWidth is the narrowest an annotation may be squeezed to before it is
// dropped instead. A couple of surviving characters and an ellipsis tell the reader
// nothing except that something was cut, and the columns are better spent on the
// filename.
const minDetailWidth = 12

// actionDetail returns the annotation for one row of the change list. Only an
// update carries one: a create has no counterpart on the other side to differ from,
// and a delete is being removed rather than compared, so for both the column is
// left empty rather than filled with a claim about a file that has no second copy.
func actionDetail(a compare.Action) string {
	if a.Verb != "update" {
		return ""
	}
	return differenceLabel(a.Diff)
}

// differenceLabel names what differs between the two copies of an updated file.
// Content differs on every update csync reports, because the comparison settles
// each candidate by checksum, so it is named only where nothing else differs: a
// file whose size and timestamp both match yet whose bytes do not is the one state
// a reader would otherwise take for a bug. "only" marks that exclusivity, so the
// ordinary case (both attributes differ) reads without it.
func differenceLabel(d compare.Difference) string {
	switch {
	case d.Size && d.ModTime:
		return "size and mtime"
	case d.ModTime:
		return "mtime only"
	case d.Size:
		return "size only"
	default:
		return "contents only"
	}
}

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
func fitDetail(detail string, used, width int) string {
	if detail == "" || width <= 0 {
		return detail
	}
	available := width - used - len(detailGap)
	if available < minDetailWidth {
		return ""
	}
	if lipgloss.Width(detail) <= available {
		return detail
	}
	return truncateCells(detail, available-1) + "…"
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
