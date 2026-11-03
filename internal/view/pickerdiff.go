// pickerdiff.go holds the picker's diff view: the notice for a file with nothing to
// compare, the fetch behind the "d" key, and the overlay that shows the result.

package view

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// diffFetchedMsg delivers a finished fetch's text to the picker.
type diffFetchedMsg struct {
	text string
}

// noDiffNotice returns what to say about a change with only one copy to look at, or
// "" for a change with two. It says what syncing would do, because a file about to
// be deleted is the case where someone most wants to look before choosing.
func noDiffNotice(verb string) string {
	switch verb {
	case "create":
		return "no diff: new file, will be created on the destination if selected"
	case "delete":
		return "no diff: missing from the source, will be deleted from the destination if selected"
	default:
		return ""
	}
}

// pressDiff handles the "d" key for the cursor's row, returning the fetch to run
// when there are two copies to compare. The fetch runs as a command so a slow remote
// can't freeze the picker, and only one runs at a time because each writes to the
// run log.
func (m pickerModel) pressDiff() (pickerModel, tea.Cmd) {
	action := m.actions[m.cursor]
	notice := noDiffNotice(action.Verb)
	if notice != "" {
		m.notice = notice
		return m, nil
	}
	if m.fetching {
		return m, nil
	}
	m.fetching = true
	fetch := m.fetchDiff
	return m, func() tea.Msg {
		text, err := fetch(action.Path)
		if err != nil {
			text = "couldn't fetch for diff: " + err.Error()
		}
		return diffFetchedMsg{text: text}
	}
}

// openOverlay shows a finished fetch. The alternate screen is what lets the picker
// come back exactly as it was, preamble included, when the overlay closes.
func (m pickerModel) openOverlay(msg diffFetchedMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	m.overlay = strings.Split(strings.TrimRight(msg.text, "\n"), "\n")
	return m, tea.EnterAltScreen
}

// overlayView renders as much of the overlay as fits, leaving one row spare so the
// frame's trailing newline can't scroll its first line away.
func (m pickerModel) overlayView() string {
	lines := m.overlay
	if m.height > 0 && len(lines) > m.height-1 {
		lines = lines[:m.height-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

// wrappedNotice returns the notice wrapped to the terminal's width, since the
// renderer cuts off anything wider. The width is unknown until the first resize.
func (m pickerModel) wrappedNotice() string {
	if m.notice == "" || m.width <= 0 {
		return m.notice
	}
	return lipgloss.NewStyle().Width(m.width).Render(m.notice)
}
