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
