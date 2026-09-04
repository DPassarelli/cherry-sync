package compare

import (
	"testing"
	"time"
)

// at builds a time from rsync's %M layout, so a test states the timestamp the way
// rsync prints it.
func at(text string) time.Time {
	return parseModTime(text)
}

// Behavior: the reverse pass reports the destination's size and modification time
// for each path it was asked about, in the same `|`-delimited shape as the main
// compare minus the itemize code. Every field must survive: the size and the time
// are the two halves of the comparison, and the path is what joins them to an
// action.
func TestParseMetaLines_ReadsEveryField(t *testing.T) {
	got := parseMetaLines("13|2030/01/01-00:00:00|smaller.txt\n5|2020/01/01-00:00:00|same.txt\n")

	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got["smaller.txt"].size != 13 {
		t.Errorf("got size %d, want 13", got["smaller.txt"].size)
	}
	if !got["smaller.txt"].modTime.Equal(at("2030/01/01-00:00:00")) {
		t.Errorf("got modTime %v, want 2030-01-01", got["smaller.txt"].modTime)
	}
}

// Behavior: a path containing the field delimiter belongs wholly to the path,
// since it is the last field. Splitting from the right would truncate the name and
// then fail to match it against any action, silently costing that row its delta.
func TestParseMetaLines_PathContainingPipe_IsKeptWhole(t *testing.T) {
	got := parseMetaLines("5|2020/01/01-00:00:00|weird|name.txt\n")

	if _, ok := got["weird|name.txt"]; !ok {
		t.Errorf("got %+v, want an entry for %q", got, "weird|name.txt")
	}
}

// Behavior: rsync's verbose chatter and summary lines carry no readable
// metadata record and must not become entries. A line whose size or timestamp
// cannot be read is useless here — unlike the main compare, where an action
// survives without its metadata, there is nothing left of a metadata record
// once its fields are gone.
func TestParseMetaLines_Chatter_IsIgnored(t *testing.T) {
	got := parseMetaLines("sending incremental file list\n" +
		"5|2020/01/01-00:00:00|real.txt\n" +
		"not-a-size|not-a-time|bogus.txt\n" +
		"sent 82 bytes  received 85 bytes\n")

	if len(got) != 1 {
		t.Fatalf("got %d entries, want only the readable one: %+v", len(got), got)
	}
	if _, ok := got["real.txt"]; !ok {
		t.Errorf("got %+v, want the entry for real.txt", got)
	}
}

// Behavior: a delta is the source's copy measured against the destination's, so a
// source that is bigger and newer yields positive values in both. The sign is the
// whole meaning of the field — it is what the words "larger" and "newer" are read
// off — so a merge that took the absolute difference would render every row as
// though the incoming copy were the newer one.
func TestApplyDeltas_SourceAheadOfDestination_IsPositive(t *testing.T) {
	actions := []Action{{
		Verb: "update", Path: "a.txt", Size: 500, ModTime: at("2026/09/03-12:00:00"),
	}}
	dest := map[string]fileMeta{"a.txt": {size: 200, modTime: at("2026/09/01-12:00:00")}}

	got := applyDeltas(actions, dest)

	if !got[0].Delta.Known {
		t.Fatalf("got Delta %+v, want it known", got[0].Delta)
	}
	if got[0].Delta.Size != 300 {
		t.Errorf("got Size %d, want 300", got[0].Delta.Size)
	}
	if got[0].Delta.Time != 48*time.Hour {
		t.Errorf("got Time %v, want 48h", got[0].Delta.Time)
	}
}

// Behavior: a source behind the destination yields negative values. This is the
// case worth catching — it means syncing would overwrite the newer copy — so it
// must be representable rather than collapsed to the same reading as its opposite.
func TestApplyDeltas_SourceBehindDestination_IsNegative(t *testing.T) {
	actions := []Action{{
		Verb: "update", Path: "a.txt", Size: 200, ModTime: at("2026/09/01-12:00:00"),
	}}
	dest := map[string]fileMeta{"a.txt": {size: 500, modTime: at("2026/09/03-12:00:00")}}

	got := applyDeltas(actions, dest)

	if got[0].Delta.Size != -300 {
		t.Errorf("got Size %d, want -300", got[0].Delta.Size)
	}
	if got[0].Delta.Time != -48*time.Hour {
		t.Errorf("got Time %v, want -48h", got[0].Delta.Time)
	}
}

// Behavior: an update the destination pass said nothing about keeps an unknown
// delta. The reverse pass reports only what its quick check flags, so a file
// differing solely in content produces no record — and a pass that failed outright
// produces none at all. Either way the row must fall back to the itemize labels
// rather than render a delta of zero as though both sides had been measured.
func TestApplyDeltas_PathAbsentFromDestination_StaysUnknown(t *testing.T) {
	actions := []Action{{Verb: "update", Path: "a.txt", Size: 500, ModTime: at("2026/09/03-12:00:00")}}

	got := applyDeltas(actions, map[string]fileMeta{})

	if got[0].Delta.Known {
		t.Errorf("got Delta %+v, want it unknown", got[0].Delta)
	}
}

// Behavior: only an update carries a delta. A create has no counterpart on the
// destination and a delete is not being compared, so neither gets one even if the
// destination pass happened to report that path.
func TestApplyDeltas_NonUpdates_NeverCarryADelta(t *testing.T) {
	actions := []Action{
		{Verb: "create", Path: "a.txt", Size: 500, ModTime: at("2026/09/03-12:00:00")},
		{Verb: "delete", Path: "b.txt"},
	}
	dest := map[string]fileMeta{
		"a.txt": {size: 1, modTime: at("2026/09/01-12:00:00")},
		"b.txt": {size: 1, modTime: at("2026/09/01-12:00:00")},
	}

	got := applyDeltas(actions, dest)

	for _, a := range got {
		if a.Delta.Known {
			t.Errorf("%s of %s: got Delta %+v, want it unknown", a.Verb, a.Path, a.Delta)
		}
	}
}

// Behavior: only updates are worth asking the destination about, so the path list
// sent to the reverse pass holds those and nothing else. Including creates would
// ask for files that do not exist on the far side, and including deletes would ask
// about files being removed.
func TestUpdatePaths_ListsOnlyUpdates(t *testing.T) {
	actions := []Action{
		{Verb: "update", Path: "a.txt"},
		{Verb: "create", Path: "b.txt"},
		{Verb: "delete", Path: "c.txt"},
		{Verb: "update", Path: "d.txt"},
	}

	got := updatePaths(actions)

	want := []string{"a.txt", "d.txt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}
