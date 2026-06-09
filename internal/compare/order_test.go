package compare

import "testing"

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
