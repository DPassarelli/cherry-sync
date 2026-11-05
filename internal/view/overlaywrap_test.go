package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// A line that fits stays one row, untouched.
func TestWrapRows_FittingLineIsOneRow(t *testing.T) {
	got := wrapRows("short", 10)
	if len(got) != 1 || got[0] != "short" {
		t.Errorf("wrapRows = %q, want [short]", got)
	}
}

// A wide line breaks at the exact column, every continuation opens with the
// indicator, and no row is wider than the terminal.
func TestWrapRows_BreaksAtTheColumn(t *testing.T) {
	line := strings.Repeat("abcdefghij", 3)
	got := wrapRows(line, 12)
	var rebuilt strings.Builder
	for i, row := range got {
		plain := ansi.Strip(row)
		if ansi.StringWidth(plain) > 12 {
			t.Errorf("row %d is %d wide, over 12: %q", i, ansi.StringWidth(plain), plain)
		}
		if i > 0 && !strings.HasPrefix(plain, wrapIndicator) {
			t.Errorf("row %d lacks the indicator: %q", i, plain)
		}
		rebuilt.WriteString(strings.TrimPrefix(plain, wrapIndicator))
	}
	if rebuilt.String() != line {
		t.Errorf("rows rebuild to %q, want %q", rebuilt.String(), line)
	}
	if ansi.Strip(got[0]) != line[:12] {
		t.Errorf("first row = %q, want the first 12 columns", ansi.Strip(got[0]))
	}
}

// Before the first resize the width is unknown, and a terminal too narrow for the
// indicator leaves no room to continue, so neither wraps.
func TestWrapRows_NoUsableWidth(t *testing.T) {
	for _, width := range []int{0, ansi.StringWidth(wrapIndicator)} {
		got := wrapRows("abcdefghij", width)
		if len(got) != 1 {
			t.Errorf("wrapRows at width %d = %q, want one row", width, got)
		}
	}
}

// Scrolling counts rows, so a wrapped line's rows can all be reached.
func TestOverlayKey_ScrollsThroughWrappedRows(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 10, 6
	updated, _ := m.Update(diffFetchedMsg{text: strings.Repeat("x", 100)})
	m = updated.(pickerModel)
	if m.lastOverlayOffset() == 0 {
		t.Error("a line wrapped across many rows left nothing to scroll")
	}
}

// A tab is drawn at a fixed two columns, so wrapping measures the row as the
// terminal will show it.
func TestShowTabs_FixedWidth(t *testing.T) {
	got := showTabs("a\t\tb")
	if ansi.Strip(got) != "a → →b" {
		t.Errorf("showTabs = %q, want %q", ansi.Strip(got), "a → →b")
	}
}
