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
// current width: the title, each side's full path, and a rule setting them apart
// from the diff.
func (m pickerModel) overlayHeaderRows() []string {
	lines := []string{
		lipgloss.NewStyle().Bold(true).Render("Diff of " + m.overlayPath),
		destinationStyle.Render(endpointLine("-", "Destination:", endpointPath(m.differ.Destination, m.overlayPath))),
		sourceStyle.Render(endpointLine("+", "Source:", endpointPath(m.differ.Source, m.overlayPath))),
	}
	var rows []string
	for _, line := range lines {
		rows = append(rows, wrapRows(line, m.width)...)
	}
	rule := lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", max(m.width, 1)))
	return append(rows, rule)
}

// endpointLine names one side of the diff, marked the way its lines are marked.
func endpointLine(marker, label, path string) string {
	return fmt.Sprintf("%s %-13s%s", marker, label, path)
}

// endpointPath is where relPath lives under operand, written the way csync names
// the operand elsewhere. A root operand already ends in the separator.
func endpointPath(operand, relPath string) string {
	return strings.TrimSuffix(operand, "/") + "/" + relPath
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
