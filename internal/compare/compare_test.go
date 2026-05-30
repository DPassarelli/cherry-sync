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
