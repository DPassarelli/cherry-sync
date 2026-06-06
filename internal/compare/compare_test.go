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

// The tests below pin the individual ordering rules of comparePaths, one rule
// per pair. The holistic, end-to-end ordering is covered by the scenario in
// features/order-reported-actions.feature; these isolate each rule (and edge
// cases the feature fixture doesn't contain, like 01 vs 1) so a failure points
// straight at the rule that broke.

// assertSortsBefore checks that comparePaths orders "before" ahead of "after",
// and is antisymmetric — the reverse comparison must be positive. The reverse
// check stops a degenerate comparator (e.g. one that always returns -1) from
// passing.
func assertSortsBefore(t *testing.T, before, after string) {
	t.Helper()
	got := comparePaths(before, after)
	if got >= 0 {
		t.Errorf("comparePaths(%q, %q) = %d, want < 0", before, after, got)
	}
	gotReverse := comparePaths(after, before)
	if gotReverse <= 0 {
		t.Errorf("comparePaths(%q, %q) = %d, want > 0", after, before, gotReverse)
	}
}

// Rule 1: dot entries sort before non-dot entries.
func TestComparePaths_DotFileBeforeNonDotFile(t *testing.T) {
	assertSortsBefore(t, ".gitignore", "README.md")
}

// Rule 2: within a group, files sort before subdirectories — here both are dot.
func TestComparePaths_DotFileBeforeDotDirectory(t *testing.T) {
	assertSortsBefore(t, ".gitignore", ".config/settings.toml")
}

// Rule 2: a top-level file sorts before any subdirectory's contents.
func TestComparePaths_FileBeforeSubdirectory(t *testing.T) {
	assertSortsBefore(t, "main.go", "src/adder.go")
}

// Rule 3: number-leading names sort before letter-leading names.
func TestComparePaths_NumberLeadingBeforeLetterLeading(t *testing.T) {
	assertSortsBefore(t, "2-config.yml", "LICENSE")
}

// Rule 4: numbers compare by value, not lexically (lexical would put 10 first).
func TestComparePaths_NumbersCompareByValue(t *testing.T) {
	assertSortsBefore(t, "2-config.yml", "10-data.csv")
}

// Rule 4: equal numeric value falls back to byte order, so 01 precedes 1.
func TestComparePaths_LeadingZeroBeforeBareNumber(t *testing.T) {
	assertSortsBefore(t, "01-setup.md", "1-setup.md")
}

// Rule 4: letters compare case-insensitively (a byte sort would put README,
// uppercase R, ahead of main, lowercase m).
func TestComparePaths_LettersAreCaseInsensitive(t *testing.T) {
	assertSortsBefore(t, "main.go", "README.md")
}

// Rule 4: a case-only tie falls back to byte order, so uppercase precedes lower.
func TestComparePaths_UpperBeforeLowerOnCaseTie(t *testing.T) {
	assertSortsBefore(t, "TODO.md", "todo.md")
}

// The rules apply at each level: equal parent directories are descended into.
func TestComparePaths_DescendsIntoSubdirectory(t *testing.T) {
	assertSortsBefore(t, "src/adder.go", "src/parser.go")
}
