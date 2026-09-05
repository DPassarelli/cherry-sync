// counterpart.go measures the destination's copy of each changed file. rsync's
// per-file output describes only the source, so the destination's size and
// modification time are gathered separately — by stat when the destination is on
// this machine, and by a second, metadata-only rsync pass when it is not.

package compare

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// metaFields is how many `|`-separated columns a destination metadata record
// carries: the size, the modification time, and the path. Also the split limit,
// which keeps a `|` inside a filename part of the path.
const metaFields = 3

// fileMeta is what the destination's copy of a file measures — the two attributes
// a delta is computed from.
type fileMeta struct {
	size    int64
	modTime time.Time
}

// withCounterparts measures the destination's copy of each updated file and returns
// the actions carrying those measurements. Failure is not propagated: the
// measurement only annotates a row, so a destination that cannot be reached costs
// the rows their numbers and leaves the itemize labels standing, rather than failing
// a comparison that has already succeeded.
func withCounterparts(ctx context.Context, r *command.Runner, source, destination string, actions []Action, progress Progress) []Action {
	paths := updatePaths(actions)
	if len(paths) == 0 {
		return actions
	}
	progress.report("measuring differences")
	dest, err := destinationMeta(ctx, r, source, destination, paths)
	if err != nil {
		return actions
	}
	return attachCounterparts(actions, dest)
}

// updatePaths lists the paths worth asking the destination about: the updates, and
// only those. A create has no copy on the far side to measure, and a delete is
// being removed rather than compared, so including either would ask about files
// that cannot answer.
func updatePaths(actions []Action) []string {
	var paths []string
	for _, a := range actions {
		if a.Verb == "update" {
			paths = append(paths, a.Path)
		}
	}
	return paths
}

// destinationMeta measures the destination's copy of each path. A destination on
// this machine is measured with stat, which costs nothing; a remote one needs a
// second rsync pass, and that round trip is the reason the local case is worth
// separating rather than running the pass unconditionally.
func destinationMeta(ctx context.Context, r *command.Runner, source, destination string, paths []string) (map[string]fileMeta, error) {
	if !isRemote(destination) {
		return statDestination(destination, paths), nil
	}
	return remoteDestinationMeta(ctx, r, source, destination, paths)
}

// statDestination measures the destination's copy of each path on this machine. A
// path that cannot be stat'ed is left out rather than reported as zero: absent from
// the map is what "not measured" means downstream, and a zero-size entry would read
// as a real measurement of an empty file.
func statDestination(root string, paths []string) map[string]fileMeta {
	meta := make(map[string]fileMeta, len(paths))
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			continue
		}
		meta[p] = fileMeta{size: info.Size(), modTime: info.ModTime()}
	}
	return meta
}

// remoteDestinationMeta measures the destination's copy of each path by running
// rsync the other way round, so the side that was the destination is now the one
// rsync describes. The pass is deliberately cheap: no --checksum, so neither end
// hashes any content, and an explicit path list, so it stats only the handful of
// files already known to differ.
//
// Its quick check reports a file whose size or timestamp differs and stays silent
// about one that matches on both, which is exactly the set worth measuring: a file
// agreeing on size and time has no delta to state, and applyDeltas leaves it to the
// itemize labels.
func remoteDestinationMeta(ctx context.Context, r *command.Runner, source, destination string, paths []string) (map[string]fileMeta, error) {
	// Each path terminated by a NUL (not separated) so --from0 reads them all, and
	// so a newline inside a filename cannot smuggle an extra entry into the list —
	// the same handling the transfer's path list gets, and required for the same
	// reason (see SECURITY.md).
	stdin := strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := r.Run(ctx, "rsync", metaRsyncArgs(source, destination), stdin)
	if err != nil {
		return nil, err
	}
	return parseMetaLines(string(out.Stdout)), nil
}

// metaRsyncArgs builds the argument vector for the destination-measuring pass. The
// operands are reversed against the main compare — the destination is read here —
// and, as there, a `--` separator precedes them so a path beginning with `-` is
// parsed as a path and never as an option (see SECURITY.md).
func metaRsyncArgs(source, destination string) []string {
	return []string{
		"--dry-run",
		"--recursive",
		// --times matches the main compare so both passes model the same operation.
		// There is deliberately no --checksum: this pass is not deciding what differs,
		// only measuring files already known to differ, and hashing them again on both
		// ends would double the comparison's cost to learn nothing.
		"--times",
		// -8 for the same reason the main compare passes it: rsync otherwise
		// octal-escapes high-bit bytes in a path, and an escaped name would not match
		// the action it belongs to.
		"-8",
		"--files-from=-",
		"--from0",
		"--out-format=%l|%M|%n",
		"--",
		destination + "/",
		source + "/",
	}
}

// parseMetaLines reads the destination pass's records into a lookup by path. A line
// whose size or timestamp cannot be read is skipped: unlike the main compare, where
// an action survives losing its metadata, a metadata record with no readable fields
// has nothing left to contribute.
func parseMetaLines(out string) map[string]fileMeta {
	meta := make(map[string]fileMeta)
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.SplitN(line, "|", metaFields)
		if len(fields) < metaFields {
			continue
		}
		path := fields[metaFields-1]
		modTime := parseModTime(fields[1])
		// An unreadable timestamp is what separates a record from a line of rsync
		// chatter that happens to hold two pipes: %M always renders, so a record
		// always has one.
		if path == "" || modTime.IsZero() {
			continue
		}
		meta[path] = fileMeta{size: parseSize(fields[0]), modTime: modTime}
	}
	return meta
}

// attachCounterparts records, on each updated action, what the destination's copy
// of that path measures. Only updates get one: a create has no copy on the far side
// and a delete is being removed rather than compared. An action the destination
// pass said nothing about is left unmeasured, so the row falls back to the itemize
// labels rather than comparing against a zero that was never measured.
func attachCounterparts(actions []Action, dest map[string]fileMeta) []Action {
	measured := make([]Action, len(actions))
	copy(measured, actions)
	for i, a := range measured {
		if a.Verb != "update" {
			continue
		}
		other, ok := dest[a.Path]
		if !ok {
			continue
		}
		measured[i].Dest = Counterpart{Known: true, Size: other.size, ModTime: other.modTime}
	}
	return measured
}
