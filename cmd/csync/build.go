// build.go identifies the running binary: its injected release version and,
// for a dev build, the commit it was built from.

package main

import (
	"runtime/debug"

	"github.com/dpassarelli/cherry-sync/internal/view"
)

// currentBuild returns the identity of the running binary, the one value every
// version display reads.
func currentBuild() view.Build {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return view.Build{Version: version}
	}
	return view.Build{Version: version, Commit: shortCommit(info.Settings)}
}

// shortCommitLength matches git's default abbreviation, so the hash a dev build
// shows can be pasted straight into git.
const shortCommitLength = 7

// shortCommit returns the revision Go recorded from the source checkout,
// abbreviated, or "" when it recorded none.
func shortCommit(settings []debug.BuildSetting) string {
	for _, s := range settings {
		if s.Key == "vcs.revision" {
			return s.Value[:min(len(s.Value), shortCommitLength)]
		}
	}
	return ""
}
