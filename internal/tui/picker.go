// Package tui is csync's interactive terminal UI: a Bubble Tea file picker shown
// when csync runs attached to a terminal, with the keyboard driving a Selection.
// It is the TTY front-end; the typed-grammar prompt remains the non-TTY fallback.
package tui

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
// user chose to sync and whether they confirmed with Enter (accepted). A cancel —
// Ctrl-C/Esc/q — returns no actions and accepted=false, so the caller can report
// it differently from a confirmed-but-empty selection. It drives a Bubble Tea
// program and so needs a terminal: main selects it only when stdin and stdout are
// TTYs.
func RunPicker(actions []compare.Action) (chosen []compare.Action, accepted bool, err error) {
	final, err := tea.NewProgram(newModel(actions)).Run()
	if err != nil {
		return nil, false, err
	}
	m, ok := final.(pickerModel)
	if !ok || !m.accepted {
		return nil, false, nil
	}
	return m.sel.Selected(), true, nil
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
// out at a glance. The current row is marked and emphasized. Untested by design
// (the visual layer); the keyboard logic it reflects is pinned by the Update
// tests, the grouping by GroupByDir's.
func (m pickerModel) View() string {
	dim := lipgloss.NewStyle().Faint(true)
	dirHeading := lipgloss.NewStyle().Bold(true)

	var b strings.Builder
	b.WriteString("Choose which files to sync:\n")
	fmt.Fprintf(&b, "%s\n", dim.Render("  ↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel"))

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
			marker := "  "
			if flat == m.cursor {
				marker = "> "
			}
			checked := m.sel.IsChecked(flat)
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			base := path.Base(a.Path)
			pad := strings.Repeat(" ", width-lipgloss.Width(base))
			// A checked row is verb-colored; an unchecked row is dimmed so the
			// selected set stands out. Either way the cursor row is bolded.
			style := dim
			if checked {
				style = verbStyle(a.Verb)
			}
			if flat == m.cursor {
				style = style.Bold(true)
			}
			fmt.Fprintf(&b, "%s%s %s\n", marker, box, style.Render(base+pad+"  "+a.Verb))
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
