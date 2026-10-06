// operands.go tells the two sides of a sync apart: which of a source/destination
// pair names a remote host, and which one names a directory on this machine. Both
// answers are needed wherever csync must act on the local side alone — reading
// ignore rules, measuring a file — so they sit apart from any one caller.

package compare

import (
	"strings"
)

// isRemote reports whether an rsync path operand names a remote host: it holds a
// ':' that appears before any '/'. `host:/path` and `user@host:p` are remote;
// `./rel`, `/abs`, and a local `rel/with:colon` are not. Mirrors rsync's own
// colon-before-slash test for spotting a remote spec.
func isRemote(path string) bool {
	colon := strings.IndexByte(path, ':')
	if colon < 0 {
		return false
	}
	slash := strings.IndexByte(path, '/')
	return slash < 0 || colon < slash
}

// localSyncDir returns the local operand of a source/destination pair — the side
// rsync reads or writes on this machine — and whether one exists. A remote
// operand is an rsync `[user@]host:path` spec (a colon before the first slash);
// the other side is local. Source is preferred when both are local (a
// local-to-local sync), and ok is false when both are remote.
func localSyncDir(source, destination string) (string, bool) {
	if !isRemote(source) {
		return source, true
	}
	if !isRemote(destination) {
		return destination, true
	}
	return "", false
}
