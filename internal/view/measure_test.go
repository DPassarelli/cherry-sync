package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCountRows(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		width int
		want  int
	}{
		{"empty is zero rows", "", 80, 0},
		{"two short lines", "Source: .\nDestination: host:/p\n", 80, 2},
		{"banner trailing blank counts", "cherry-sync\n\n", 80, 2},
		{"no trailing newline", "abc", 80, 1},
		{"long line wraps to the ceiling", strings.Repeat("x", 10), 4, 3},
		{"exact multiple of width does not over-count", strings.Repeat("x", 8), 4, 2},
		{"unknown width falls back to line count", strings.Repeat("x", 10), 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := countRows(c.s, c.width)
			if got != c.want {
				t.Errorf("countRows(%q, %d) = %d, want %d", c.s, c.width, got, c.want)
			}
		})
	}
}

// TestCountRowsIgnoresANSI pins that measurement is by display cells, not bytes: a
// styled line whose escape codes make it far longer in bytes than in visible width
// must not be counted as wrapping. The preamble's values are bold, so a byte count
// would over-reserve and waste rows.
func TestCountRowsIgnoresANSI(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Render("Destination: host:/p") + "\n"
	if got := countRows(styled, 80); got != 1 {
		t.Errorf("countRows(styled, 80) = %d, want 1 (ANSI must not count toward width)", got)
	}
}
