package view

import (
	"testing"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// now is the moment every relative-time case below is measured against. Relative
// time is rendered against an injected clock rather than time.Now precisely so
// these cases can exist: a test that could only assert "some time ago" would pin
// nothing.
var now = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

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
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{4300, "4.2 KB"},
		{12288, "12 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{104857600, "100 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, c := range cases {
		got := formatBytes(c.size)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.size, got, c.want)
		}
	}
}

// Behavior: a modification time is rendered relative to now, coarsening as it
// recedes — the age of a file is what the user is judging, and "3 min ago" answers
// that where a timestamp would have to be subtracted first. Past roughly a month
// the relative form stops helping ("47 days ago" is not a date anyone can place),
// so it gives way to the absolute one.
func TestRelativeTime(t *testing.T) {
	cases := []struct {
		name string
		when time.Time
		want string
	}{
		{"seconds ago", now.Add(-30 * time.Second), "just now"},
		{"one minute", now.Add(-time.Minute), "1 min ago"},
		{"minutes", now.Add(-3 * time.Minute), "3 min ago"},
		{"under an hour", now.Add(-59 * time.Minute), "59 min ago"},
		{"one hour", now.Add(-time.Hour), "1 hour ago"},
		{"hours", now.Add(-5 * time.Hour), "5 hours ago"},
		{"one day", now.Add(-24 * time.Hour), "1 day ago"},
		{"days", now.Add(-3 * 24 * time.Hour), "3 days ago"},
		{"a month out", now.Add(-40 * 24 * time.Hour), "2026-07-25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := relativeTime(c.when, now)
			if got != c.want {
				t.Errorf("relativeTime(%v) = %q, want %q", c.when, got, c.want)
			}
		})
	}
}

// Behavior: a timestamp in the future reads as "just now" rather than as a
// negative age. Clock skew between a local machine and a remote dev box is
// routine, and a file stamped a few seconds ahead is not worth a row that says
// "-4 min ago" — it is, for the user's purposes, current.
func TestRelativeTime_Future_ReadsAsJustNow(t *testing.T) {
	got := relativeTime(now.Add(time.Hour), now)

	if got != "just now" {
		t.Errorf("got %q, want %q for a future timestamp", got, "just now")
	}
}

// Behavior: an unreadable modification time arrives as the zero Time (see
// compare.parseModTime) and must render as nothing at all. Formatting it would
// put "year 1 ago" in the change list, which is worse than the silence it stands
// in for.
func TestRelativeTime_Zero_RendersNothing(t *testing.T) {
	got := relativeTime(time.Time{}, now)

	if got != "" {
		t.Errorf("got %q, want the empty string for a zero time", got)
	}
}

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

// Behavior: an update's detail carries the size, the age, and the label saying
// what differs, in that order and separated by a middot.
func TestActionDetail_Update_CarriesSizeAgeAndLabel(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "main.go", Size: 4300, ModTime: now.Add(-3 * time.Minute),
		Diff: compare.Difference{Content: true, Size: true, ModTime: true},
	}

	got := actionDetail(a, now)

	want := "4.2 KB · 3 min ago · size and mtime"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Behavior: a create carries its size and age but no difference label — there is
// no counterpart on the other side for it to differ from, so naming a difference
// would be a claim about a file that does not exist.
func TestActionDetail_Create_OmitsTheDifferenceLabel(t *testing.T) {
	a := compare.Action{Verb: "create", Path: "new.go", Size: 1126, ModTime: now.Add(-3 * time.Minute)}

	got := actionDetail(a, now)

	want := "1.1 KB · 3 min ago"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Behavior: a delete carries no detail at all. Its record's size and time columns
// describe nothing (compare drops them), and the file is being removed rather than
// inspected, so there is no measurement to report.
func TestActionDetail_Delete_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "delete", Path: "stale.txt"}

	got := actionDetail(a, now)

	if got != "" {
		t.Errorf("got %q, want the empty string for a delete", got)
	}
}

// Behavior: when the modification time is unreadable the detail keeps the parts it
// does have rather than rendering an empty or dangling separator. Metadata is an
// annotation, so losing one field must cost only that field.
func TestActionDetail_MissingModTime_KeepsTheRest(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "main.go", Size: 4300,
		Diff: compare.Difference{Content: true, Size: true},
	}

	got := actionDetail(a, now)

	want := "4.2 KB · size only"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
