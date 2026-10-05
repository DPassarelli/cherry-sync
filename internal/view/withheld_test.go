package view

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// creates returns n create actions named dir/f01, dir/f02, … for building a group.
func creates(dir string, n int) []compare.Action {
	acts := make([]compare.Action, n)
	for i := range acts {
		acts[i] = compare.Action{Verb: "create", Path: fmt.Sprintf("%s/f%02d", dir, i+1)}
	}
	return acts
}

// Behavior: files at the transfer root are never summarized, however many there
// are, since they share no folder a summary row could name.
func TestWithheldRows_RootFilesNeverGroup(t *testing.T) {
	var acts []compare.Action
	for i := range 12 {
		acts = append(acts, compare.Action{Verb: "create", Path: fmt.Sprintf("f%02d.log", i+1)})
	}
	got := len(withheldRows(acts))
	want := 12
	if got != want {
		t.Errorf("rows: got %d, want %d", got, want)
	}
}

// Behavior: a summary row stands where its folder's first change would have been,
// so the withheld list keeps the order of the changes it replaces.
func TestWithheldRows_SummaryTakesFirstMembersPlace(t *testing.T) {
	acts := []compare.Action{{Verb: "create", Path: "a.txt"}}
	acts = append(acts, creates("build/x", 11)...)
	acts = append(acts, compare.Action{Verb: "delete", Path: "z.txt"})
	got := withheldRows(acts)
	want := []withheldRow{
		{label: "a.txt", actions: "create"},
		{label: "build/", count: 11, actions: "create"},
		{label: "z.txt", actions: "delete"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows: got %+v, want %+v", got, want)
	}
}

// Behavior: mixed actions are listed most frequent first, and a tie is broken by
// verb name, so the same set of changes always reads the same way.
func TestWithheldRows_ActionsOrderByCountThenVerb(t *testing.T) {
	acts := creates("logs", 4)
	for i := range 4 {
		acts = append(acts, compare.Action{Verb: "delete", Path: fmt.Sprintf("logs/d%02d", i+1)})
	}
	for i := range 5 {
		acts = append(acts, compare.Action{Verb: "update", Path: fmt.Sprintf("logs/u%02d", i+1)})
	}
	got := withheldRows(acts)[0].actions
	want := "5 update · 4 create · 4 delete"
	if got != want {
		t.Errorf("actions: got %q, want %q", got, want)
	}
}

// Behavior: a large count is written with thousands separators, matching how
// people read the size of a folder like .ansible/ at a glance.
func TestGroupDigits(t *testing.T) {
	for n, want := range map[int]string{7: "7", 999: "999", 1000: "1,000", 3412: "3,412", 1234567: "1,234,567"} {
		got := groupDigits(n)
		if got != want {
			t.Errorf("groupDigits(%d): got %q, want %q", n, got, want)
		}
	}
}
