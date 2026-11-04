// detail.go renders the trailing annotation the change list carries for the row
// under the cursor: how the incoming copy's size compares with the one it would
// replace, and how old each copy is. The measurements come from compare's Action;
// the wording is here, so rsync's vocabulary never reaches the screen.

package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// detailSeparator joins the parts of a row's annotation. A middot rather than a
// comma keeps the parts visually separable from the commas inside them and from
// the filename that precedes them.
const detailSeparator = " · "

// detailSegment is one run of a row's annotation. The annotation stays in segments
// up to the point of rendering because its text is measured and trimmed first, and
// neither is safe once escape sequences have been written into the string.
type detailSegment struct {
	text       string
	emphasized bool
}

// actionDetail returns the annotation for one row of the change list as plain text,
// measured against now.
func actionDetail(a compare.Action, now time.Time) string {
	var b strings.Builder
	for _, s := range detailSegments(a, now) {
		b.WriteString(s.text)
	}
	return b.String()
}

// detailSegments builds the annotation for one row of the change list. Only an update
// carries one: a create has no counterpart on the other side and a delete is being
// removed rather than compared, so for both the column is left empty rather than
// filled with a claim about a file that has no second copy.
//
// The annotation states the size gap between the two copies and then each copy's
// own age, rather than the gap between their timestamps. Two ages answer "is this
// the copy I was just working on?" directly, where a single interval between them
// leaves the reader to work out whether either is recent at all. The newer copy's
// phrase is emphasized because which copy is the live one is what a reader most needs
// from the row before deciding to overwrite it.
//
// An update whose destination could not be measured falls back to the itemize
// labels, which say which attributes differ without needing the far side.
func detailSegments(a compare.Action, now time.Time) []detailSegment {
	if a.Verb != "update" {
		return nil
	}
	if !a.Dest.Known {
		return []detailSegment{{text: differenceLabel(a.Diff)}}
	}
	return []detailSegment{
		{text: sourceLead(a)},
		{text: "last updated " + age(a.ModTime, now), emphasized: isNewer(a.ModTime, a.Dest.ModTime)},
		{text: detailSeparator + "dest "},
		{text: "last updated " + age(a.Dest.ModTime, now), emphasized: isNewer(a.Dest.ModTime, a.ModTime)},
	}
}

// sourceLead opens the annotation by naming the incoming copy and how its size
// compares with the one it would replace. The size half is omitted when the two
// copies are the same size, leaving "source" to attach to the timestamp so the clause
// always names whose age it is reporting.
func sourceLead(a compare.Action) string {
	gap := a.Size - a.Dest.Size
	if gap == 0 {
		return "source "
	}
	return "source is " + formatBytes(abs64(gap)) + " " + comparative(gap > 0, "larger", "smaller") + ", "
}

// isNewer reports whether one copy's timestamp is later than the other's. An unreadable
// timestamp ranks nothing, since calling its counterpart newer would be a guess. Whole
// seconds are the finest precision both sides share: rsync reports only seconds, while
// a stat of a local destination also carries nanoseconds.
func isNewer(when, other time.Time) bool {
	if when.IsZero() || other.IsZero() {
		return false
	}
	return when.Truncate(time.Second).After(other.Truncate(time.Second))
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
