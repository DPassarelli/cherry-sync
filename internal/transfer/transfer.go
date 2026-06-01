// Package transfer runs rsync to copy a chosen set of files from a source to a
// destination, using rsync's --files-from to move only the selected paths.
package transfer

import (
	"fmt"
	"os/exec"
	"strings"
)

// Run transfers exactly the given relative paths from source to destination.
// Both paths get a trailing slash so the paths in the list are interpreted
// relative to the source root and recreated under the destination root.
//
// The path list is fed to rsync on stdin via --files-from=- and NUL-delimited
// with --from0, so a newline embedded in a filename cannot smuggle additional
// entries into the transfer set — a SECURITY.md invariant. Passing an empty
// list is a no-op.
func Run(source, destination string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := rsyncArgs(source, destination)
	cmd := exec.Command("rsync", args...) // #nosec G204 -- see compare.Run / SECURITY.md
	// Each path terminated by a NUL (not separated) so --from0 reads them all,
	// including a trailing one, without a spurious empty final entry.
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync: %w: %s", err, out)
	}
	return nil
}

// rsyncArgs builds the argument vector for the transfer. As in compare.rsyncArgs,
// the `--` end-of-options separator immediately before the paths ensures a
// source or destination beginning with `-` reaches rsync as a path, never an
// option — closing off rsync argument injection (e.g. `--rsh=…`).
func rsyncArgs(source, destination string) []string {
	return []string{
		"--recursive",
		"--files-from=-",
		"--from0",
		"--",
		source + "/",
		destination + "/",
	}
}
