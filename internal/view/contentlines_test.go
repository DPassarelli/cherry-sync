package view

import (
	"strings"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestContentLinesCursorLine pins the cursor-line mapping: contentLines must report
// the index of the cursor's row within the flattened lines, accounting for the
// blank-line + directory-heading rows that sit between groups. An off-by-one here
// is exactly the bug class that pushed the caret off-screen.
func TestContentLinesCursorLine(t *testing.T) {
	actions := []compare.Action{
		{Verb: "create", Path: "_features/a.feature"},
		{Verb: "create", Path: "_features/b.feature"},
		{Verb: "create", Path: "cmd/main.go"},
	}
	cases := []struct {
		name     string
		cursor   int
		basename string
	}{
		{"first row, first group", 0, "a.feature"},
		{"first row, second group", 2, "main.go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newModel(actions)
			m.cursor = c.cursor
			lines, cursorLine := m.contentLines()
			if cursorLine < 0 || cursorLine >= len(lines) {
				t.Fatalf("cursorLine = %d out of range [0,%d)", cursorLine, len(lines))
			}
			row := lines[cursorLine]
			if !strings.Contains(row, "❯") {
				t.Errorf("lines[%d] = %q, want it to carry the caret", cursorLine, row)
			}
			if !strings.Contains(row, c.basename) {
				t.Errorf("lines[%d] = %q, want the cursor row %q", cursorLine, row, c.basename)
			}
		})
	}
}
