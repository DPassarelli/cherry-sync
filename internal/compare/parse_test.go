package compare

import (
	"reflect"
	"testing"
)

// line assembles one rsync --itemize-changes line, so a test states only the code
// and path it is about.
func line(code, path string) string {
	return code + " " + path + "\n"
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
	got := parseActions(line(">fcst......", "README.md"))

	want := []Action{{
		Verb: "update", Path: "README.md",
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
	got := parseActions(line(">f+++++++++", "src/adder.go"))

	want := []Action{{Verb: "create", Path: "src/adder.go"}}
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
	got := parseActions(line(">fcst....", "after-merge.sh"))

	want := []Action{{
		Verb: "update", Path: "after-merge.sh",
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
	got := parseActions(line(">f+++++++", "after-merge.sh"))

	want := []Action{{Verb: "create", Path: "after-merge.sh"}}
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
	got := parseActions(line("<fcs.......", "README.md"))

	want := []Action{{
		Verb: "update", Path: "README.md",
		Diff: Difference{Content: true, Size: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a push new-file line (`<f` with an all-`+` attribute run) yields a
// create Action, just as the `>f+++` pull form does.
func TestParseActions_PushCreate_ReturnsCreateAction(t *testing.T) {
	got := parseActions(line("<f+++++++++", "newfile.txt"))

	want := []Action{{Verb: "create", Path: "newfile.txt"}}
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
	got := parseActions(line("*deleting  ", "gone.txt"))

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: openrsync's `*deleting` line — the 9-char code column holds the word
// exactly, so no padding follows it. Parses to the same delete Action.
func TestParseActions_OpenrsyncDelete_ReturnsDeleteAction(t *testing.T) {
	got := parseActions(line("*deleting", "gone.txt"))

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
	got := parseActions(line("*deleting  ", "gone.txt") + line("*deleting  ", "gone.txt") + line("*deleting  ", "other.txt"))

	want := []Action{{Verb: "delete", Path: "gone.txt"}, {Verb: "delete", Path: "other.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: the duplicate collapse keys on the verb and path alone. What differs
// about a file is not part of the identity of a planned change — the same path
// cannot take the same verb twice — so two lines for one path collapse even where
// their attribute columns disagree. Folding the Difference into the key would let a
// byte of drift between two lines resurrect the duplicate this guards against.
func TestParseActions_DuplicateWithDifferingColumns_IsReportedOnce(t *testing.T) {
	got := parseActions(line(">fcst......", "a.txt") + line(">fc.t......", "a.txt"))

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1: %+v", len(got), got)
	}
	if !got[0].Diff.Size {
		t.Errorf("got Diff %+v, want the first line's columns to stand", got[0].Diff)
	}
}

// Behavior: a delete candidate whose name contains an rsync filter glob
// metacharacter (`*`, `?`, or `[`) is dropped, not reported — applying it would
// mean handing that name to the deletion filter, where the metacharacter could
// match and remove the wrong file.
func TestParseActions_DeleteWithGlobMeta_IsDropped(t *testing.T) {
	for _, name := range []string{"a*.txt", "a?.txt", "a[1].txt"} {
		got := parseActions(line("*deleting  ", name))
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
			got := parseActions(line(c.code, "a.txt"))
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
	got := parseActions(line(">fcsT......", "a.txt"))

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
	got := parseActions(line(">f+++++++++", "new.txt"))

	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1", len(got))
	}
	if (got[0].Diff != Difference{}) {
		t.Errorf("got Diff %+v, want the zero Difference for a create", got[0].Diff)
	}
}

// Behavior: the -vv verbose output rsync prints alongside the itemize lines (the
// lines naming what an --exclude held back, and its transfer statistics) matches
// none of the itemize shapes and must be skipped. These are real lines captured
// from a -vv run; one of them mentioning a path must not become an action.
func TestParseActions_VerboseChatter_IsIgnored(t *testing.T) {
	out := "sending incremental file list\n" +
		"[sender] hiding directory .git because of pattern .git\n" +
		"delta-transmission disabled for local transfer or --whole-file\n" +
		".d..t...... ./\n" +
		line(">f+++++++++", "keep.txt") +
		"total: matches=0  hash_hits=0  false_alarms=0 data=0\n" +
		"sent 82 bytes  received 85 bytes  334.00 bytes/sec\n"

	got := parseActions(out)

	want := []Action{{Verb: "create", Path: "keep.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
