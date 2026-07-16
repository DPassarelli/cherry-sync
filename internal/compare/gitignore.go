// gitignore.go holds the local-side exclusion logic: csync's own .csync.toml
// (always, when present), the git-authoritative excludes built from the local
// repo's ignore rules, dropping ignored remote-only paths that slip past that
// pre-filter on a pull, and the git interrogation helpers (`ls-files`,
// `check-ignore`, `rev-parse`) behind them.

package compare

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// exclusions holds what csync withholds from the comparison for the local side of
// a sync: the rsync --exclude patterns to apply, how many of them are gitignored
// paths (the count the CLI discloses), whether the local side is a git work tree
// at all, and whether csync's own .csync.toml was among the withheld (csyncToml),
// which the CLI discloses separately like .git/. Its zero value means nothing was
// hidden — no patterns, no config file, not a work tree.
type exclusions struct {
	patterns   []string
	gitignored []string
	inWorkTree bool
	csyncToml  bool
}

// localExclusions gathers the exclusions for the local side of a sync. Two
// independent sources contribute, so the zero value (nothing hidden) is returned
// only when neither applies.
//
// csync's own .csync.toml is excluded whenever it is present — unconditional on
// git — because its saved remote is meaningless on the other machine and offering
// it would clutter every diff. The "/" anchors it to the transfer root (it lives
// only there, cwd-only discovery), and csyncToml records it so the CLI can
// disclose it like .git/.
//
// When the local side is a git work tree, patterns also gains ".git": git never
// reports its own metadata directory as ignored (it special-cases .git/), so
// without an explicit exclude a push/pull from a repo would offer every .git/
// object for transfer — noise that would also clobber the other side's git state.
// The pattern is floating (no leading '/', matching at any depth) and slash-free
// (matching a .git that is either a directory or a file), so it also holds out the
// nested git metadata a submodule carries: a checked-out submodule's .git is a
// *file* — a "gitdir:" pointer — deep in the tree, which an anchored or
// directory-only pattern would miss. This is the one exclude we deliberately float,
// because a .git is git metadata regardless of depth, unlike the gitignore paths
// (which are anchored to keep a top-level rule off a same-named nested path). The
// gitignored count covers only the gitignored paths, not this .git entry, which the
// CLI discloses separately.
//
// The patterns are applied to the comparison ONLY; the transfer uses --files-from
// and never re-derives them. That's safe because the comparison is the single gate:
// an excluded file never appears, so it can't be selected, so --files-from never
// lists one.
func localExclusions(r *command.Runner, source, destination string) (exclusions, error) {
	dir, ok := localSyncDir(source, destination)
	if !ok {
		return exclusions{}, nil
	}
	var exc exclusions
	if hasCsyncToml(dir) {
		exc.patterns = append(exc.patterns, "/.csync.toml")
		exc.csyncToml = true
	}
	if isGitWorkTree(dir) {
		gitignored, err := gitignoreExcludes(r, dir)
		if err != nil {
			return exclusions{}, err
		}
		exc.patterns = append(exc.patterns, ".git")
		exc.patterns = append(exc.patterns, gitignored...)
		exc.gitignored = excludedNames(gitignored)
		exc.inWorkTree = true
	}
	return exc, nil
}

// hasCsyncToml reports whether dir contains a .csync.toml file — a regular file,
// not a directory of that name. It gates both the config-file exclusion and its
// disclosure, so a sync of a directory without one stays silent.
func hasCsyncToml(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".csync.toml"))
	if err != nil {
		return false
	}
	return !info.IsDir()
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
//
// It runs through r so the query lands in the run log. `-C dir` stands in for the
// working directory the direct call set — equivalent for git's repo discovery and
// relative-path output (verified by experiment), and it keeps the logged invocation
// self-describing. The work-tree probe above stays a direct, unlogged capability check.
func gitignoreExcludes(r *command.Runner, dir string) ([]string, error) {
	if !isGitWorkTree(dir) {
		return nil, nil
	}
	args := []string{"-C", dir, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory"}
	out, err := r.Run("git", args, nil)
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	trimmed := strings.Trim(string(out.Stdout), "\n")
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
// ignores, returning the surviving actions and the names of those dropped. It closes a
// gap the --exclude pre-filter cannot: that filter is built from `git
// ls-files`, which lists only files present in the LOCAL tree, so on a pull a file
// that exists only on the remote yet matches a local ignore rule slips past it and
// would be pulled. checkIgnored evaluates each surviving path against the local
// repo's ignore rules — file existence not required — catching exactly those
// remote-only cases. The two filters are disjoint: the pre-filter removes ignored
// LOCAL files before rsync ever walks them, so they never reach this list, and this
// pass only ever removes paths that survived to the comparison. The dropped names
// join the disclosed set, since these are gitignored paths held back too.
func dropIgnoredActions(r *command.Runner, dir string, actions []Action) ([]Action, []string, error) {
	if len(actions) == 0 {
		return actions, nil, nil
	}
	paths := make([]string, len(actions))
	for i, a := range actions {
		paths[i] = a.Path
	}
	ignored, err := checkIgnored(r, dir, paths)
	if err != nil {
		return nil, nil, err
	}
	if len(ignored) == 0 {
		return actions, nil, nil
	}
	kept := make([]Action, 0, len(actions))
	var dropped []string
	for _, a := range actions {
		if ignored[a.Path] {
			dropped = append(dropped, a.Path)
			continue
		}
		kept = append(kept, a)
	}
	return kept, dropped, nil
}

// excludedNames turns the rsync exclude patterns from gitignoreExcludes into the plain
// names the CLI and run log show: each pattern is root-anchored with a leading slash
// (rsync syntax), which is stripped so "/build/" reads as "build/" — the form that
// matches the .gitignore rule a user wrote and would search the log for.
func excludedNames(patterns []string) []string {
	names := make([]string, len(patterns))
	for i, p := range patterns {
		names[i] = strings.TrimPrefix(p, "/")
	}
	return names
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
//
// It runs through r so the query lands in the run log; `-C dir` replaces the working
// directory the direct call set (see gitignoreExcludes). The runner returns cmd.Run's
// error unwrapped, so the exit-code-1 test below still sees the *exec.ExitError.
func checkIgnored(r *command.Runner, dir string, paths []string) (map[string]bool, error) {
	args := []string{"-C", dir, "check-ignore", "-z", "--stdin"}
	stdin := strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := r.Run("git", args, stdin)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git check-ignore: %w", err)
	}
	ignored := map[string]bool{}
	for p := range strings.SplitSeq(strings.Trim(string(out.Stdout), "\x00"), "\x00") {
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
