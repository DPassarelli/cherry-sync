package view

import (
	"testing"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// now is the moment every age below is measured against. Ages are rendered against
// an injected clock rather than time.Now precisely so these cases can exist: a test
// that could only assert "some age" would pin nothing.
var now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

// Behavior: the difference label names what differs between the two copies, and
// says "only" where one attribute differs alone. Content differs on every update
// (the comparison runs with --checksum), so it is worth naming only in the case
// where nothing else does — a file whose size and timestamp both match yet whose
// bytes do not is the one state a user would otherwise read as a bug.
func TestDifferenceLabel(t *testing.T) {
	cases := []struct {
		name string
		diff compare.Difference
		want string
	}{
		{"both", compare.Difference{Content: true, Size: true, ModTime: true}, "size and mtime"},
		{"mtime alone", compare.Difference{Content: true, ModTime: true}, "mtime only"},
		{"size alone", compare.Difference{Content: true, Size: true}, "size only"},
		{"neither", compare.Difference{Content: true}, "contents only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := differenceLabel(c.diff)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Behavior: a create carries no detail. There is no counterpart on the other side
// to measure it against, so any comparison would be a claim about a file that does
// not exist there.
func TestActionDetail_Create_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "create", Path: "new.go", Size: 1126}

	got := actionDetail(a, now)

	if got != "" {
		t.Errorf("got %q, want the empty string for a create", got)
	}
}

// Behavior: a delete carries no detail either. The file is being removed rather
// than compared, so there is nothing to measure it against.
func TestActionDetail_Delete_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "delete", Path: "stale.txt"}

	got := actionDetail(a, now)

	if got != "" {
		t.Errorf("got %q, want the empty string for a delete", got)
	}
}

// Behavior: a size is rendered in the largest unit that leaves a value of at least
// one, with a single decimal below ten and none above it. The decimal is what
// separates 4.2 KB from 4.9 KB, which is information; a tenth of a megabyte is
// noise, so it is dropped once the leading value is big enough to carry the
// meaning on its own.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		size int64
		want string
	}{
		{0, "0 B"}, {512, "512 B"}, {1023, "1023 B"}, {1024, "1.0 KB"},
		{4300, "4.2 KB"}, {12288, "12 KB"}, {1572864, "1.5 MB"}, {1073741824, "1.0 GB"},
	}
	for _, c := range cases {
		got := formatBytes(c.size)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.size, got, c.want)
		}
	}
}

// Behavior: a time span is rendered in the largest whole unit it fills, kept
// compact because it sits inside a row rather than in prose. Days do not give way
// to weeks or months: "m" has to mean minutes unambiguously, and a span of years
// in days is still a number the reader can place.
func TestCompactDuration(t *testing.T) {
	cases := []struct {
		span time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{time.Minute, "1m"},
		{4 * time.Minute, "4m"},
		{59 * time.Minute, "59m"},
		{2 * time.Hour, "2h"},
		{23 * time.Hour, "23h"},
		{3 * 24 * time.Hour, "3d"},
		{400 * 24 * time.Hour, "400d"},
	}
	for _, c := range cases {
		got := compactDuration(c.span)
		if got != c.want {
			t.Errorf("compactDuration(%v) = %q, want %q", c.span, got, c.want)
		}
	}
}

// Behavior: an update's annotation states how the incoming copy's size compares
// with the one it would replace, then how old each copy is. The two ages are
// measured against now rather than against each other: knowing the destination was
// last touched 17 days ago is what says the local edit is the live one.
func TestActionDetail_Update_ComparesSizeAndAgesBothCopies(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "compare.go",
		Size: 4096 + 2048, ModTime: now.Add(-3 * time.Minute),
		Dest: compare.Counterpart{Known: true, Size: 4096, ModTime: now.Add(-17 * 24 * time.Hour)},
	}

	got := actionDetail(a, now)

	want := "source is 2.0 KB larger, last updated 3m ago · dest last updated 17d ago"
	if got != want {
		t.Errorf("got %q,\nwant %q", got, want)
	}
}

// Behavior: a smaller incoming copy says so. The direction is the whole point of
// the clause — it is what tells the reader whether syncing would add content or
// remove it — so both directions are pinned.
func TestActionDetail_SmallerSource_SaysSmaller(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "a.go", Size: 100, ModTime: now.Add(-time.Hour),
		Dest: compare.Counterpart{Known: true, Size: 1124, ModTime: now.Add(-2 * time.Hour)},
	}

	got := actionDetail(a, now)

	want := "source is 1.0 KB smaller, last updated 1h ago · dest last updated 2h ago"
	if got != want {
		t.Errorf("got %q,\nwant %q", got, want)
	}
}

// Behavior: when both copies are the same size the size clause is dropped, and
// "source" attaches to the timestamp instead so the annotation still names whose
// age it is reporting. Without that the row would open with a bare "last updated"
// and leave the reader guessing which side it meant.
func TestActionDetail_EqualSizes_OmitTheSizeClause(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "a.go", Size: 512, ModTime: now.Add(-4 * time.Minute),
		Dest: compare.Counterpart{Known: true, Size: 512, ModTime: now.Add(-2 * 24 * time.Hour)},
	}

	got := actionDetail(a, now)

	want := "source last updated 4m ago · dest last updated 2d ago"
	if got != want {
		t.Errorf("got %q,\nwant %q", got, want)
	}
}

// Behavior: an update whose destination could not be measured falls back to the
// itemize labels. The destination is not always reachable, and the row must still
// say what it can rather than going blank or inventing a comparison.
func TestActionDetail_UnmeasuredDestination_FallsBackToTheItemizeLabel(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "a.go",
		Diff: compare.Difference{Content: true, Size: true, ModTime: true},
	}

	got := actionDetail(a, now)

	if got != "size and mtime" {
		t.Errorf("got %q, want the itemize fallback %q", got, "size and mtime")
	}
}

// Behavior: a timestamp ahead of now reads as "just now" rather than as a negative
// age. Clock skew between a local machine and a remote dev box is routine, and a
// file stamped a few seconds ahead is, for the reader's purposes, current.
func TestAge_Future_ReadsAsJustNow(t *testing.T) {
	if got := age(now.Add(time.Hour), now); got != "just now" {
		t.Errorf("got %q, want %q", got, "just now")
	}
}

// Behavior: an unreadable timestamp arrives as the zero Time and reports "unknown".
// Rendering it would put an age counted from year one into the row, which is worse
// than admitting the value is missing.
func TestAge_Zero_ReadsAsUnknown(t *testing.T) {
	if got := age(time.Time{}, now); got != "unknown" {
		t.Errorf("got %q, want %q", got, "unknown")
	}
}
