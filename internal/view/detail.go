// detail.go renders the trailing annotation the change list carries for the row
// under the cursor: how the incoming copy's size compares with the one it would
// replace, and how old each copy is. The measurements come from compare's Action;
// the wording is here, so rsync's vocabulary never reaches the screen.

package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// detailSeparator joins the parts of a row's annotation. A middot rather than a
// comma keeps the parts visually separable from the commas inside them and from
// the filename that precedes them.
const detailSeparator = " · "

// actionDetail returns the annotation for one row of the change list, measured
// against now. Only an update carries one: a create has no counterpart on the other
// side and a delete is being removed rather than compared, so for both the column
// is left empty rather than filled with a claim about a file that has no second
// copy.
//
// The annotation states the size gap between the two copies and then each copy's
// own age, rather than the gap between their timestamps. Two ages answer "is this
// the copy I was just working on?" directly, where a single interval between them
// leaves the reader to work out whether either is recent at all.
//
// An update whose destination could not be measured falls back to the itemize
// labels, which say which attributes differ without needing the far side.
func actionDetail(a compare.Action, now time.Time) string {
	if a.Verb != "update" {
		return ""
	}
	if !a.Dest.Known {
		return differenceLabel(a.Diff)
	}
	source := sourceClause(a, now)
	return source + detailSeparator + "dest last updated " + age(a.Dest.ModTime, now)
}

// sourceClause describes the incoming copy: how its size compares with the one it
// would replace, and when it was last written. The size half is omitted when the
// two copies are the same size, and "source" then attaches to the timestamp instead
// so the clause always names whose age it is reporting.
func sourceClause(a compare.Action, now time.Time) string {
	gap := a.Size - a.Dest.Size
	if gap == 0 {
		return "source last updated " + age(a.ModTime, now)
	}
	return "source is " + formatBytes(abs64(gap)) + " " + comparative(gap > 0, "larger", "smaller") +
		", last updated " + age(a.ModTime, now)
}

// age renders how long ago a file was last written, as a compact span. The zero
// Time — an unreadable timestamp — reports "unknown" rather than an age counted
// from year one, and a timestamp ahead of now reports "just now": clock skew
// between a local machine and a remote dev box is routine, and a negative age would
// report that skew as though it were a fact about the file.
func age(when, now time.Time) string {
	if when.IsZero() {
		return "unknown"
	}
	if !when.Before(now) {
		return "just now"
	}
	return compactDuration(now.Sub(when)) + " ago"
}

// comparative picks between the two directions of a comparison, so the sign of a
// difference is turned into a word in one place rather than at each call.
func comparative(ahead bool, whenAhead, whenBehind string) string {
	if ahead {
		return whenAhead
	}
	return whenBehind
}

// abs64 returns the magnitude of n. The direction is carried by the wording, so the
// number itself is always rendered unsigned.
func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// compactDuration renders a span in the largest whole unit it fills. It stays
// compact because it sits inside a change-list row rather than in prose, and it
// stops at days: "m" has to mean minutes unambiguously, which rules out a month
// abbreviation, and a long span in days is still a number a reader can place.
func compactDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
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

// byteUnits are the size suffixes in ascending order, each 1024 times the last.
// Binary steps rather than decimal ones, because the sizes being described are
// file sizes as every other tool on the box reports them.
var byteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

// formatBytes renders a size in the largest unit that leaves a value of at least
// one. Below ten it keeps a single decimal, which is what separates 4.2 KB from
// 4.9 KB; above ten the decimal is noise the leading digits already carry, so it is
// dropped. Whole bytes are never fractional.
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d %s", n, byteUnits[0])
	}
	value := float64(n)
	unit := 0
	// Step up while there is a larger unit that still leaves a value of at least one.
	for value >= 1024 && unit < len(byteUnits)-1 {
		value /= 1024
		unit++
	}
	if value < 10 {
		return fmt.Sprintf("%.1f %s", value, byteUnits[unit])
	}
	return fmt.Sprintf("%.0f %s", value, byteUnits[unit])
}

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
