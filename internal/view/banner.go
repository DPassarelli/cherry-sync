// banner.go renders csync's title banner, shown at the top of an interactive run.

package view

import "github.com/charmbracelet/lipgloss"

// Banner returns csync's title banner — the bold tool name followed by a blank
// line, ready to print above the header. It carries no version yet: version
// injection is deferred until a --version flag exists, so today the banner is just
// the name. main shows it only on an interactive run, where decoration belongs;
// piped output skips it.
func Banner() string {
	bold := lipgloss.NewStyle().Bold(true)
	return bold.Render("cherry-sync") + "\n\n"
}
