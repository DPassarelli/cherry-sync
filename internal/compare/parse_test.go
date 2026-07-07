package compare

import (
	"reflect"
	"testing"
)

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
	got := parseActions(">f.st...... README.md\n")

	want := []Action{{Verb: "update", Path: "README.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a `>f+++++++++` line (all-new attribute markers) yields a create
// Action. Mirrors the new-file half of the Gherkin scenario "Two of the files
// are different" in features/compare-directories.feature.
func TestParseActions_OneCreate_ReturnsOneCreateAction(t *testing.T) {
	got := parseActions(">f+++++++++ src/adder.go\n")

	want := []Action{{Verb: "create", Path: "src/adder.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: the itemize code's width is implementation-specific — GNU rsync
// emits 11 chars, macOS's openrsync 9 (two fewer attribute columns). The two
// tests below pin parseActions to openrsync's actual byte layout (captured from
// `rsync --itemize-changes` under `openrsync: protocol version 29`), so the
// parser can't regress to assuming GNU's field widths. A fixed-offset parser
// would slice one byte into the path here, dropping its first character.

// Behavior: an openrsync update line (9-char code) yields an update Action with
// the path intact — no leading byte eaten.
func TestParseActions_OpenrsyncUpdate_KeepsFirstPathByte(t *testing.T) {
	got := parseActions(">f.st.... after-merge.sh\n")

	want := []Action{{Verb: "update", Path: "after-merge.sh"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: an openrsync new-file line marks every attribute column '+' — seven
// of them, not GNU's nine — and must still be recognized as a create, not an
// update. A fixed nine-'+' check would misread this as an update.
func TestParseActions_OpenrsyncCreate_ReturnsCreateAction(t *testing.T) {
	got := parseActions(">f+++++++ after-merge.sh\n")

	want := []Action{{Verb: "create", Path: "after-merge.sh"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: rsync marks the transfer direction in the first byte of the
// itemize code — `>` for files received into the destination (pull /
// local-to-local), `<` for files sent to a remote (a real push over SSH). The
// godog harness only ever runs local-to-local, which always emits `>`, so the
// `<` direction is invisible to it; a real push (`csync ./local host:/remote`)
// emits `<f` and a `>`-only parser drops every line — reporting zero changes
// and syncing nothing. The two tests below pin parseActions to the `<f` bytes,
// captured from `rsync --itemize-changes` pushing to a remote (GNU rsync 3.4.1).

// Behavior: a push update line (`<f`) yields an update Action, the same as its
// `>f` pull counterpart — the leading byte is direction, not a different change.
func TestParseActions_PushUpdate_ReturnsUpdateAction(t *testing.T) {
	got := parseActions("<f.s....... README.md\n")

	want := []Action{{Verb: "update", Path: "README.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a push new-file line (`<f` with an all-`+` attribute run) yields a
// create Action, just as the `>f+++` pull form does.
func TestParseActions_PushCreate_ReturnsCreateAction(t *testing.T) {
	got := parseActions("<f+++++++++ newfile.txt\n")

	want := []Action{{Verb: "create", Path: "newfile.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: with --delete on the compare, a file present on the destination but
// gone from the source itemizes as `*deleting <path>` — a distinct shape from
// the `<f`/`>f` transfer codes (a word, not a fixed-width flag run, followed by
// padding then the path). It must yield a delete Action so the picker can offer
// the removal as a red row. The two forms below pin both rsync layouts: GNU pads
// `*deleting` (9 chars) within an 11-char code column, so three spaces precede
// the path; openrsync's 9-char column leaves `*deleting` flush against a single
// separator space. Both must land the same path with no leading byte eaten.
// Captured from `rsync -rn --delete --itemize-changes` (GNU rsync 3.4.1).
func TestParseActions_GNUDelete_ReturnsDeleteAction(t *testing.T) {
	got := parseActions("*deleting   gone.txt\n")

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: openrsync's `*deleting` line — the 9-char code column holds the word
// exactly, so a single separator space sits between it and the path. Parses to
// the same delete Action as the GNU form above.
func TestParseActions_OpenrsyncDelete_ReturnsDeleteAction(t *testing.T) {
	got := parseActions("*deleting gone.txt\n")

	want := []Action{{Verb: "delete", Path: "gone.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: openrsync's dry-run itemize prints a `*deleting` line for the same
// path twice (GNU rsync emits it once), so a single stale file would otherwise be
// reported — and counted — as two removals. parseActions collapses identical
// (verb, path) actions to one, while leaving distinct deletions intact. Captured
// from `rsync -rn --delete --itemize-changes` under openrsync (macOS), which the
// GNU-only local smoke never exercised; a naive parser reports 3 actions here.
func TestParseActions_DuplicateDelete_IsReportedOnce(t *testing.T) {
	got := parseActions("*deleting   gone.txt\n*deleting   gone.txt\n*deleting   other.txt\n")

	want := []Action{{Verb: "delete", Path: "gone.txt"}, {Verb: "delete", Path: "other.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Behavior: a delete candidate whose name contains an rsync filter glob
// metacharacter (`*`, `?`, or `[`) is dropped, not reported — applying it would
// mean handing that name to the deletion filter, where the metacharacter could
// match and remove the wrong file. Escaping is deferred, so detection holds these
// back entirely. Each metacharacter is checked; a plain name is unaffected.
func TestParseActions_DeleteWithGlobMeta_IsDropped(t *testing.T) {
	for _, name := range []string{"a*.txt", "a?.txt", "a[1].txt"} {
		got := parseActions("*deleting   " + name + "\n")
		if len(got) != 0 {
			t.Errorf("delete of %q: got %+v, want no actions (dropped)", name, got)
		}
	}
}
