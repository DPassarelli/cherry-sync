package view

import (
	"testing"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

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

	got := actionDetail(a)

	if got != "" {
		t.Errorf("got %q, want the empty string for a create", got)
	}
}

// Behavior: a delete carries no detail either. The file is being removed rather
// than compared, so there is nothing to measure it against.
func TestActionDetail_Delete_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "delete", Path: "stale.txt"}

	got := actionDetail(a)

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

// Behavior: a delta reads as the incoming copy measured against the one it would
// replace, so a positive size is "larger" and a positive age difference "newer".
// The negative cases are the ones that matter most — they say the sync would
// overwrite a newer file — so both signs are pinned.
func TestDeltaLabel_Signs(t *testing.T) {
	cases := []struct {
		name  string
		delta compare.Delta
		want  string
	}{
		{"bigger and newer", compare.Delta{Known: true, Size: 4300, Time: 3 * 24 * time.Hour}, "4.2 KB larger · 3d newer"},
		{"smaller and older", compare.Delta{Known: true, Size: -4300, Time: -3 * 24 * time.Hour}, "4.2 KB smaller · 3d older"},
		{"newer, same size", compare.Delta{Known: true, Time: 4 * time.Minute}, "4m newer"},
		{"larger, same time", compare.Delta{Known: true, Size: 4096}, "4.0 KB larger"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deltaLabel(c.delta)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Behavior: a measured delta of zero on both attributes means the two copies agree
// in size and timestamp yet differ in content — the state a reader is most likely
// to take for a bug, and the one case where naming the content is the whole point.
func TestDeltaLabel_BothZero_NamesTheContents(t *testing.T) {
	got := deltaLabel(compare.Delta{Known: true})

	if got != "contents only" {
		t.Errorf("got %q, want %q", got, "contents only")
	}
}

// Behavior: an update whose delta could not be measured falls back to the itemize
// labels. The destination is not always reachable, and the row must still say what
// it can rather than going blank or claiming a measurement it never made.
func TestActionDetail_UnknownDelta_FallsBackToTheItemizeLabel(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "main.go",
		Diff: compare.Difference{Content: true, Size: true, ModTime: true},
	}

	got := actionDetail(a)

	if got != "size and mtime" {
		t.Errorf("got %q, want the itemize fallback %q", got, "size and mtime")
	}
}

// Behavior: a measured delta is preferred over the itemize label, since it says
// how much and in which direction where the label says only which attributes.
func TestActionDetail_KnownDelta_PrefersTheNumbers(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "main.go",
		Diff:  compare.Difference{Content: true, Size: true, ModTime: true},
		Delta: compare.Delta{Known: true, Size: 4300, Time: 3 * 24 * time.Hour},
	}

	got := actionDetail(a)

	if got != "4.2 KB larger · 3d newer" {
		t.Errorf("got %q, want the measured delta", got)
	}
}
