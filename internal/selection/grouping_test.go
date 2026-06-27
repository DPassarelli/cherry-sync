package selection

import (
	"reflect"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// TestGroupByDir checks that GroupByDir buckets compare-sorted actions into one
// group per directory, preserving input order, with each header rendered
// relative to the transfer root ("./" for a root-level file, "./sub" for a
// nested one). Each case is one condition that changes the grouping outcome.
func TestGroupByDir(t *testing.T) {
	cases := []struct {
		name string
		in   []compare.Action
		want []Group
	}{
		{
			name: "no actions yields no groups",
			in:   nil,
			want: nil,
		},
		{
			name: "a root-level file groups under ./",
			in:   []compare.Action{{Verb: "update", Path: "README.md"}},
			want: []Group{
				{Dir: "./", Actions: []compare.Action{{Verb: "update", Path: "README.md"}}},
			},
		},
		{
			name: "a file in a subdirectory groups under ./<dir>",
			in:   []compare.Action{{Verb: "create", Path: "_features/user_selection.feature"}},
			want: []Group{
				{Dir: "./_features", Actions: []compare.Action{{Verb: "create", Path: "_features/user_selection.feature"}}},
			},
		},
		{
			name: "a deeply nested file uses its full parent path",
			in:   []compare.Action{{Verb: "update", Path: "src/a/b.go"}},
			want: []Group{
				{Dir: "./src/a", Actions: []compare.Action{{Verb: "update", Path: "src/a/b.go"}}},
			},
		},
		{
			name: "files in the same directory share one group, in order",
			in: []compare.Action{
				{Verb: "update", Path: "src/main.go"},
				{Verb: "create", Path: "src/adder.go"},
			},
			want: []Group{
				{Dir: "./src", Actions: []compare.Action{
					{Verb: "update", Path: "src/main.go"},
					{Verb: "create", Path: "src/adder.go"},
				}},
			},
		},
		{
			name: "files in different directories form separate groups, in input order",
			in: []compare.Action{
				{Verb: "update", Path: "README.md"},
				{Verb: "create", Path: "_features/user_selection.feature"},
				{Verb: "update", Path: "src/main.go"},
			},
			want: []Group{
				{Dir: "./", Actions: []compare.Action{{Verb: "update", Path: "README.md"}}},
				{Dir: "./_features", Actions: []compare.Action{{Verb: "create", Path: "_features/user_selection.feature"}}},
				{Dir: "./src", Actions: []compare.Action{{Verb: "update", Path: "src/main.go"}}},
			},
		},
		{
			// compare's sort interleaves a directory's files with subdirectories
			// (dot-before-non-dot, file-before-subdir at each level), so two root
			// files can arrive with a subdirectory between them. They must still
			// coalesce into one "./" group rather than producing a second heading.
			name: "non-contiguous rows in the same directory coalesce into one group",
			in: []compare.Action{
				{Verb: "update", Path: ".goreleaser.yaml"},
				{Verb: "create", Path: ".github/workflows/ci.yml"},
				{Verb: "create", Path: "NOTES.md"},
			},
			want: []Group{
				{Dir: "./", Actions: []compare.Action{
					{Verb: "update", Path: ".goreleaser.yaml"},
					{Verb: "create", Path: "NOTES.md"},
				}},
				{Dir: "./.github/workflows", Actions: []compare.Action{
					{Verb: "create", Path: ".github/workflows/ci.yml"},
				}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupByDir(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("GroupByDir(%+v)\n got: %+v\nwant: %+v", tc.in, got, tc.want)
			}
		})
	}
}
