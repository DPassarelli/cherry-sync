package view

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
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

// Behavior: a row carries its annotation after the verb, so the list says what the
// file is as well as what would happen to it. This is the wiring test — the wording
// itself is pinned in detail_test.go — and it fails if contentLines renders the verb
// and stops.
func TestContentLines_RowCarriesItsDetail(t *testing.T) {
	m := newModel([]compare.Action{{
		Verb: "update", Path: "cmd/main.go",
		Delta: compare.Delta{Known: true, Size: 4300, Time: 3 * 24 * time.Hour},
	}})

	lines, _ := m.contentLines()

	row := strings.Join(lines, "\n")
	if !strings.Contains(row, "4.2 KB larger · 3d newer") {
		t.Errorf("rendered rows %q, want them to carry the measured delta", row)
	}
}

// Behavior: a delete row carries no annotation. Its rsync record holds no usable
// size or timestamp, so anything rendered there would be invented.
func TestContentLines_DeleteRowHasNoDetail(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "delete", Path: "stale.txt"}})

	lines, _ := m.contentLines()

	row := strings.Join(lines, "\n")
	if strings.Contains(row, "larger") || strings.Contains(row, "newer") || strings.Contains(row, "only") {
		t.Errorf("rendered rows %q, want no annotation on a delete", row)
	}
}

// Behavior: the annotation is trimmed to what the terminal can show, so a row never
// wraps onto a second terminal line. A wrapped row would still count as one line to
// the scroll window, sliding the window out of step with what was actually drawn —
// the bug class this guards, not merely an untidy display.
func TestContentLines_NarrowTerminal_KeepsRowsWithinTheWidth(t *testing.T) {
	m := newModel([]compare.Action{{
		Verb: "update", Path: "cmd/some-rather-long-name.go",
		Delta: compare.Delta{Known: true, Size: 4300, Time: 3 * 24 * time.Hour},
	}})
	m.width = 46

	lines, _ := m.contentLines()

	for _, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %q is %d cells wide, want at most %d", line, w, m.width)
		}
	}
}
