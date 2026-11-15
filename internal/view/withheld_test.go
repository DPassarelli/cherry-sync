package view

import (
	"fmt"
	"reflect"
	"strings"
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

// deletes returns a delete action for each path, in order, the way rsync reports
// a folder's removal: its contents first, then the folder entries themselves.
func deletes(paths ...string) []compare.Action {
	acts := make([]compare.Action, len(paths))
	for i, p := range paths {
		acts[i] = compare.Action{Verb: "delete", Path: p}
	}
	return acts
}

// Behavior: a deleted folder takes one row however few files it holds, owned by
// the outermost folder being deleted and counting only the files beneath it.
func TestWithheldRows_DeletedFolderIsOneRow(t *testing.T) {
	acts := deletes("build/a/f01", "build/a/f02", "build/b.txt", "build/a/", "build/")
	got := withheldRows(acts)
	want := []withheldRow{{label: "build/", count: 3, actions: "delete", removesFolder: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows: got %+v, want %+v", got, want)
	}
}

// Behavior: a deleted subfolder collapses to one row of its own while the kept
// folder around it is still listed change by change.
func TestWithheldRows_DeletedSubfolderIsOneRow(t *testing.T) {
	acts := []compare.Action{{Verb: "create", Path: "logs/a.log"}}
	acts = append(acts, deletes("logs/old/x.log", "logs/old/")...)
	got := withheldRows(acts)
	want := []withheldRow{
		{label: "logs/a.log", actions: "create"},
		{label: "logs/old/", count: 1, actions: "delete", removesFolder: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows: got %+v, want %+v", got, want)
	}
}

// Rule 1: the summary threshold counts files only, so a deleted subfolder's own
// entry does not tip a folder of ten files into a summary.
func TestWithheldRows_ThresholdCountsFilesOnly(t *testing.T) {
	acts := creates("logs", 9)
	acts = append(acts, deletes("logs/old/x.log", "logs/old/")...)
	got := len(withheldRows(acts))
	want := 10
	if got != want {
		t.Errorf("rows: got %d, want %d", got, want)
	}
}

// Rule 2: a top-level summary absorbs a deleted subfolder beneath it, counting
// its files among the folder's changes.
func TestWithheldRows_SummaryAbsorbsDeletedSubfolder(t *testing.T) {
	acts := creates("logs", 9)
	acts = append(acts, deletes("logs/old/x.log", "logs/old/y.log", "logs/old/")...)
	got := withheldRows(acts)
	want := []withheldRow{{label: "logs/", count: 11, actions: "9 create · 2 delete"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows: got %+v, want %+v", got, want)
	}
}

// Behavior: a deleted folder holding a single file says "file", not "files".
func TestWithheld_DeletedFolderOfOneFileIsSingular(t *testing.T) {
	got := Withheld(deletes("tmp/x", "tmp/"))
	want := "delete  (with 1 file)"
	if !strings.Contains(got, want) {
		t.Errorf("Withheld: got %q, want it to contain %q", got, want)
	}
}
