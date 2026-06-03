package compare

import (
	"fmt"
	"os/exec"
	"strings"
)

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

// gitignoreExcludes returns rsync exclude patterns for everything the git
// repository at dir ignores, or nil when dir is not inside a git work tree — in
// which case compare proceeds with no exclusions. It runs `git ls-files` with dir
// as the working directory, so the emitted paths are relative to dir, the rsync
// transfer root. Each path is anchored with a leading '/': rsync treats an
// unanchored entry as a basename match at any depth, which would let a top-level
// ignore (e.g. `build/`) wrongly suppress a same-named nested path (`src/build/`);
// the leading '/' pins it to the transfer root. git escapes any newline within a
// path in its own output, so joining the patterns with newlines for
// --exclude-from cannot smuggle extra entries.
func gitignoreExcludes(dir string) ([]string, error) {
	if !isGitWorkTree(dir) {
		return nil, nil
	}
	cmd := exec.Command("git", "ls-files", "--others", "--ignored", "--exclude-standard", "--directory")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	trimmed := strings.Trim(string(out), "\n")
	if trimmed == "" {
		return nil, nil
	}
	var patterns []string
	for line := range strings.SplitSeq(trimmed, "\n") {
		if line == "" {
			continue
		}
		patterns = append(patterns, "/"+line)
	}
	return patterns, nil
}

// isGitWorkTree reports whether dir lies inside a git working tree. A missing git
// binary or any git error counts as "no", so a machine without git simply gets no
// gitignore exclusions rather than a failure.
func isGitWorkTree(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
