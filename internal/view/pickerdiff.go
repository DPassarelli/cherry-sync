// pickerdiff.go holds the picker's diff view: the notice for a file with nothing to
// compare, the fetch behind the "d" key, and the overlay that shows the result.

package view

import (
	"fmt"
	"slices"
	"strings"
	"time"

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
	m.fetchID++
	id := m.fetchID
	fetch := m.fetchDiff
	reveal := tea.Tick(spinnerDelay, func(time.Time) tea.Msg { return fetchRevealMsg{fetch: id} })
	return m, tea.Batch(reveal, func() tea.Msg {
		result, err := fetch(action.Path)
		if err != nil {
			return diffFetchedMsg{text: "couldn't fetch for diff: " + err.Error()}
		}
		return diffFetchedMsg{notice: fetchedNotice(result), text: result.Text}
	})
}

// openOverlay shows a finished fetch, as a notice when there is nothing to diff. The
// alternate screen is what lets the picker
// come back exactly as it was, preamble included, when the overlay closes.
func (m pickerModel) openOverlay(msg diffFetchedMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	m.notice = ""
	if msg.notice != "" {
		m.notice = msg.notice
		m.offset = m.scrollOffset()
		return m, nil
	}
	m.overlayOffset = 0
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

// overlayFooterRows is the rule and the key hint drawn under the diff.
const overlayFooterRows = 2

// overlayRows is how many diff lines fit above the footer, leaving one row spare so
// the frame's trailing newline can't scroll its first line away.
func (m pickerModel) overlayRows() int {
	if m.height <= 0 {
		return len(m.overlay)
	}
	return max(m.height-1-overlayFooterRows, 1)
}

// lastOverlayOffset is the furthest the overlay can scroll while staying full.
func (m pickerModel) lastOverlayOffset() int {
	return max(len(m.overlay)-m.overlayRows(), 0)
}

// overlayView renders the part of the diff in view, then the footer. The offset is
// limited again here because a resize can move the end of the diff after it was
// stored. A short diff is padded so the footer stays on the bottom row, and the rule
// sets the hint apart, because an indented hint would otherwise read as a context
// line of the diff.
func (m pickerModel) overlayView() string {
	dim := lipgloss.NewStyle().Faint(true)
	first := min(m.overlayOffset, m.lastOverlayOffset())
	last := min(first+m.overlayRows(), len(m.overlay))
	lines := slices.Clone(m.overlay[first:last])
	if m.height > 0 {
		for len(lines) < m.overlayRows() {
			lines = append(lines, "")
		}
	}
	lines = append(lines, dim.Render(strings.Repeat("─", max(m.width, 1))), "   "+dim.Render("↑/↓ scroll · esc back"))
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
