// overlayframe.go lays out the diff overlay: the box drawn around it, and the
// header and footer pinned around whichever part of the diff is in view.

package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// frameStyle draws the box and its dividers, faint so they recede behind the diff.
var frameStyle = lipgloss.NewStyle().Faint(true)

// frameMargin is the columns the box takes from each row: a side and a space of
// padding at each end.
const frameMargin = 4

// overlayFooterRows is the divider, the key hint, and the box's bottom edge.
const overlayFooterRows = 3

// innerWidth is how many columns of content fit inside the box.
func (m pickerModel) innerWidth() int {
	return max(m.width-frameMargin, 1)
}

// frameEdge is a horizontal edge of the box, width columns wide, between the given
// corner pieces.
func frameEdge(left, right string, width int) string {
	return frameStyle.Render(left + strings.Repeat("─", max(width-2, 0)) + right)
}

// framed places one row of content between the box's sides, padded so the right
// side lines up whatever the row's width.
func (m pickerModel) framed(row string) string {
	pad := strings.Repeat(" ", max(m.innerWidth()-ansi.StringWidth(row), 0))
	side := frameStyle.Render("│")
	return side + " " + row + pad + " " + side
}

// framedRows wraps line to the box's inner width and frames each row.
func (m pickerModel) framedRows(line string) []string {
	var rows []string
	for _, row := range wrapRows(line, m.innerWidth()) {
		rows = append(rows, m.framed(row))
	}
	return rows
}

// overlayRows is how many diff lines fit between the header and the footer, leaving
// one row spare so the frame's trailing newline can't scroll its title away.
func (m pickerModel) overlayRows() int {
	if m.height <= 0 {
		return len(m.overlayRowsAll())
	}
	return max(m.height-1-len(m.overlayHeaderRows())-overlayFooterRows, 1)
}

// lastOverlayOffset is the furthest the overlay can scroll while staying full.
func (m pickerModel) lastOverlayOffset() int {
	return max(len(m.overlayRowsAll())-m.overlayRows(), 0)
}

// overlayView renders the header, the part of the diff in view, then the footer.
// The offset is limited again here because a resize can move the end of the diff
// after it was stored. A short diff is padded so the box's bottom stays on the
// bottom row rather than jumping between diffs.
func (m pickerModel) overlayView() string {
	rows := m.overlayRowsAll()
	first := min(m.overlayOffset, m.lastOverlayOffset())
	last := min(first+m.overlayRows(), len(rows))
	body := rows[first:last]
	if m.height > 0 {
		for len(body) < m.overlayRows() {
			body = append(body, "")
		}
	}
	lines := m.overlayHeaderRows()
	for _, row := range body {
		lines = append(lines, m.framed(row))
	}
	lines = append(lines,
		frameEdge("├", "┤", m.width),
		m.framed(frameStyle.Render("↑/↓ scroll · esc back")),
		frameEdge("└", "┘", m.width),
	)
	return strings.Join(lines, "\n") + "\n"
}
