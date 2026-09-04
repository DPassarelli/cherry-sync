package view

import (
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// Behavior: the difference label names what differs between the two copies, and
// says "only" where one attribute differs alone. Content differs on every update
// (the comparison runs with --checksum), so it is worth naming only in the case
// where nothing else does — a file whose size and timestamp both match yet whose
// bytes do not is the one state a user would otherwise read as a bug.
func TestDifferenceLabel(t *testing.T) {
	cases := []struct {
		name string
		diff compare.Difference
		want string
	}{
		{"both", compare.Difference{Content: true, Size: true, ModTime: true}, "size and mtime"},
		{"mtime alone", compare.Difference{Content: true, ModTime: true}, "mtime only"},
		{"size alone", compare.Difference{Content: true, Size: true}, "size only"},
		{"neither", compare.Difference{Content: true}, "contents only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := differenceLabel(c.diff)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Behavior: an update's detail is the label saying what differs about it.
func TestActionDetail_Update_CarriesTheDifferenceLabel(t *testing.T) {
	a := compare.Action{
		Verb: "update", Path: "main.go",
		Diff: compare.Difference{Content: true, Size: true, ModTime: true},
	}

	got := actionDetail(a)

	want := "size and mtime"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Behavior: a create carries no detail — there is no counterpart on the other side
// for it to differ from, so naming a difference would be a claim about a file that
// does not exist.
func TestActionDetail_Create_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "create", Path: "new.go"}

	got := actionDetail(a)

	if got != "" {
		t.Errorf("got %q, want the empty string for a create", got)
	}
}

// Behavior: a delete carries no detail either. The file is being removed rather
// than compared, so there is nothing about it that differs.
func TestActionDetail_Delete_CarriesNoDetail(t *testing.T) {
	a := compare.Action{Verb: "delete", Path: "stale.txt"}

	got := actionDetail(a)

	if got != "" {
		t.Errorf("got %q, want the empty string for a delete", got)
	}
}
