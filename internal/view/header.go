// header.go renders the source/destination header: two label-aligned lines whose
// values are emphasized in a terminal and plain when piped.

package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// headerLabelWidth is the column the header values start in: the widest label
// ("Destination:") plus a gap, so "Source:" and "Destination:" line their values
// up underneath each other.
const headerLabelWidth = 15

// Endpoint is one side of the header: the operand to display, plus — when csync
// rewrote it — the original path portion to note inline, so the rewrite is
// disclosed beside the value rather than on a separate line.
type Endpoint struct {
	// Path is the operand as csync will use it (already normalized), shown bold.
	Path string
	// From is the original path portion when the operand was rewritten (e.g.
	// "~/working"); "" when it was not, in which case no note is shown.
	From string
}

// Header returns the two-line source/destination header. The labels are padded to
// a common width so the values align, and each value is bold — lipgloss drops the
// emphasis when stdout is not a terminal, so piped output stays plain. When an
// endpoint was rewritten, a faint "(rewritten from …)" note follows the value on
// the same line.
func Header(source, destination Endpoint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s%s\n", headerLabelWidth, "Source:", renderEndpoint(source))
	fmt.Fprintf(&b, "%-*s%s\n", headerLabelWidth, "Destination:", renderEndpoint(destination))
	return b.String()
}

// renderEndpoint formats one endpoint's value: the operand emphasized, followed
// by a faint "(rewritten from <original>)" note when csync rewrote it. Both
// styles drop to plain text when stdout is not a terminal.
func renderEndpoint(e Endpoint) string {
	bold := lipgloss.NewStyle().Bold(true)
	value := bold.Render(e.Path)
	if e.From != "" {
		dim := lipgloss.NewStyle().Faint(true)
		value += " " + dim.Render("(rewritten from "+e.From+")")
	}
	return value
}
