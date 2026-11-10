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

// thumbStyle draws the scrollbar's thumb at full brightness and bold, since in the
// box's faint style it barely stood out from the border it sits in.
var thumbStyle = lipgloss.NewStyle().Bold(true)

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
	return m.framedBeside(row, frameStyle.Render("│"))
}

// framedBeside is framed with right, already styled, as the box's right side, which
// is where the scrollbar's thumb is drawn so it costs the diff no width.
func (m pickerModel) framedBeside(row, right string) string {
	pad := strings.Repeat(" ", max(m.innerWidth()-ansi.StringWidth(row), 0))
	return frameStyle.Render("│") + " " + row + pad + " " + right
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
	bar := scrollThumb(m.overlayRows(), len(rows), first)
	lines := m.overlayHeaderRows()
	for i, row := range body {
		right := frameStyle.Render("│")
		if i >= bar.start && i < bar.start+bar.size {
			right = thumbStyle.Render("┃")
		}
		lines = append(lines, m.framedBeside(row, right))
	}
	lines = append(lines,
		frameEdge("├", "┤", m.width),
		m.framed(frameStyle.Render(m.overlayHint(len(rows)))),
		frameEdge("└", "┘", m.width),
	)
	return strings.Join(lines, "\n") + "\n"
}

// overlayHint offers scrolling only when the diff's total rows are more than fit,
// since offering a key that does nothing would read as a sign something is hidden.
func (m pickerModel) overlayHint(total int) string {
	if total > m.overlayRows() {
		return "↑/↓ scroll · esc back"
	}
	return "esc back"
}

// thumb is the run of the diff's rows the scrollbar marks: size rows from start.
type thumb struct {
	start int
	size  int
}

// scrollThumb places the thumb for a view of rows rows, offset rows into a diff of
// total rows: as tall as the share of the diff in view, but never so short it
// vanishes, and moved through the rest of the track in step with the offset, so it
// reaches the bottom exactly when the view does. A diff that fits has no thumb.
func scrollThumb(rows, total, offset int) thumb {
	if total <= rows {
		return thumb{}
	}
	size := max(rows*rows/total, 1)
	travel := total - rows
	start := (offset*(rows-size)*2 + travel) / (2 * travel)
	return thumb{start: start, size: size}
}
