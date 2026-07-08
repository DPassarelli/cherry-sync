// Package view owns csync's user-facing rendering: the banner, the source/
// destination header, the change list, the interactive Bubble Tea picker, and the
// post-sync summary. Styling is applied with lipgloss, which drops ANSI when
// stdout is not a terminal, so the same calls render styled in a terminal and
// plain when piped — the visual graceful-fallback lives here, in one place. The
// picker is the TTY front-end; the typed-grammar prompt (driven by main and
// selection) remains the non-TTY input path.
package view

import (
	"fmt"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// cursorBG is the background bar drawn across the cursor's row. AdaptiveColor keeps
// it legible on both light and dark terminals; it pairs with the caret, which
// carries the cursor cue on terminals where the bar is hard to see.
var cursorBG = lipgloss.AdaptiveColor{Light: "252", Dark: "236"}

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

// View renders the picker: a directions header that stays fixed, then the changes
// grouped by directory, scrolled so the cursor's row is always visible. The window
// is always bracketed by an indicator line top and bottom — an arrow where content
// is hidden on that side, a blank where it isn't. Rendering both lines
// unconditionally keeps a fixed blank line under the header and stops the list from
// jumping when an arrow appears or disappears. The list itself is built by
// contentLines; the scroll math and the cursor-line mapping are pinned by tests, so
// only the styling is untested.
func (m pickerModel) View() string {
	prompt := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	// A leading blank line separates the picker from the Source/Destination header
	// main prints above it; the bold "? " prompt and dimmed directions mirror the
	// mockup (and npm-check-updates' "?"-led question). This block is pickerHeaderLines
	// tall and is held out of the scroll region so it never scrolls away.
	out := []string{
		"",
		"? " + prompt.Render(pickerPrompt(len(m.actions))),
		"   " + dim.Render("↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel"),
	}

	vp := m.visible()
	// The "above" indicator line doubles as the fixed blank under the header when
	// nothing is hidden above, so the prompt-to-list gap never changes height.
	out = append(out, scrollIndicator(dim, "▲ more above", vp.HiddenAbove))
	out = append(out, vp.Lines...)
	out = append(out, scrollIndicator(dim, "▼ more below", vp.HiddenBelow))
	return strings.Join(out, "\n") + "\n"
}

// contentLines renders the change list — the per-directory headings and their rows —
// into one string per line, and returns the index of the cursor's row within them.
// It is the unwindowed list: View slices it through scroll. A blank line separates
// each group from the one before, but the first heading has none — the fixed "above"
// indicator line View draws is the gap under the header, and a leading blank here
// would scroll away and shift the list. A single flat index walks the actions in
// display order; GroupByDir preserves that order, so the index matches the cursor
// and the Selection one-for-one. Each row is a cursor marker, a "[x]"/"[ ]"
// checkbox, the basename, and the verb; a checked row is verb-colored, an unchecked
// row dimmed, and the cursor's row carries the caret and a background bar across all
// segments.
func (m pickerModel) contentLines() ([]string, int) {
	dim := lipgloss.NewStyle().Faint(true)
	dirHeading := lipgloss.NewStyle().Bold(true)

	// Align the verb column by padding every basename to the widest one.
	// lipgloss.Width measures display cells, so multi-byte names line up too.
	width := 0
	for _, a := range m.actions {
		w := lipgloss.Width(path.Base(a.Path))
		if w > width {
			width = w
		}
	}

	var lines []string
	cursorLine := 0
	flat := 0
	for _, g := range selection.GroupByDir(m.actions) {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, dirHeading.Render(g.Dir))
		for _, a := range g.Actions {
			cursorRow := flat == m.cursor
			checked := m.sel.IsChecked(flat)
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			base := path.Base(a.Path)
			pad := strings.Repeat(" ", width-lipgloss.Width(base))
			textStyle := dim
			if checked {
				textStyle = verbStyle(a.Verb)
			}
			caretStyle := lipgloss.NewStyle().Bold(true)
			boxStyle := lipgloss.NewStyle()
			if cursorRow {
				textStyle = textStyle.Background(cursorBG)
				caretStyle = caretStyle.Background(cursorBG)
				boxStyle = boxStyle.Background(cursorBG)
			}
			marker := "  "
			if cursorRow {
				marker = caretStyle.Render("❯ ")
			}
			lines = append(lines, fmt.Sprintf("%s%s%s", marker, boxStyle.Render(box+" "), textStyle.Render(base+pad+"  "+a.Verb)))
			if cursorRow {
				cursorLine = len(lines) - 1
			}
			flat++
		}
	}
	return lines, cursorLine
}

// pickerPrompt returns the picker's question line for a list of count files. It
// names the count so a list that renders and then waits for input reads as "these
// are all of them," not a freeze or a bug (issue #64). The count is the plain total
// on offer, not the currently-checked subset, and the phrasing is not pluralized —
// a lone file reads "(1 available)". Styling is applied by the caller; this returns
// plain text so it can be pinned by a test without a terminal.
func pickerPrompt(count int) string {
	return fmt.Sprintf("Choose which files to sync (%d available):", count)
}

// scrollIndicator renders one bracket line of the scroll window: the dimmed label
// (e.g. "▲ more above") when hidden rows exist on that side, or an empty line when
// none do. The blank holds the row so the frame's height doesn't change as the
// cursor crosses the list's ends.
func scrollIndicator(style lipgloss.Style, label string, hidden int) string {
	if hidden == 0 {
		return ""
	}
	return "  " + style.Render(label)
}

// verbStyle returns the lipgloss style that colors a change by its verb: green for
// create, yellow for update, red for delete. An unknown verb renders unstyled.
func verbStyle(verb string) lipgloss.Style {
	switch verb {
	case "create":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case "update":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case "delete":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	default:
		return lipgloss.NewStyle()
	}
}
