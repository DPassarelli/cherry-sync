package compare

import (
	"reflect"
	"testing"
)

// Behavior: rsyncArgs ends with a `--` end-of-options separator immediately
// before the source and destination paths. This is what stops a path that
// begins with `-` (e.g. `-e`, `--rsh=touch /tmp/pwned`) from being parsed by
// rsync as an OPTION rather than a path — rsync's `-e`/`--rsh` can run an
// arbitrary remote shell command. Guards against rsync argument injection.
func TestRsyncArgs_SeparatesOptionsFromPaths(t *testing.T) {
	got := rsyncArgs("-e/evil", "./dest")

	tail := got[len(got)-3:]
	want := []string{"--", "-e/evil/", "./dest/"}
	if !reflect.DeepEqual(tail, want) {
		t.Errorf("arg tail: got %+v, want %+v", tail, want)
	}
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
	if got := comparePaths(before, after); got >= 0 {
		t.Errorf("comparePaths(%q, %q) = %d, want < 0", before, after, got)
	}
	if got := comparePaths(after, before); got <= 0 {
		t.Errorf("comparePaths(%q, %q) = %d, want > 0", after, before, got)
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
