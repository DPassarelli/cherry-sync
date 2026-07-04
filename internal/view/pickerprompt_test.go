package view

import "testing"

// TestPickerPrompt pins the picker's question line: it must state how many files
// are available to select, so a rendered-then-quiet list can't be mistaken for a
// freeze or a bug (issue #64). The count is the plain total on offer; the phrasing
// follows the issue's mockup and is not pluralized, so a lone file reads
// "(1 available)". Drop the count or format it wrong and a case goes red.
func TestPickerPrompt(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  string
	}{
		{"one file", 1, "Choose which files to sync (1 available):"},
		{"several files", 3, "Choose which files to sync (3 available):"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickerPrompt(c.count)
			if got != c.want {
				t.Errorf("pickerPrompt(%d) = %q, want %q", c.count, got, c.want)
			}
		})
	}
}
