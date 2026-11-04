package filediff

import "testing"

// A notice has to name exactly the copies that tripped its check.
func TestWhichCopies(t *testing.T) {
	cases := []struct {
		source, destination bool
		want                Copies
	}{
		{false, false, NoCopies},
		{true, false, SourceCopy},
		{false, true, DestinationCopy},
		{true, true, BothCopies},
	}
	for _, c := range cases {
		got := whichCopies(c.source, c.destination)
		if got != c.want {
			t.Errorf("whichCopies(%v, %v) = %v, want %v", c.source, c.destination, got, c.want)
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
