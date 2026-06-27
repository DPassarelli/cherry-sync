package view

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// sampleActions returns a fixed three-action list for the picker tests.
func sampleActions() []compare.Action {
	return []compare.Action{
		{Verb: "update", Path: "README.md"},
		{Verb: "create", Path: "src/adder.go"},
		{Verb: "update", Path: "src/main.go"},
	}
}

// applyKey sends one keypress to the model and returns the updated model.
func applyKey(m pickerModel, key tea.KeyMsg) pickerModel {
	updated, _ := m.Update(key)
	return updated.(pickerModel)
}

// runeKey builds a keypress for a single printable rune (a letter key).
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// isQuit reports whether cmd is the tea.Quit command — invoking it yields a
// tea.QuitMsg. A nil cmd is not a quit.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// Down advances the cursor one row.
func TestUpdate_Down_AdvancesCursor(t *testing.T) {
	m := newModel(sampleActions())

	m = applyKey(m, tea.KeyMsg{Type: tea.KeyDown})

	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
}

// Down stops at the last row rather than running past the end.
func TestUpdate_Down_StopsAtLastRow(t *testing.T) {
	m := newModel(sampleActions())
	m.cursor = 2 // last of three

	m = applyKey(m, tea.KeyMsg{Type: tea.KeyDown})

	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.cursor)
	}
}

// Up retreats the cursor one row.
func TestUpdate_Up_RetreatsCursor(t *testing.T) {
	m := newModel(sampleActions())
	m.cursor = 2

	m = applyKey(m, tea.KeyMsg{Type: tea.KeyUp})

	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
}

// Up stops at the first row rather than going negative.
func TestUpdate_Up_StopsAtFirstRow(t *testing.T) {
	m := newModel(sampleActions())

	m = applyKey(m, tea.KeyMsg{Type: tea.KeyUp})

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", m.cursor)
	}
}

// j and k mirror the down and up arrows.
func TestUpdate_JK_MirrorArrowKeys(t *testing.T) {
	m := newModel(sampleActions())

	m = applyKey(m, runeKey('j'))
	if m.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", m.cursor)
	}
	m = applyKey(m, runeKey('k'))
	if m.cursor != 0 {
		t.Errorf("after k: cursor = %d, want 0", m.cursor)
	}
}

// Space toggles the row under the cursor — here the second row — leaving the
// others alone.
func TestUpdate_Space_TogglesRowAtCursor(t *testing.T) {
	m := newModel(sampleActions())
	m.cursor = 1

	m = applyKey(m, tea.KeyMsg{Type: tea.KeySpace})

	if m.sel.IsChecked(1) {
		t.Errorf("row 1 still checked after space at cursor 1")
	}
	if !m.sel.IsChecked(0) || !m.sel.IsChecked(2) {
		t.Errorf("space at cursor 1 changed another row")
	}
}

// Space acts on whichever row the cursor is on — triangulates that it isn't fixed
// to one index.
func TestUpdate_Space_TogglesDifferentRowAtDifferentCursor(t *testing.T) {
	m := newModel(sampleActions())
	m.cursor = 0

	m = applyKey(m, tea.KeyMsg{Type: tea.KeySpace})

	if m.sel.IsChecked(0) {
		t.Errorf("row 0 still checked after space at cursor 0")
	}
}

// 'a' clears the selection when every row is currently checked (the New default).
func TestUpdate_A_DeselectsAllWhenAllChecked(t *testing.T) {
	m := newModel(sampleActions())

	m = applyKey(m, runeKey('a'))

	n := len(m.sel.Selected())
	if n != 0 {
		t.Errorf("Selected = %d rows, want 0", n)
	}
}

// 'a' checks every row when at least one is currently unchecked.
func TestUpdate_A_SelectsAllWhenNotAllChecked(t *testing.T) {
	m := newModel(sampleActions())
	m.sel.Toggle(1) // one off, so not all checked

	m = applyKey(m, runeKey('a'))

	n := len(m.sel.Selected())
	if n != len(sampleActions()) {
		t.Errorf("Selected = %d rows, want %d", n, len(sampleActions()))
	}
}

// Enter accepts the current selection and quits.
func TestUpdate_Enter_AcceptsAndQuits(t *testing.T) {
	m := newModel(sampleActions())

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !updated.(pickerModel).accepted {
		t.Errorf("accepted = false after Enter, want true")
	}
	if !isQuit(cmd) {
		t.Errorf("Enter did not return the quit command")
	}
}

// Ctrl-C cancels — it quits without marking the selection accepted.
func TestUpdate_CtrlC_CancelsAndQuits(t *testing.T) {
	m := newModel(sampleActions())

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if updated.(pickerModel).accepted {
		t.Errorf("accepted = true after Ctrl-C, want false")
	}
	if !isQuit(cmd) {
		t.Errorf("Ctrl-C did not return the quit command")
	}
}

// Esc and q also cancel, mirroring Ctrl-C.
func TestUpdate_EscAndQ_AlsoCancel(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, runeKey('q')} {
		m := newModel(sampleActions())

		updated, cmd := m.Update(key)

		if updated.(pickerModel).accepted {
			t.Errorf("%s: accepted = true, want false", key.String())
		}
		if !isQuit(cmd) {
			t.Errorf("%s: did not return the quit command", key.String())
		}
	}
}
