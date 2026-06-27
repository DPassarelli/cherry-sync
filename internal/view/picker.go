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
// offer, their checkbox Selection, the cursor's current row, and whether the user
// accepted (Enter) rather than cancelled (Ctrl-C/Esc/q).
type pickerModel struct {
	actions  []compare.Action
	sel      *selection.Selection
	cursor   int
	accepted bool
}

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
// so a cancel must report nothing rather than leak the default set. It drives a
// Bubble Tea program and so needs a terminal: main selects it only when stdin and
// stdout are TTYs.
func RunPicker(actions []compare.Action) ([]compare.Action, error) {
	final, err := tea.NewProgram(newModel(actions)).Run()
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

// Update handles one keypress: arrow/jk move the cursor (clamped at the ends),
// space toggles the row under it, 'a' toggles the whole list (all↔none), Enter
// accepts the selection and quits, and Ctrl-C/Esc/q cancel — quitting without
// accepting. Non-key messages pass through untouched.
func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
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
	return m, nil
}

// View renders the picker: a directions header, then the changes grouped by
// directory — each group under its "./dir" heading, each row a cursor marker, a
// "[x]"/"[ ]" checkbox, the file's basename, and its verb. A checked row is
// colored by change type; an unchecked row is dimmed so the selected set stands
// out at a glance. The current row is marked with a heavy caret and a subtle
// background bar. Untested by design (the visual layer); the keyboard logic it
// reflects is pinned by the Update tests, the grouping by GroupByDir's.
func (m pickerModel) View() string {
	dim := lipgloss.NewStyle().Faint(true)
	dirHeading := lipgloss.NewStyle().Bold(true)
	prompt := lipgloss.NewStyle().Bold(true)
	// The cursor row is marked with a heavy bold caret and a subtle background bar
	// rather than bold text; AdaptiveColor keeps the bar legible on both light and
	// dark terminals.
	cursorBG := lipgloss.AdaptiveColor{Light: "252", Dark: "236"}

	var b strings.Builder
	// A leading blank line separates the picker from the Source/Destination header
	// main prints above it; the bold "? " prompt and dimmed directions mirror the
	// mockup (and npm-check-updates' "?"-led question).
	fmt.Fprintf(&b, "\n? %s\n", prompt.Render("Choose which files to sync:"))
	fmt.Fprintf(&b, "   %s\n", dim.Render("↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel"))

	// Align the verb column by padding every basename to the widest one.
	// lipgloss.Width measures display cells, so multi-byte names line up too.
	width := 0
	for _, a := range m.actions {
		w := lipgloss.Width(path.Base(a.Path))
		if w > width {
			width = w
		}
	}

	// A single flat index walks the actions in display order. GroupByDir preserves
	// that order, so this index matches the cursor and the Selection one-for-one as
	// the rows are emitted group by group.
	flat := 0
	for _, g := range selection.GroupByDir(m.actions) {
		fmt.Fprintf(&b, "\n%s\n", dirHeading.Render(g.Dir))
		for _, a := range g.Actions {
			cursorRow := flat == m.cursor
			checked := m.sel.IsChecked(flat)
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			base := path.Base(a.Path)
			pad := strings.Repeat(" ", width-lipgloss.Width(base))
			// A checked row is verb-colored; an unchecked row is dimmed so the
			// selected set stands out.
			textStyle := dim
			if checked {
				textStyle = verbStyle(a.Verb)
			}
			caretStyle := lipgloss.NewStyle().Bold(true)
			boxStyle := lipgloss.NewStyle()
			// The cursor row carries the background across all three segments, so the
			// highlight reads as one continuous bar rather than a single colored word.
			if cursorRow {
				textStyle = textStyle.Background(cursorBG)
				caretStyle = caretStyle.Background(cursorBG)
				boxStyle = boxStyle.Background(cursorBG)
			}
			marker := "  "
			if cursorRow {
				marker = caretStyle.Render("❯ ")
			}
			fmt.Fprintf(&b, "%s%s%s\n", marker, boxStyle.Render(box+" "), textStyle.Render(base+pad+"  "+a.Verb))
			flat++
		}
	}
	return b.String()
}

// verbStyle returns the lipgloss style that colors a change by its verb: green for
// create, yellow for update, red for delete. delete is reserved — csync emits no
// delete actions yet — and an unknown verb renders unstyled.
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
