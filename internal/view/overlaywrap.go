// overlaywrap.go fits the overlay's lines to the terminal's width, so nothing in a
// diff or an error is hidden past the right edge, and draws tabs at a width it can
// measure.

package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wrapIndicator opens each row that continues a line too wide for the terminal. It
// keeps a wrapped diff line from reading as a new one without its "+" or "-".
const wrapIndicator = "↪ "

// tabMark is how a tab is drawn: a fixed width the wrapping can measure, where a real
// tab's width depends on where it falls, and visibly not text a file could contain.
const tabMark = " →"

// showTabs replaces each tab in line with a dimmed tabMark.
func showTabs(line string) string {
	return strings.ReplaceAll(line, "\t", lipgloss.NewStyle().Faint(true).Render(tabMark))
}

// wrapRows breaks line into rows of at most width columns. It breaks at the exact
// column rather than at a space, so code wraps exactly as written.
func wrapRows(line string, width int) []string {
	room := width - ansi.StringWidth(wrapIndicator)
	total := ansi.StringWidth(line)
	if room <= 0 || total <= width {
		return []string{line}
	}
	indicator := lipgloss.NewStyle().Faint(true).Render(wrapIndicator)
	rows := []string{ansi.Cut(line, 0, width)}
	for start := width; start < total; start += room {
		rows = append(rows, indicator+ansi.Cut(line, start, min(start+room, total)))
	}
	return rows
}

// overlayRowsAll is every row of the overlay at the terminal's current width.
func (m pickerModel) overlayRowsAll() []string {
	var rows []string
	for _, line := range m.overlay {
		rows = append(rows, wrapRows(showTabs(line), m.width)...)
	}
	return rows
}
