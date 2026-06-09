// gitignore.go holds the git-authoritative exclusion logic: building the rsync
// exclude list from the local repo's ignore rules, dropping ignored remote-only
// paths that slip past that pre-filter on a pull, and the git interrogation
// helpers (`ls-files`, `check-ignore`, `rev-parse`) behind both.

package compare

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// exclusions holds what csync withholds from the comparison for the local side of
// a sync: the rsync --exclude patterns to apply, how many of them are gitignored
// paths (the count the CLI discloses), and whether the local side is a git work
// tree at all. Its zero value means "not a work tree" — no patterns, nothing hidden.
type exclusions struct {
	patterns   []string
	gitignored int
	inWorkTree bool
}

// localExclusions gathers the exclusions for the local side of a sync. When the
// local side is not a git work tree it returns the zero value, so compare runs with
// no exclusions — no repo, nothing hidden.
//
// When the local side IS a work tree, patterns always begins with "/.git/": git
// never reports its own metadata directory as ignored (it special-cases .git/), so
// without an explicit, anchored exclude a push/pull from a repo would offer every
// .git/ object for transfer — noise that would also clobber the other side's git
// state. The leading "/" anchors it to the transfer root (see the floating-".git"
// TODO in honor-gitignore.feature for the submodule case). The gitignored count
// covers only the gitignored paths, not this .git/ entry, which the CLI discloses
// separately.
//
// The patterns are applied to the comparison ONLY; the transfer uses --files-from
// and never re-derives them. That's safe because the comparison is the single gate:
// an excluded file never appears, so it can't be selected, so --files-from never
// lists one.
func localExclusions(source, destination string) (exclusions, error) {
	dir, ok := localSyncDir(source, destination)
	if !ok {
		return exclusions{}, nil
	}
	if !isGitWorkTree(dir) {
		return exclusions{}, nil
	}
	gitignored, err := gitignoreExcludes(dir)
	if err != nil {
		return exclusions{}, err
	}
	return exclusions{
		patterns:   append([]string{"/.git/"}, gitignored...),
		gitignored: len(gitignored),
		inWorkTree: true,
	}, nil
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
// path in its own output, so splitting that output on newlines yields one pattern
// per ignored path; each then reaches rsync as its own --exclude arg, so a newline
// in a filename can neither split a pattern here nor smuggle a second one there.
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

// dropIgnoredActions removes from actions any whose path the git repository at dir
// ignores, returning the surviving actions and how many were dropped. It closes a
// gap the --exclude pre-filter cannot: that filter is built from `git
// ls-files`, which lists only files present in the LOCAL tree, so on a pull a file
// that exists only on the remote yet matches a local ignore rule slips past it and
// would be pulled. checkIgnored evaluates each surviving path against the local
// repo's ignore rules — file existence not required — catching exactly those
// remote-only cases. The two filters are disjoint: the pre-filter removes ignored
// LOCAL files before rsync ever walks them, so they never reach this list, and this
// pass only ever removes paths that survived to the comparison. The dropped count
// is added to the disclosed total, since these are gitignored paths held back too.
func dropIgnoredActions(dir string, actions []Action) ([]Action, int, error) {
	if len(actions) == 0 {
		return actions, 0, nil
	}
	paths := make([]string, len(actions))
	for i, a := range actions {
		paths[i] = a.Path
	}
	ignored, err := checkIgnored(dir, paths)
	if err != nil {
		return nil, 0, err
	}
	if len(ignored) == 0 {
		return actions, 0, nil
	}
	kept := make([]Action, 0, len(actions))
	dropped := 0
	for _, a := range actions {
		if ignored[a.Path] {
			dropped++
			continue
		}
		kept = append(kept, a)
	}
	return kept, dropped, nil
}

// checkIgnored returns the set of paths (from the given list) that the git
// repository at dir ignores, per its .gitignore / .git/info/exclude / global
// rules. It drives `git check-ignore -z --stdin`, run with dir as the working
// directory so the paths are read relative to the transfer root. The check is
// rule-based, not filesystem-based: it matches a path that does not exist locally —
// the property that lets a remote-only ignored file be caught on a pull — yet it
// respects the index, so a tracked path (e.g. one force-added past its ignore rule)
// is reported as NOT ignored. Both verified by experiment.
//
// Paths are written and read NUL-delimited (-z): a newline inside a filename then
// cannot split one entry into two — the same smuggling guard SECURITY.md requires
// for --files-from. check-ignore exits 0 when at least one path is ignored, 1 when
// none are (NOT an error: returned as an empty set), and anything else is a real
// failure (e.g. dir not a work tree) surfaced to the caller.
func checkIgnored(dir string, paths []string) (map[string]bool, error) {
	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git check-ignore: %w", err)
	}
	ignored := map[string]bool{}
	for p := range strings.SplitSeq(strings.Trim(string(out), "\x00"), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored, nil
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
