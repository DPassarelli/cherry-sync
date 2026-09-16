// Package view owns csync's user-facing rendering: the banner, the source/
// destination header, the change list, the interactive Bubble Tea picker, and the
// post-sync summary. Styling is applied with lipgloss, which drops ANSI when
// stdout is not a terminal, so the same calls render styled in a terminal and
// plain when piped — the visual graceful-fallback lives here, in one place. The
// picker is the TTY front-end; the typed-grammar prompt (driven by main and
// selection) remains the non-TTY input path.
package view

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/selection"
)

// pickerModel is the Bubble Tea model for the interactive picker: the actions on
// offer, their checkbox Selection, the cursor's current row, the terminal width and
// height (learned from WindowSizeMsg, 0 until the first one) that bound the scroll
// window, the preamble main printed above the picker (banner, header, exclusions)
// whose on-screen rows — width-dependent, since a long line wraps — must be held out
// of the scroll region so it stays visible, the offset of the scroll window's first
// line (carried between frames so the window holds its position until the cursor
// reaches an edge), and whether the user accepted (Enter) rather than cancelled
// (Ctrl-C/Esc/q).
type pickerModel struct {
	actions  []compare.Action
	sel      *selection.Selection
	cursor   int
	width    int
	height   int
	preamble string
	offset   int
	accepted bool
	// now is the moment each row's ages are measured against, fixed when the picker
	// is built rather than read per frame: the list does not re-render on a timer,
	// and an age that crept forward between two repaints of the same unchanged list
	// would read as a change that never happened.
	now time.Time
}

// pickerHeaderLines is how many rows View prints above the change list — a leading
// blank, the "? Choose…" prompt, and the key hint — and so must be subtracted from
// the terminal height to get the rows available for the scrolling list region.
const pickerHeaderLines = 3

// scrollMargin is one row left unused at the bottom of the picker's frame. View
// ends its output with a trailing newline; without a spare row that newline scrolls
// the terminal up by one, pushing the top line (part of the preamble) off screen —
// the very failure this margin prevents.
const scrollMargin = 1

// newModel builds a pickerModel over actions, every row checked to start. The
// actions are reordered so each directory's files are contiguous, directories in
// first-appearance order: compare's report order interleaves a directory's files
// with subdirectories, so the picker regroups for display. This grouped order is
// what the cursor moves through and what the Selection indices line up with, so
// View can render group by group without the cursor jumping around the screen.
func newModel(actions []compare.Action) pickerModel {
	var ordered []compare.Action
	for _, g := range selection.GroupByDir(actions) {
		ordered = append(ordered, g.Actions...)
	}
	return pickerModel{
		actions: ordered,
		sel:     selection.New(ordered),
		now:     time.Now(),
	}
}

// RunPicker shows the interactive picker over actions and returns the actions the
// user chose to sync — empty when there is nothing to do, whether the user
// cancelled (Ctrl-C/Esc/q) or confirmed with nothing checked. The accepted check
// is what separates the two from a real selection: the picker starts all-checked,
// so a cancel must report nothing rather than leak the default set. preamble is the
// text main has already printed above the picker (banner, header, exclusions); the
// picker measures its on-screen rows at the live terminal width and holds them out
// of its scroll region so the preamble stays on screen. It drives a Bubble Tea
// program and so needs a terminal: main selects it only when stdin and stdout are
// TTYs.
func RunPicker(actions []compare.Action, preamble string) ([]compare.Action, error) {
	m := newModel(actions)
	m.preamble = preamble
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	m, ok := final.(pickerModel)
	if !ok || !m.accepted {
		return nil, nil
	}
	return m.sel.Selected(), nil
}

// Init is part of tea.Model; the picker needs no startup command.
func (m pickerModel) Init() tea.Cmd {
	return nil
}

// Update handles one message: a WindowSizeMsg records the terminal width and height
// that bound the scroll window; a keypress moves or acts. Keys: arrow/jk move the
// cursor (clamped at the ends), space toggles the row under it, 'a' toggles the
// whole list (all↔none), Enter accepts the selection and quits, and Ctrl-C/Esc/q
// cancel — quitting without accepting. After a move or resize the scroll offset is
// re-settled so the window follows the cursor to its edges. Other messages pass
// through untouched.
func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.offset = m.scrollOffset()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.actions)-1 {
				m.cursor++
			}
		case " ":
			m.sel.Toggle(m.cursor)
		case "a":
			// Toggle the whole list: clear it if every row is checked, otherwise check
			// every row.
			m.sel.SetAll(!m.sel.AllChecked())
		case "enter":
			m.accepted = true
			return m, tea.Quit
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}
		m.offset = m.scrollOffset()
	}
	return m, nil
}

// scrollOffset settles the scroll window's first line for the current cursor and
// terminal height, moving the stored offset the minimum needed to keep the cursor's
// row on screen. It rebuilds the line layout (cheap for a change list) so the
// offset it stores and the window View later slices agree on the same lines. On the
// first row it snaps to the very top so the leading group heading — which sits above
// the first row and is not a cursor-reachable line, so edge-triggered scrolling
// alone would strand it — comes into view; the bottom end is already fully revealed
// by scroll's clamp.
func (m pickerModel) scrollOffset() int {
	if m.cursor == 0 {
		return 0
	}
	lines, cursorLine := m.contentLines()
	return scroll(lines, cursorLine, m.offset, m.scrollHeight()).Offset
}

// scrollHeight is the number of terminal rows the picker's list region may occupy:
// the terminal height less the preamble's on-screen rows (measured at the current
// width, so wrapped lines count for all the rows they take), the fixed header rows,
// and the one-row bottom margin. Keeping the list within it is what stops the
// preamble from scrolling off the top.
func (m pickerModel) scrollHeight() int {
	return m.height - countRows(m.preamble, m.width) - pickerHeaderLines - scrollMargin
}

// visible is the scroll window over the current change list for the settled offset:
// the rows to draw plus how many are hidden above and below. View renders it; a test
// pins how reserved rows shrink it.
func (m pickerModel) visible() viewport {
	lines, cursorLine := m.contentLines()
	return scroll(lines, cursorLine, m.offset, m.scrollHeight())
}
