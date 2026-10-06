// git.go holds the git interrogation behind the exclusion policy: whether a
// directory is a work tree at all, what that repository ignores, and whether a
// given path is ignored. Each answer comes from git itself rather than from
// csync parsing .gitignore files, so csync inherits git's own precedence rules
// (nested .gitignore files, .git/info/exclude, the global core.excludesFile)
// without reimplementing any of them.

package compare

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

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
func gitignoreExcludes(ctx context.Context, r *command.Runner, dir string) ([]string, error) {
	if !isGitWorkTree(dir) {
		return nil, nil
	}
	args := []string{"-C", dir, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory"}
	out, err := r.Run(ctx, "git", args, nil)
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
func checkIgnored(ctx context.Context, r *command.Runner, dir string, paths []string) (map[string]bool, error) {
	args := []string{"-C", dir, "check-ignore", "-z", "--stdin"}
	stdin := strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := r.Run(ctx, "git", args, stdin)
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
