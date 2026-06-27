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

// Header returns the two-line source/destination header. The labels are padded to
// a common width so the values align, and each value is bold — lipgloss drops the
// emphasis when stdout is not a terminal, so piped output stays plain.
func Header(source, destination string) string {
	bold := lipgloss.NewStyle().Bold(true)
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s%s\n", headerLabelWidth, "Source:", bold.Render(source))
	fmt.Fprintf(&b, "%-*s%s\n", headerLabelWidth, "Destination:", bold.Render(destination))
	return b.String()
}
