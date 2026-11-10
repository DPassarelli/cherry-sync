// overlaystyle.go draws what tells a reader of the diff overlay what they are
// looking at: the pinned title and header, and the colors tying each line to a side.

package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// Differ is what the picker needs to show a row's diff: the operands that name each
// side in the overlay's header, and how to fetch the row's two copies.
type Differ struct {
	Source      string
	Destination string
	Fetch       func(path string) (filediff.Result, error)
}

// Each side keeps one color in the header and in the diff, so the header doubles as
// the key to the lines below it. They are the picker's colors for a file arriving
// and a file being removed, which is what each kind of line amounts to.
var (
	sourceStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	destinationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hunkStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// overlayHeaderRows is every row of the overlay's pinned header at the terminal's
// current width: a blank row matching the spare one under the box, the box's top,
// the title naming the file, then each side's operand between dividers. The file's path is left to the title so the
// endpoint lines stay short.
func (m pickerModel) overlayHeaderRows() []string {
	rows := []string{"", frameEdge("┌", "┐", m.width)}
	rows = append(rows, m.framedRows(lipgloss.NewStyle().Bold(true).Render("Diff of "+m.overlayPath))...)
	rows = append(rows, frameEdge("├", "┤", m.width))
	rows = append(rows, m.framedRows(destinationStyle.Render(endpointLine("-", "Destination:", m.differ.Destination)))...)
	rows = append(rows, m.framedRows(sourceStyle.Render(endpointLine("+", "Source:", m.differ.Source)))...)
	return append(rows, frameEdge("├", "┤", m.width))
}

// endpointLine names one side of the diff, marked the way its lines are marked.
func endpointLine(marker, label, path string) string {
	return fmt.Sprintf("%s %-13s%s", marker, label, path)
}

// diffLineStyle is the style for one line of a unified diff, chosen by its marker.
func diffLineStyle(line string) lipgloss.Style {
	switch {
	case strings.HasPrefix(line, "@@"):
		return hunkStyle
	case strings.HasPrefix(line, "+"):
		return sourceStyle
	case strings.HasPrefix(line, "-"):
		return destinationStyle
	case strings.HasPrefix(line, `\`):
		return lipgloss.NewStyle().Faint(true)
	default:
		return lipgloss.NewStyle()
	}
}
