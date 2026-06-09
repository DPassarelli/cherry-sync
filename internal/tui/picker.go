// Package tui is csync's interactive terminal UI: a Bubble Tea file picker shown
// when csync runs attached to a terminal, with the keyboard driving a Selection.
// It is the TTY front-end; the typed-grammar prompt remains the non-TTY fallback.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// newModel builds a pickerModel over actions, every row checked to start.
func newModel(actions []compare.Action) pickerModel {
	return pickerModel{
		actions: actions,
		sel:     selection.New(actions),
	}
}

// RunPicker shows the interactive picker over actions and returns the actions the
// user chose to sync. Cancelling (Ctrl-C/Esc/q) — or confirming with nothing
// checked — returns no actions; both mean "sync nothing". It drives a Bubble Tea
// program and so needs a terminal: main selects it only when stdin and stdout are
// TTYs.
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

// View renders the picker: a directions header, then one row per action with a
// cursor marker, a checkbox, the verb, and the path. Plain text for now — color
// and directory grouping arrive in a later slice. Untested by design (the visual
// layer); the keyboard logic it reflects is pinned by the Update tests.
func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString("Choose which files to sync:\n")
	b.WriteString("  ↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel\n\n")
	for i, a := range m.actions {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.sel.IsChecked(i) {
			box = "[x]"
		}
		fmt.Fprintf(&b, "%s%s %s  %s\n", cursor, box, a.Verb, a.Path)
	}
	return b.String()
}
