// detail.go renders the trailing annotation the change list carries for each file —
// how big it is, how long ago it was touched, and what differs about it. The values
// come from compare's Action; the wording is here, so rsync's vocabulary never
// reaches the screen.

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

// relativeCutoff is the age past which a file's timestamp is shown as a date
// instead of as an age. Beyond roughly a month "47 days ago" is a number the reader
// has to convert back into a date, which is the work the relative form exists to
// save.
const relativeCutoff = 30 * 24 * time.Hour

// dateLayout renders a timestamp too old for the relative form.
const dateLayout = "2006-01-02"

// actionDetail returns the annotation for one row of the change list, measured
// against now: the file's size, its age, and — for an update — what differs about
// it. Empty for a delete, whose rsync record carries no usable metadata and whose
// file is being removed rather than examined. Any part that is missing is left out
// rather than rendered as a placeholder, so a row degrades to the parts that are
// known instead of to a dangling separator.
func actionDetail(a compare.Action, now time.Time) string {
	if a.Verb == "delete" {
		return ""
	}
	parts := []string{formatBytes(a.Size)}
	age := relativeTime(a.ModTime, now)
	if age != "" {
		parts = append(parts, age)
	}
	// A create has no counterpart on the other side, so it has nothing to differ
	// from and carries no label.
	if a.Verb == "update" {
		parts = append(parts, differenceLabel(a.Diff))
	}
	return strings.Join(parts, detailSeparator)
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

// relativeTime renders when as its age at now — the question a change list is
// actually asked, since a file's age is what says whether it is the copy you were
// just working on. It coarsens as the age grows and gives way to a plain date past
// relativeCutoff. The zero Time renders as nothing: it means the timestamp could
// not be read (see compare.parseModTime), and an age computed from it would be
// nonsense rather than a measurement.
//
// A timestamp ahead of now reads as "just now". Clock skew between a local machine
// and a remote dev box is routine, and a negative age would report that skew as
// though it were a fact about the file.
func relativeTime(when, now time.Time) string {
	if when.IsZero() {
		return ""
	}
	age := now.Sub(when)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return plural(int(age.Minutes()), "min", "min")
	case age < 24*time.Hour:
		return plural(int(age.Hours()), "hour", "hours")
	case age < relativeCutoff:
		return plural(int(age.Hours()/24), "day", "days")
	default:
		return when.Format(dateLayout)
	}
}

// plural renders a count of units as "N unit ago", choosing between the singular
// and plural forms. Units that don't inflect (an abbreviation like "min") pass the
// same word twice.
func plural(n int, singular, plural string) string {
	unit := plural
	if n == 1 {
		unit = singular
	}
	return fmt.Sprintf("%d %s ago", n, unit)
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
