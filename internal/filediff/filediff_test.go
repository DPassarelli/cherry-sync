package filediff

import (
	"strings"
	"testing"
)

// A local copy is fetched first, whichever side of the sync it is on.
func TestFetchOrder_LocalFirst(t *testing.T) {
	cases := []struct {
		source, destination string
		first               Copy
	}{
		{"./project", "host:/project", SourceCopy},
		{"host:/project", "./project", DestinationCopy},
		{"./a", "./b", SourceCopy},
		{"one:/a", "two:/b", SourceCopy},
	}
	for _, c := range cases {
		got := fetchOrder(c.source, c.destination)[0].copy
		if got != c.first {
			t.Errorf("fetchOrder(%q, %q) starts with %v, want %v", c.source, c.destination, got, c.first)
		}
	}
}

// A NUL byte anywhere marks a copy as binary, not just one near the start.
func TestIsBinary(t *testing.T) {
	if isBinary([]byte("plain text\n")) {
		t.Error("plain text reported as binary")
	}
	if !isBinary([]byte("text then \x00 a NUL")) {
		t.Error("content with a NUL byte not reported as binary")
	}
}

// The hunk header counts the lines in each copy, so a one-line file must not be
// reported as two.
func TestUnified_CountsLinesExactly(t *testing.T) {
	got, err := unified([]byte("old\n"), []byte("new\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "@@ -1 +1 @@") {
		t.Errorf("hunk header wrong in:\n%s", got)
	}
}

// The diff opens with its first hunk, since a header naming each copy would only
// repeat the full paths the overlay already shows above it.
func TestUnified_OpensWithTheHunk(t *testing.T) {
	got, err := unified([]byte("old\n"), []byte("new\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "@@ ") {
		t.Errorf("diff opens with something before its hunk:\n%s", got)
	}
}

// A newline added at the end of a file is a real difference, and the diff has to say
// so in the form diff readers expect, rather than running the two lines together.
func TestUnified_ShowsAMissingFinalNewline(t *testing.T) {
	got, err := unified([]byte("same"), []byte("same\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "-same\n\\ No newline at end of file\n+same\n"
	if !strings.Contains(got, want) {
		t.Errorf("got:\n%s\nwant it to contain:\n%s", got, want)
	}
}

// Splitting must not invent a line, or the diff shows a blank line the file lacks.
func TestSplitLines_NoInventedLine(t *testing.T) {
	cases := map[string]int{"": 0, "a\n": 1, "a\nb\n": 2, "a": 1}
	for content, want := range cases {
		got := len(splitLines([]byte(content)))
		if got != want {
			t.Errorf("splitLines(%q) gave %d lines, want %d", content, got, want)
		}
	}
}
