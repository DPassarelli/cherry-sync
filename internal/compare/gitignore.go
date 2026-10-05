// gitignore.go holds csync's exclusion policy: what it withholds from a
// comparison and how that is disclosed. csync's own .csync.toml (always, when
// present), the ignored directories that become rsync --exclude patterns, the
// ignored paths that are dropped from the results instead, and the names the CLI
// and run log report. The git queries behind these answers live in git.go.

package compare

import (
	"context"

	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// exclusions holds what csync withholds from the comparison for the local side of
// a sync: the rsync --exclude patterns to apply, how many of them are gitignored
// paths (the count the CLI discloses), whether the local side is a git work tree
// at all, and whether csync's own .csync.toml was among the withheld (csyncToml),
// which the CLI discloses separately like .git/. inWorkTree gates the gitignore
// work alone; the .git pattern is unconditional, so a populated patterns list says
// nothing about whether either side holds a repository.
type exclusions struct {
	patterns   []string
	gitignored []string
	inWorkTree bool
	csyncToml  bool
}

// localExclusions gathers the exclusions csync applies to a sync. The name marks
// where they are DERIVED, not where they apply: the .csync.toml and gitignore
// parts are read off the local side, while rsync applies every pattern to
// whichever side it reads. Only a sync with no local operand at all (both ends
// remote) returns the zero value.
//
// csync's own .csync.toml is excluded whenever it is present — unconditional on
// git — because its saved remote is meaningless on the other machine and offering
// it would clutter every diff. The "/" anchors it to the transfer root (it lives
// only there, cwd-only discovery), and csyncToml records it so the CLI can
// disclose it like .git/.
//
// ".git" is excluded on every run, gated on nothing: git never reports its own
// metadata directory as ignored (it special-cases .git/), so without an explicit
// exclude a sync involving a repo would offer every .git/ object for transfer —
// noise that would also clobber the other side's git state. It cannot be gated on
// the local side being a work tree, because on a pull the repository being read is
// the remote one and no local check can see it (#103). Passing it where no .git
// exists costs nothing: the pattern matches nothing, and the disclosure reads
// rsync's report of what it actually withheld rather than this list, so a sync of
// two plain directories still says nothing about git.
//
// The pattern is floating (no leading '/', matching at any depth) and slash-free
// (matching a .git that is either a directory or a file), so it also holds out the
// nested git metadata a submodule carries: a checked-out submodule's .git is a
// *file* — a "gitdir:" pointer — deep in the tree, which an anchored or
// directory-only pattern would miss. This is the one exclude we deliberately float,
// because a .git is git metadata regardless of depth, unlike the gitignore paths
// (which are anchored to keep a top-level rule off a same-named nested path). The
// gitignored count covers only the gitignored paths, not this .git entry, which the
// CLI discloses separately (see gitDirHidden for where that disclosure comes from).
//
// The patterns are applied to the comparison ONLY; the transfer uses --files-from
// and never re-derives them. That's safe because the comparison is the single gate:
// an excluded file never appears, so it can't be selected, so --files-from never
// lists one.
func localExclusions(ctx context.Context, r *command.Runner, source, destination string) (exclusions, error) {
	dir, ok := localSyncDir(source, destination)
	if !ok {
		return exclusions{}, nil
	}
	var exc exclusions
	exc.patterns = append(exc.patterns, ".git")
	if hasCsyncToml(dir) {
		exc.patterns = append(exc.patterns, "/.csync.toml")
		exc.csyncToml = true
	}
	if isGitWorkTree(dir) {
		gitignored, err := gitignoreExcludes(ctx, r, dir)
		if err != nil {
			return exclusions{}, err
		}
		// csync withholds its own .csync.toml above, unconditionally — and a project
		// with a saved target commonly gitignores it too, so git reports it as ignored
		// as well. Drop it here so it stays one exclusion: left in, it would inflate the
		// gitignored count the CLI discloses (announcing the file twice) and reach rsync
		// as a second, redundant --exclude for the same path.
		if exc.csyncToml {
			gitignored = slices.DeleteFunc(gitignored, func(p string) bool {
				return p == "/.csync.toml"
			})
		}
		// Only the ignored DIRECTORIES become --exclude patterns. An ignored file is
		// deliberately left in the comparison so rsync says whether it differs, which is
		// what lets csync report it as a withheld change rather than a bare count (#59);
		// dropIgnoredActions removes it again before it can be offered. Directories stay
		// pre-excluded because un-excluding one makes rsync walk every file beneath it —
		// measured at roughly 5x on a large node_modules — and nobody is surprised that
		// an ignored build directory did not sync.
		exc.patterns = append(exc.patterns, ignoredDirs(gitignored)...)
		exc.gitignored = excludedNames(gitignored)
		exc.inWorkTree = true
	}
	return exc, nil
}

// ignoredDirs returns the subset of patterns that name directories, which `git
// ls-files --directory` marks with a trailing slash. These are the only gitignore
// patterns csync passes to rsync as --exclude; see localExclusions for why the
// ignored files are deliberately left in.
func ignoredDirs(patterns []string) []string {
	var dirs []string
	for _, p := range patterns {
		if strings.HasSuffix(p, "/") {
			dirs = append(dirs, p)
		}
	}
	return dirs
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

// dropIgnoredActions removes from actions any whose path the git repository at dir
// ignores, returning the surviving actions and the names of those dropped. It closes a
// gap the --exclude pre-filter cannot: that filter is built from `git
// ls-files`, which lists only files present in the LOCAL tree, so on a pull a file
// that exists only on the remote yet matches a local ignore rule slips past it and
// would be pulled. checkIgnored evaluates each surviving path against the local
// repo's ignore rules — file existence not required — catching exactly those
// remote-only cases. It is also what removes the ignored LOCAL files that
// localExclusions now deliberately leaves in the comparison, so this pass carries
// every gitignored change csync declined to offer. The dropped actions are returned
// whole rather than as names because the withheld disclosure reports each one's verb
// alongside its path.
func dropIgnoredActions(ctx context.Context, r *command.Runner, dir string, actions []Action) ([]Action, []Action, error) {
	if len(actions) == 0 {
		return actions, nil, nil
	}
	paths := make([]string, len(actions))
	for i, a := range actions {
		paths[i] = a.Path
	}
	ignored, err := checkIgnored(ctx, r, dir, paths)
	if err != nil {
		return nil, nil, err
	}
	if len(ignored) == 0 {
		return actions, nil, nil
	}
	kept := make([]Action, 0, len(actions))
	var dropped []Action
	for _, a := range actions {
		if ignored[a.Path] {
			dropped = append(dropped, a)
			continue
		}
		kept = append(kept, a)
	}
	return kept, dropped, nil
}

// mergeExcluded adds each dropped action's path to names unless it is already
// there. A gitignored file in the local tree is now disclosed twice over — once by
// `git ls-files` and again when this pass drops its change — and counting it twice
// would tell the user more paths were held back than were.
func mergeExcluded(names []string, dropped []Action) []string {
	for _, a := range dropped {
		if slices.Contains(names, a.Path) {
			continue
		}
		names = append(names, a.Path)
	}
	return names
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
