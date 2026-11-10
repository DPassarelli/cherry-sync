package main

import (
	"runtime/debug"
	"testing"
)

// TestShortCommit covers which commit a dev build names: Go's recorded revision,
// abbreviated to the length git itself shows, or nothing when the build recorded
// no revision (built outside a git checkout, or via `go run`).
func TestShortCommit(t *testing.T) {
	cases := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{
			"full revision",
			[]debug.BuildSetting{
				{Key: "vcs", Value: "git"},
				{Key: "vcs.revision", Value: "e608fd7175fe37cf5f38912ae11cf5b545cbe453"},
				{Key: "vcs.modified", Value: "true"},
			},
			"e608fd7",
		},
		{"no revision", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}}, ""},
		{"revision shorter than abbreviation", []debug.BuildSetting{{Key: "vcs.revision", Value: "e608"}}, "e608"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shortCommit(c.settings)

			if got != c.want {
				t.Errorf("shortCommit: got %q, want %q", got, c.want)
			}
		})
	}
}
