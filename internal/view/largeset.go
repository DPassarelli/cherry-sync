// largeset.go renders the gate that stands in for the picker when a comparison
// turns up more changes than csync is built to review. It occupies the row the
// picker's prompt would have filled, states the limit, and offers the only two
// answers there are: take the whole set, or take none of it.

package view

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LargeSetLimit is the number of changes past which csync stops offering a choice
// and offers an escape hatch instead (#61). Reviewing a list this long a row at a
// time is not something anyone does, and rendering it makes the picker sluggish to
// scroll, so beyond this point the list is not worth drawing at all. The number is
// a judgement about what a person will actually read, not a performance ceiling —
// it is deliberately well below where the picker starts to struggle.
const LargeSetLimit = 200

// largeSetHint names the only two keys the gate answers to, in the picker's own
// hint format so the two read as parts of the same program.
const largeSetHint = "enter to sync entire set regardless · ctrl-c to cancel"

// largeSetWarning returns the gate's message. It states the limit rather than the
// run's actual count because what the user needs to act on is csync's stated design
// boundary; the exact number of changes beyond it makes no difference to the choice.
func largeSetWarning() string {
	return fmt.Sprintf("There are more than %d files that need to be synced. This program is not designed for working with large file sets.", LargeSetLimit)
}

// largeSetModel is the gate itself: a frame that waits for one of two keys and
// records which arrived, plus the terminal width it wraps its warning to (0 until
// the first WindowSizeMsg).
type largeSetModel struct {
	proceed bool
	width   int
}

// Init starts nothing. The gate has no animation and no work to launch — it exists
// only to wait for a keypress.
func (m largeSetModel) Init() tea.Cmd { return nil }

// Update ends the gate on either of the two keys it advertises and ignores every
// other. Nothing else may end it: syncing a set this large is the decision the
// warning exists to slow down, so a stray keypress must not be able to make it.
func (m largeSetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	size, resized := msg.(tea.WindowSizeMsg)
	if resized {
		m.width = size.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter":
		m.proceed = true
		return m, tea.Quit
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// largeSetLines lays the warning out for a terminal of the given width, returning
// each row already prefixed: the "! " marker on the first and a two-space hanging
// indent on any row the sentence wraps onto, so the text stays in one column under
// the marker. Wrapping is this gate's own problem to solve rather than the
// terminal's — Bubble Tea truncates each line of a frame to the terminal width
// instead of letting it wrap, so an unwrapped warning simply loses its tail on an
// 80-column screen. A non-positive width is the terminal size before the first
// WindowSizeMsg; there is nothing to wrap to yet, so the sentence is returned whole.
// Returned unstyled so a test can measure it without a terminal.
func largeSetLines(width int) []string {
	text := largeSetWarning()
	if width > len(warnMarker) {
		text = lipgloss.NewStyle().Width(width - len(warnMarker)).Render(text)
	}

	var out []string
	for i, line := range strings.Split(text, "\n") {
		prefix := warnIndent
		if i == 0 {
			prefix = warnMarker
		}
		out = append(out, prefix+strings.TrimRight(line, " "))
	}
	return out
}

// warnMarker leads the warning, taking the place of the picker's "? " prompt because
// this states something rather than asking it. warnIndent is the same width, and
// holds wrapped rows in the marker's text column. hintIndent matches the picker's
// own key-hint indent, so the two programs' directions sit in the same place.
const (
	warnMarker = "! "
	warnIndent = "  "
	hintIndent = "   "
)

// View draws the warning in the slot the picker's prompt would have occupied,
// mirroring its shape: a leading blank line, the marked and bolded statement, then
// dimmed directions. The frame is returned unchanged when the gate ends, which
// leaves it on screen — nothing draws over it, and an answered warning belongs in
// the scrollback above whatever happens next.
func (m largeSetModel) View() string {
	warn := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	out := []string{""}
	for _, line := range largeSetLines(m.width) {
		// Bold only the text: the marker and the indent that aligns under it are
		// structure, and carry no emphasis of their own.
		prefix, text := line[:len(warnMarker)], line[len(warnMarker):]
		out = append(out, prefix+warn.Render(text))
	}
	out = append(out, hintIndent+dim.Render(largeSetHint), "")

	return strings.Join(out, "\n")
}

// RunLargeSetGate shows the large-set warning and reports whether the user chose to
// sync the whole set anyway. It drives a Bubble Tea program and so needs a terminal;
// main calls it only on an interactive run, where it stands in for the picker.
func RunLargeSetGate() (bool, error) {
	final, err := tea.NewProgram(largeSetModel{}).Run()
	if err != nil {
		return false, err
	}
	done, ok := final.(largeSetModel)
	if !ok {
		return false, nil
	}
	return done.proceed, nil
}
