package compare

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

// stamp is the modification time every fixture line below carries, as the Time
// parseActions is expected to produce for it. Held in one place so a test asserts
// the field it cares about without restating the layout.
var stamp = time.Date(2026, 9, 3, 17, 32, 53, 0, time.Local)

// stampText is stamp in rsync's %M rendering — the text side of the same moment.
const stampText = "2026/09/03-17:32:53"

// line assembles one rsync --out-format='%i|%l|%M|%n' record, so a test states
// only the code, size, and path it is about.
func line(code string, size int64, path string) string {
	return code + "|" + itoa(size) + "|" + stampText + "|" + path + "\n"
}

// itoa renders n in base 10.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// Behavior: with no rsync output, parseActions returns no actions. Mirrors
// the Gherkin scenario "None of the files are different" in
// features/compare-directories.feature.
func TestParseActions_Empty_ReturnsNoActions(t *testing.T) {
	got := parseActions("")

	if len(got) != 0 {
		t.Errorf("got %d actions, want 0", len(got))
	}
}

// Behavior: a single `>f...` update line yields a single update Action.
// Mirrors the Gherkin scenario "One of the files is different" in
// features/compare-directories.feature.
func TestParseActions_OneUpdate_ReturnsOneUpdateAction(t *testing.T) {
	got := parseActions(line(">fcst......", 42, "README.md"))

	want := []Action{{
		Verb: "update", Path: "README.md", Size: 42, ModTime: stamp,
		Diff: Difference{Content: true, Size: true, ModTime: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a `>f+++++++++` line (all-new attribute markers) yields a create
// Action. Mirrors the new-file half of the Gherkin scenario "Two of the files
// are different" in features/compare-directories.feature.
func TestParseActions_OneCreate_ReturnsOneCreateAction(t *testing.T) {
	got := parseActions(line(">f+++++++++", 10, "src/adder.go"))

	want := []Action{{Verb: "create", Path: "src/adder.go", Size: 10, ModTime: stamp}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: the itemize code's width is implementation-specific — GNU rsync
// emits 11 chars, macOS's openrsync 9 (two fewer attribute columns). The two
// tests below pin parseActions to openrsync's actual byte layout, so the parser
// can't regress to assuming GNU's field widths.

// Behavior: an openrsync update line (9-char code) yields an update Action with
// the path intact — no leading byte eaten.
func TestParseActions_OpenrsyncUpdate_KeepsFirstPathByte(t *testing.T) {
	got := parseActions(line(">fcst....", 42, "after-merge.sh"))

	want := []Action{{
		Verb: "update", Path: "after-merge.sh", Size: 42, ModTime: stamp,
		Diff: Difference{Content: true, Size: true, ModTime: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: an openrsync new-file line marks every attribute column '+' — seven
// of them, not GNU's nine — and must still be recognized as a create, not an
// update. A fixed nine-'+' check would misread this as an update.
func TestParseActions_OpenrsyncCreate_ReturnsCreateAction(t *testing.T) {
	got := parseActions(line(">f+++++++", 10, "after-merge.sh"))

	want := []Action{{Verb: "create", Path: "after-merge.sh", Size: 10, ModTime: stamp}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: rsync marks the transfer direction in the first byte of the itemize
// code — `>` for files received into the destination (pull / local-to-local),
// `<` for files sent to a remote (a real push over SSH). The godog harness only
// ever runs local-to-local, which always emits `>`, so the `<` direction is
// invisible to it; a real push emits `<f` and a `>`-only parser drops every line.

// Behavior: a push update line (`<f`) yields an update Action, the same as its
// `>f` pull counterpart — the leading byte is direction, not a different change.
func TestParseActions_PushUpdate_ReturnsUpdateAction(t *testing.T) {
	got := parseActions(line("<fcs.......", 42, "README.md"))

	want := []Action{{
		Verb: "update", Path: "README.md", Size: 42, ModTime: stamp,
		Diff: Difference{Content: true, Size: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a push new-file line (`<f` with an all-`+` attribute run) yields a
// create Action, just as the `>f+++` pull form does.
func TestParseActions_PushCreate_ReturnsCreateAction(t *testing.T) {
	got := parseActions(line("<f+++++++++", 7, "newfile.txt"))

	want := []Action{{Verb: "create", Path: "newfile.txt", Size: 7, ModTime: stamp}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: with --delete on the compare, a file present on the destination but
// gone from the source itemizes as `*deleting <path>` — a distinct shape from
// the `<f`/`>f` transfer codes (a word, not a fixed-width flag run, followed by
// padding then the path). It must yield a delete Action so the picker can offer
// the removal as a red row. GNU pads `*deleting` within an 11-char code column;
// openrsync's 9-char column holds it flush. Both must land the same path.
func TestParseActions_GNUDelete_ReturnsDeleteAction(t *testing.T) {
	got := parseActions(line("*deleting  ", 0, "gone.txt"))

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: openrsync's `*deleting` line — the 9-char code column holds the word
// exactly, so no padding follows it. Parses to the same delete Action.
func TestParseActions_OpenrsyncDelete_ReturnsDeleteAction(t *testing.T) {
	got := parseActions(line("*deleting", 0, "gone.txt"))

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: rsync reports a length of 0 and an epoch modification time on a
// `*deleting` line — the fields describe nothing, since the file is absent from
// the side being read. They must not reach the Action as though they were real
// measurements, or the change list would offer to remove a "0 B" file last
// touched in 1969. Size and ModTime stay zero.
func TestParseActions_Delete_CarriesNoMetadata(t *testing.T) {
	got := parseActions("*deleting  |0|1969/12/31-19:00:00|gone.txt\n")

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: openrsync's dry-run itemize prints a `*deleting` line for the same
// path twice (GNU rsync emits it once), so a single stale file would otherwise be
// reported — and counted — as two removals. parseActions collapses identical
// (verb, path) actions to one, while leaving distinct deletions intact.
func TestParseActions_DuplicateDelete_IsReportedOnce(t *testing.T) {
	got := parseActions(line("*deleting  ", 0, "gone.txt") + line("*deleting  ", 0, "gone.txt") + line("*deleting  ", 0, "other.txt"))

	want := []Action{{Verb: "delete", Path: "gone.txt"}, {Verb: "delete", Path: "other.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: the duplicate collapse keys on the verb and path alone. Metadata is
// not part of the identity of a planned change — the same file cannot be created
// twice — so two records for one path collapse even where their size or timestamp
// disagree. Folding the metadata into the key would let a byte of drift between
// two lines resurrect the duplicate this guards against.
func TestParseActions_DuplicateWithDifferentMetadata_IsReportedOnce(t *testing.T) {
	got := parseActions(line(">fcst......", 42, "a.txt") + ">fcst......|99|2020/01/01-00:00:00|a.txt\n")

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1: %+v", len(got), got)
	}
	if got[0].Size != 42 {
		t.Errorf("got Size %d, want 42 (the first record wins)", got[0].Size)
	}
}

// Behavior: a delete candidate whose name contains an rsync filter glob
// metacharacter (`*`, `?`, or `[`) is dropped, not reported — applying it would
// mean handing that name to the deletion filter, where the metacharacter could
// match and remove the wrong file.
func TestParseActions_DeleteWithGlobMeta_IsDropped(t *testing.T) {
	for _, name := range []string{"a*.txt", "a?.txt", "a[1].txt"} {
		got := parseActions(line("*deleting  ", 0, name))
		if len(got) != 0 {
			t.Errorf("delete of %q: got %+v, want no actions (dropped)", name, got)
		}
	}
}

// Behavior: the attribute columns say what differs, and the four combinations
// below are the only ones csync's flag set can produce for an update (verified
// against GNU rsync 3.4.1). The `c` column is always set, because the comparison
// runs with --checksum; `s` and `t` are what vary. A parser that read the columns
// positionally from the right, or ignored them, would report the same Difference
// for every one of these.
func TestParseActions_UpdateCodes_DecodeWhatDiffers(t *testing.T) {
	cases := []struct {
		name string
		code string
		want Difference
	}{
		{"size and mtime differ", ">fcst......", Difference{Content: true, Size: true, ModTime: true}},
		{"mtime differs, size matches", ">fc.t......", Difference{Content: true, ModTime: true}},
		{"size differs, mtime matches", ">fcs.......", Difference{Content: true, Size: true}},
		{"neither differs, contents do", ">fc........", Difference{Content: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseActions(line(c.code, 42, "a.txt"))
			if len(got) != 1 {
				t.Fatalf("got %d actions, want 1", len(got))
			}
			if got[0].Diff != c.want {
				t.Errorf("code %q: got %+v, want %+v", c.code, got[0].Diff, c.want)
			}
		})
	}
}

// Behavior: rsync writes an uppercase `T` in the time column when it would set
// the destination's timestamp to the transfer time rather than the source's
// (as under --size-only), where a lowercase `t` means it would copy the source's.
// Both say the timestamps do not currently agree, so both must decode as a
// modification-time difference. A test for `t` alone would let a `T`-blind parser
// pass while silently reporting "contents only" for a file whose time differs.
func TestParseActions_UppercaseTimeColumn_CountsAsModTimeDifference(t *testing.T) {
	got := parseActions(line(">fcsT......", 42, "a.txt"))

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1", len(got))
	}
	if !got[0].Diff.ModTime {
		t.Errorf("got Diff %+v, want ModTime true for an uppercase T column", got[0].Diff)
	}
}

// Behavior: a create marks every attribute column '+', which says the file is
// new rather than naming an attribute that differs. Decoding those columns as
// differences would report a brand-new file as differing in size and time
// against a counterpart that does not exist, so a create's Difference stays zero.
func TestParseActions_Create_CarriesNoDifference(t *testing.T) {
	got := parseActions(line(">f+++++++++", 10, "new.txt"))

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1", len(got))
	}
	if (got[0].Diff != Difference{}) {
		t.Errorf("got Diff %+v, want the zero Difference for a create", got[0].Diff)
	}
}

// Behavior: `|` separates the record's fields and `%n` is its last, so a filename
// containing a pipe belongs wholly to the path — it cannot be read as the start of
// another column. A split that took the last field rather than the remainder would
// truncate this name to "odd" and then look for a file that does not exist.
func TestParseActions_PathContainingPipe_IsKeptWhole(t *testing.T) {
	got := parseActions(line(">fcst......", 42, "weird|name.txt"))

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1: %+v", len(got), got)
	}
	if got[0].Path != "weird|name.txt" {
		t.Errorf("got Path %q, want %q", got[0].Path, "weird|name.txt")
	}
}

// Behavior: the -vv verbose output rsync prints alongside the records (the lines
// naming what an --exclude held back, and its transfer statistics) carries no
// pipe-delimited record shape and must be skipped. These are real lines captured
// from a -vv run; one of them mentioning a path must not become an action.
func TestParseActions_VerboseChatter_IsIgnored(t *testing.T) {
	out := "sending incremental file list\n" +
		"[sender] hiding directory .git because of pattern .git\n" +
		"delta-transmission disabled for local transfer or --whole-file\n" +
		".d         |5|2026/09/03-17:53:58|./\n" +
		line(">f+++++++++", 2, "keep.txt") +
		"total: matches=0  hash_hits=0  false_alarms=0 data=0\n" +
		"sent 82 bytes  received 85 bytes  334.00 bytes/sec\n"

	got := parseActions(out)

	want := []Action{{Verb: "create", Path: "keep.txt", Size: 2, ModTime: stamp}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a record whose size or timestamp field is unreadable still yields its
// action. The verb and path are what a sync acts on; the metadata only annotates
// the row, so losing it must cost the user the annotation, never the change
// itself. An rsync that formats either field differently than expected would
// otherwise make files silently undeletable and unsyncable.
func TestParseActions_UnparsableMetadata_StillYieldsTheAction(t *testing.T) {
	got := parseActions(">fcst......|not-a-number|not-a-time|a.txt\n")

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1: %+v", len(got), got)
	}
	if got[0].Verb != "update" || got[0].Path != "a.txt" {
		t.Errorf("got %+v, want an update of a.txt", got[0])
	}
	if got[0].Size != 0 || !got[0].ModTime.IsZero() {
		t.Errorf("got Size %d / ModTime %v, want both zero", got[0].Size, got[0].ModTime)
	}
}
