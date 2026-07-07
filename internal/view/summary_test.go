package view

import (
	"strings"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestRenderSummary covers how the post-sync header counts the applied changes
// and, when some were removals, calls them out distinctly. With no deletes it
// stays in the plain "(N files)" form; with deletes it adds the "M of which
// were removed" clause, both counts and both verbs pluralized independently.
func TestRenderSummary(t *testing.T) {
	create := compare.Action{Verb: "create", Path: "a"}
	del := compare.Action{Verb: "delete", Path: "b"}

	cases := []struct {
		name     string
		selected []compare.Action
		want     string
	}{
		{"single transfer, no removals", []compare.Action{create}, "Sync complete! (1 file)"},
		{"several transfers, no removals", []compare.Action{create, create, create}, "Sync complete! (3 files)"},
		{"one removal only", []compare.Action{del}, "Sync complete! (1 file total, 1 of which was removed)"},
		{"mixed, one removal", []compare.Action{create, del}, "Sync complete! (2 files total, 1 of which was removed)"},
		{"mixed, several removals", []compare.Action{create, del, del}, "Sync complete! (3 files total, 2 of which were removed)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderSummary(tc.selected)
			if !strings.Contains(got, tc.want) {
				t.Errorf("RenderSummary(%s):\n got %q\n want to contain %q", tc.name, got, tc.want)
			}
		})
	}
}
