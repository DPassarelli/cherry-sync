// pickerdiff.go holds the picker's diff view: the note for a file with nothing to
// compare, the fetch behind the "d" key, and the overlay that shows the result.

package view

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// diffFetchedMsg delivers a finished fetch of path to the picker: a notice to show
// in place, or text for the overlay, which failed marks as an error message rather
// than a diff.
type diffFetchedMsg struct {
	path   string
	notice string
	text   string
	failed bool
}

// noDiffNotice returns what to say about a change with only one copy to look at, or
// "" for a change with two.
func noDiffNotice(verb string) string {
	switch verb {
	case "create":
		return "cannot diff: dest does not exist!"
	case "delete":
		return "cannot diff: src does not exist!"
	default:
		return ""
	}
}

// fetchedNotice returns what to say about a fetched file that can't be diffed, or ""
// when it can. A copy too big is named because it may be the one on the far side,
// while the file in front of the user looks fine.
func fetchedNotice(r filediff.Result) string {
	switch {
	case r.Oversized == filediff.SourceCopy:
		return "cannot diff: src file too big!"
	case r.Oversized == filediff.DestinationCopy:
		return "cannot diff: dest file too big!"
	case r.Binary != filediff.NoCopy:
		return "cannot diff: binary files!"
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
	m.fetchID++
	id := m.fetchID
	fetch := m.differ.Fetch
	reveal := tea.Tick(spinnerDelay, func(time.Time) tea.Msg { return fetchRevealMsg{fetch: id} })
	return m, tea.Batch(reveal, func() tea.Msg {
		result, err := fetch(action.Path)
		if err != nil {
			return diffFetchedMsg{path: action.Path, text: "couldn't fetch for diff: " + err.Error(), failed: true}
		}
		return diffFetchedMsg{path: action.Path, notice: fetchedNotice(result), text: result.Text}
	})
}

// openOverlay shows a finished fetch, as a note when there is nothing to diff. The
// alternate screen is what lets the picker come back exactly as it was, preamble
// included, when the overlay closes.
func (m pickerModel) openOverlay(msg diffFetchedMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	m.progress = ""
	m.notice = msg.notice
	if msg.notice != "" {
		return m, nil
	}
	m.overlayOffset = 0
	m.overlayPath = msg.path
	m.overlayIsDiff = !msg.failed
	m.overlay = strings.Split(strings.TrimRight(msg.text, "\n"), "\n")
	return m, tea.EnterAltScreen
}

// overlayKey handles a keypress while the overlay is open. Movement keys scroll
// rather than move the cursor, and every other picker key is ignored, because its effect would be hidden behind the overlay and "enter" would
// start a sync the user can't see they asked for.
func (m pickerModel) overlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = nil
		return m, tea.ExitAltScreen
	case "up", "k":
		m.overlayOffset = max(m.overlayOffset-1, 0)
		return m, nil
	case "down", "j":
		m.overlayOffset = min(m.overlayOffset+1, m.lastOverlayOffset())
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		return m, nil
	}
}
