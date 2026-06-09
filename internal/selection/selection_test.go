package selection

import (
	"reflect"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// sampleActions returns a fixed three-action list used across the Selection tests,
// in the tree order compare would produce.
func sampleActions() []compare.Action {
	return []compare.Action{
		{Verb: "update", Path: "README.md"},
		{Verb: "create", Path: "src/adder.go"},
		{Verb: "update", Path: "src/main.go"},
	}
}

// A new Selection starts with every row checked, so Selected returns all actions
// in their input order — matching the typed prompt's "Enter syncs everything".
func TestSelection_New_ChecksEveryRow(t *testing.T) {
	s := New(sampleActions())

	got := s.Selected()
	want := sampleActions()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// Toggle unchecks a checked row; Selected then omits it while keeping the rest in
// input order.
func TestSelection_Toggle_UnchecksOneRow(t *testing.T) {
	s := New(sampleActions())

	s.Toggle(1)

	got := s.Selected()
	want := []compare.Action{
		{Verb: "update", Path: "README.md"},
		{Verb: "update", Path: "src/main.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// Toggling the same row twice returns it to its prior (checked) state.
func TestSelection_ToggleTwice_RestoresRow(t *testing.T) {
	s := New(sampleActions())

	s.Toggle(1)
	s.Toggle(1)

	got := s.Selected()
	want := sampleActions()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// SetAll(false) unchecks every row, so Selected returns nothing.
func TestSelection_SetAllFalse_SelectsNothing(t *testing.T) {
	s := New(sampleActions())

	s.SetAll(false)

	got := s.Selected()
	if len(got) != 0 {
		t.Errorf("Selected() = %+v, want empty", got)
	}
}

// SetAll(true) rechecks every row, even ones individually unchecked first.
func TestSelection_SetAllTrue_ReselectsEveryRow(t *testing.T) {
	s := New(sampleActions())

	s.Toggle(0)
	s.Toggle(2)
	s.SetAll(true)

	got := s.Selected()
	want := sampleActions()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// Selected returns rows in input order, not the order they were (re)checked.
func TestSelection_Selected_PreservesInputOrder(t *testing.T) {
	s := New(sampleActions())
	s.SetAll(false)

	// Re-check out of order: last row, then first.
	s.Toggle(2)
	s.Toggle(0)

	got := s.Selected()
	want := []compare.Action{
		{Verb: "update", Path: "README.md"},
		{Verb: "update", Path: "src/main.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// An out-of-range Toggle index is a no-op, so a stray cursor position can't panic
// the picker or change the selection.
func TestSelection_Toggle_OutOfRangeIsNoOp(t *testing.T) {
	s := New(sampleActions())

	s.Toggle(-1)
	s.Toggle(3)
	s.Toggle(100)

	got := s.Selected()
	want := sampleActions()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %+v, want %+v", got, want)
	}
}

// IsChecked reports a fresh row as checked, since New checks everything.
func TestSelection_IsChecked_TrueForNewRows(t *testing.T) {
	s := New(sampleActions())

	for i := range sampleActions() {
		if !s.IsChecked(i) {
			t.Errorf("IsChecked(%d) = false, want true", i)
		}
	}
}

// IsChecked reflects a Toggle: the toggled row reads unchecked while its
// neighbours stay checked.
func TestSelection_IsChecked_FalseAfterToggle(t *testing.T) {
	s := New(sampleActions())

	s.Toggle(1)

	if s.IsChecked(1) {
		t.Errorf("IsChecked(1) = true after Toggle, want false")
	}
	if !s.IsChecked(0) || !s.IsChecked(2) {
		t.Errorf("neighbours changed: IsChecked(0)=%v IsChecked(2)=%v, want both true", s.IsChecked(0), s.IsChecked(2))
	}
}

// IsChecked is total: an out-of-range index reads false rather than panicking.
func TestSelection_IsChecked_OutOfRangeIsFalse(t *testing.T) {
	s := New(sampleActions())

	if s.IsChecked(-1) || s.IsChecked(99) {
		t.Errorf("out-of-range IsChecked returned true, want false")
	}
}
