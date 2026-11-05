// pickerdiff.go holds the picker's diff view: the notice for a file with nothing to
// compare, the fetch behind the "d" key, and the overlay that shows the result.

package view

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// diffFetchedMsg delivers a finished fetch to the picker: a notice to show in place,
// or text for the overlay.
type diffFetchedMsg struct {
	notice string
	text   string
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

// fetchedNotice returns what to say about a fetched file that can't be diffed, or ""
// when it can. Naming the copy matters because the one that tripped the check may be
// on the far side, while the file in front of the user looks fine.
func fetchedNotice(r filediff.Result) string {
	if r.Oversized != filediff.NoCopies {
		return fmt.Sprintf("no diff: %s over the %d KiB limit", copiesLabel(r.Oversized), filediff.SizeLimit/1024)
	}
	if r.Binary != filediff.NoCopies {
		return fmt.Sprintf("no diff: %s binary, and binary files are not diffed", copiesLabel(r.Binary))
	}
	return ""
}

// copiesLabel names copies as the start of a sentence about them.
func copiesLabel(c filediff.Copies) string {
	switch c {
	case filediff.SourceCopy:
		return "source copy is"
	case filediff.DestinationCopy:
		return "destination copy is"
	default:
		return "both copies are"
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
		result, err := fetch(action.Path)
		if err != nil {
			return diffFetchedMsg{text: "couldn't fetch for diff: " + err.Error()}
		}
		return diffFetchedMsg{notice: fetchedNotice(result), text: result.Text}
	}
}

// openOverlay shows a finished fetch, as a notice when there is nothing to diff. The
// alternate screen is what lets the picker
// come back exactly as it was, preamble included, when the overlay closes.
func (m pickerModel) openOverlay(msg diffFetchedMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	if msg.notice != "" {
		m.notice = msg.notice
		m.offset = m.scrollOffset()
		return m, nil
	}
	m.overlay = strings.Split(strings.TrimRight(msg.text, "\n"), "\n")
	return m, tea.EnterAltScreen
}

// overlayKey handles a keypress while the overlay is open. Every picker key is
// ignored, because its effect would be hidden behind the overlay and "enter" would
// start a sync the user can't see they asked for.
func (m pickerModel) overlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = nil
		return m, tea.ExitAltScreen
	case "ctrl+c":
		return m, tea.Quit
	default:
		return m, nil
	}
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
