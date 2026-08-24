// Package transfer runs rsync to copy a chosen set of files from a source to a
// destination, using rsync's --files-from to move only the selected paths.
package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// Run transfers exactly the given relative paths from source to destination,
// running rsync through r so the invocation lands in the run log. Both paths get a
// trailing slash so the paths in the list are interpreted relative to the source
// root and recreated under the destination root.
//
// The path list is fed to rsync on stdin via --files-from=- and NUL-delimited
// with --from0, so a newline embedded in a filename cannot smuggle additional
// entries into the transfer set — a SECURITY.md invariant. Passing an empty
// list is a no-op.
func Run(ctx context.Context, r *command.Runner, source, destination string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := rsyncArgs(source, destination)
	// Each path terminated by a NUL (not separated) so --from0 reads them all,
	// including a trailing one, without a spurious empty final entry.
	stdin := strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := r.Run(ctx, "rsync", args, stdin)
	if err != nil {
		// The runner captures stdout and stderr apart; rejoin them so the error
		// still carries rsync's full diagnostic, as CombinedOutput did before.
		return fmt.Errorf("rsync: %w: %s", err, append(out.Stdout, out.Stderr...))
	}
	return nil
}

// Remove deletes exactly the given relative paths from the destination, leaving
// every other destination file in place, running rsync through r so the invocation
// lands in the run log. It is a second rsync pass, separate from
// Run's transfer: --files-from can only move files, not remove them, so removals
// go through --delete constrained by a filter. Each path and its ancestor
// directories are --include'd and everything else --exclude'd, so --delete prunes
// only the selected files. Both roots get a trailing slash so the paths are
// interpreted relative to them. An empty list is a no-op.
//
// The include patterns carry the filenames as inert argv entries (no shell) and a
// trailing `--` guards the path operands, mirroring Run. A filename holding an
// rsync filter metacharacter (`*`, `?`, `[`) or a leading space would be read as a
// pattern rather than a literal; such names are dropped upstream at detection and
// never reach here, so the patterns built below match literally.
func Remove(ctx context.Context, r *command.Runner, source, destination string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := removeArgs(source, destination, paths)
	out, err := r.Run(ctx, "rsync", args, nil)
	if err != nil {
		// Rejoin the runner's separate stdout/stderr so the error carries rsync's
		// full diagnostic, as CombinedOutput did before — see Run.
		return fmt.Errorf("rsync: %w: %s", err, append(out.Stdout, out.Stderr...))
	}
	return nil
}

// removeArgs builds the argument vector for the deletion pass: --delete scoped by
// an include/exclude filter to the selected paths. rsync evaluates filter rules
// first-match-wins and won't descend into a directory it hasn't included, so each
// target's ancestor directories are included ahead of the trailing --exclude='*'
// that protects everything else from deletion. As in rsyncArgs, `--` before the
// paths closes off rsync argument injection.
func removeArgs(source, destination string, paths []string) []string {
	args := []string{
		"--recursive",
		"--delete",
	}
	seen := map[string]bool{}
	for _, p := range paths {
		for _, inc := range includePatterns(p) {
			if seen[inc] {
				continue
			}
			seen[inc] = true
			args = append(args, "--include="+inc)
		}
	}
	args = append(args,
		"--exclude=*",
		"--",
		source+"/",
		destination+"/",
	)
	return args
}

// includePatterns returns the anchored rsync filter includes that let --delete
// reach a single relative path: one per ancestor directory (with a trailing
// slash, so rsync descends into it) in top-down order, then the file itself. Each
// is anchored with a leading slash to pin it to the transfer root, so it can't
// match a same-named entry deeper in the tree.
func includePatterns(p string) []string {
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	var prefix strings.Builder
	for i, seg := range segs {
		prefix.WriteByte('/')
		prefix.WriteString(seg)
		if i < len(segs)-1 {
			out = append(out, prefix.String()+"/")
		} else {
			out = append(out, prefix.String())
		}
	}
	return out
}

// rsyncArgs builds the argument vector for the transfer. As in compare.rsyncArgs,
// the `--` end-of-options separator immediately before the paths ensures a
// source or destination beginning with `-` reaches rsync as a path, never an
// option — closing off rsync argument injection (e.g. `--rsh=…`).
func rsyncArgs(source, destination string) []string {
	return []string{
		"--recursive",
		// --times preserves each transferred file's modification time. Without
		// it, every synced file lands with "now" as its mtime and rsync's
		// quick-check (size + mtime) re-flags it as changed on the next compare —
		// a perpetual phantom update. Must match compare.rsyncArgs.
		"--times",
		"--files-from=-",
		"--from0",
		"--",
		source + "/",
		destination + "/",
	}
}
