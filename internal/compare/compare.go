// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"fmt"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// Action is a single planned change between source and destination.
type Action struct {
	Verb string
	Path string
}

// Result is the structured outcome of comparing two paths.
type Result struct {
	Actions []Action
	// Excluded holds the gitignored paths dropped from the comparison — the names,
	// so a caller can say which files were hidden and not merely how many (the CLI's
	// header still discloses the count via len, but the run log records the names).
	// Empty when the local side isn't a git work tree or ignores nothing. There's no
	// opt-out. Each name is root-relative with no rsync anchor, e.g. "build/".
	Excluded []string
	// GitDirExcluded reports whether the local side's .git directory was held out
	// of the comparison — true whenever the local side is a git work tree. git
	// never lists .git/ as ignored, so it's excluded explicitly; the CLI discloses
	// it separately from the gitignored names (it can be true with Excluded empty).
	GitDirExcluded bool
	// CsyncTomlExcluded reports whether the local side's own .csync.toml was held
	// out of the comparison — true whenever that file is present, independent of
	// git. The CLI discloses it like the .git directory, since csync withholds it
	// with no opt-out.
	CsyncTomlExcluded bool
}

// Run invokes rsync to compute the diff between source and destination, running it
// through r so the invocation lands in the run log. Both paths get a trailing slash
// so rsync compares directory contents rather than nesting source under destination.
func Run(r *command.Runner, source, destination string) (Result, error) {
	exc, err := localExclusions(r, source, destination)
	if err != nil {
		return Result{}, err
	}

	args := rsyncArgs(source, destination, exc.patterns)
	// The variable args are safe by construction — see SECURITY.md: no shell (the
	// runner uses exec.Command, not sh -c), a `--` separator added by rsyncArgs, and
	// path operands validated in cli.Parse. The guard is proven behaviorally by the
	// "treated as a path" scenario in compare-directories.feature, which fails if the
	// `--` is removed.
	out, err := r.Run("rsync", args, nil)
	if err != nil {
		return Result{}, fmt.Errorf("rsync: %w", err)
	}
	actions := parseActions(string(out.Stdout))
	sortActions(actions)
	excluded := exc.gitignored
	// The --exclude pre-filter is built from `git ls-files`, which sees only the
	// local tree, so on a pull a remote-only file matching a local ignore rule
	// slips through. Re-check the surviving paths against the local repo's rules and
	// drop any it ignores, folding their names into the disclosed set. Skipped when the
	// local side isn't a work tree (inWorkTree false) — nothing to ask git about.
	if exc.inWorkTree {
		dir, _ := localSyncDir(source, destination)
		kept, dropped, err := dropIgnoredActions(r, dir, actions)
		if err != nil {
			return Result{}, err
		}
		actions = kept
		excluded = append(excluded, dropped...)
	}
	return Result{Actions: actions, Excluded: excluded, GitDirExcluded: exc.inWorkTree, CsyncTomlExcluded: exc.csyncToml}, nil
}

// rsyncArgs builds the argument vector for the dry-run comparison. The `--`
// end-of-options separator immediately before the paths ensures a source or
// destination beginning with `-` is parsed by rsync as a path, never as an
// option — closing off rsync argument injection (e.g. a path like `-e` or
// `--rsh=…` that would otherwise hijack rsync's remote-shell command).
func rsyncArgs(source, destination string, excludes []string) []string {
	args := []string{
		"--dry-run",
		"--itemize-changes",
		"--recursive",
		// --delete surfaces removals: a path present on the destination but gone
		// from the source itemizes as `*deleting <path>`, which parseActions turns
		// into a delete Action. Detection is always on — there's no mirror flag —
		// so every compare reports the destination's stale files alongside the
		// creates and updates. It's a dry-run, so nothing is removed here; applying
		// a selected deletion is the transfer's job. Excluded paths (the gitignore
		// --exclude list below) are protected from deletion by rsync, so ignored
		// files are never proposed for removal.
		"--delete",
		// --times mirrors transfer.rsyncArgs so the dry-run models the same
		// operation the transfer performs. It does NOT change which files are
		// reported — rsync's quick-check compares mtime regardless of this flag,
		// and a dry-run writes nothing — so the actual phantom-update fix lives
		// in transfer preserving mtime. Kept here to stop the two flag sets
		// drifting as fidelity flags (perms, links, …) are added later.
		"--times",
		// --checksum compares by content hash instead of rsync's default
		// size+mtime quick-check. Without it, a file that is byte-identical but
		// carries a different mtime (e.g. git stamps a fresh mtime on every
		// checkout, so it varies per machine) itemizes as `>f..t......` and is
		// reported as a phantom "update" — indistinguishable from a real same-size
		// edit, which produces the identical code. With --checksum the phantom
		// drops to `.f..t......` (no content bit), which parseActions ignores,
		// while a real edit keeps the `c` bit and is still reported. Compare-only:
		// the transfer uses --files-from and never re-derives this set. Cost is a
		// content hash of each candidate on both ends; acceptable for this tool's
		// dev-sync use.
		"--checksum",
		// -8 (--8-bit-output) makes rsync emit the raw bytes of a non-ASCII path
		// in its --itemize-changes output. By default rsync octal-escapes high-bit
		// bytes (a UTF-8 name like café.txt prints as `\#303\#251`); parseActions
		// would carry that escaped text through to the transfer's --files-from,
		// which wants the literal bytes, so rsync looks for a file that doesn't
		// exist and fails (exit 23) — breaking ANY accented/emoji/CJK name, and the
		// macOS-screenshot narrow-no-break-space that surfaced it. The short form is
		// used over --8-bit-output because openrsync (the Mac's rsync) lists only
		// -8. Scope: -8 covers high-bit bytes only; rsync still escapes true control
		// chars (newline/tab) regardless, which keeps the space/line parser safe but
		// is why an embedded-newline name is a separate, harder problem.
		"-8",
	}
	// Each gitignored path (and ".git") drops out of the comparison via its own
	// --exclude; the patterns are options, so they precede the `--`. Empty when the
	// local side isn't a git work tree. Passing them as args rather than an
	// --exclude-from file keeps the patterns inert literals (a newline inside one
	// can't split it into two entries) and means no temp file to write or clean up.
	// Compare-only — see localExclusions for why the transfer omits them.
	for _, pattern := range excludes {
		args = append(args, "--exclude="+pattern)
	}
	args = append(args,
		"--",
		source+"/",
		destination+"/",
	)
	return args
}
