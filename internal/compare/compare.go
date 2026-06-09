// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"fmt"
	"os/exec"
)

// Action is a single planned change between source and destination.
type Action struct {
	Verb string
	Path string
}

// Result is the structured outcome of comparing two paths.
type Result struct {
	Actions []Action
	// Excluded is how many gitignored paths were dropped from the comparison
	// (0 when the local side isn't a git work tree or ignores nothing). The CLI
	// discloses this so the user knows files were hidden — there's no opt-out.
	Excluded int
	// GitDirExcluded reports whether the local side's .git directory was held out
	// of the comparison — true whenever the local side is a git work tree. git
	// never lists .git/ as ignored, so it's excluded explicitly; the CLI discloses
	// it separately from the gitignored count (it can be true with Excluded == 0).
	GitDirExcluded bool
}

// Run invokes rsync to compute the diff between source and destination.
// Both paths get a trailing slash so rsync compares directory contents
// rather than nesting source under destination.
func Run(source, destination string) (Result, error) {
	excludeFrom, excluded, gitWorkTree, cleanup, err := excludeFile(source, destination)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	args := rsyncArgs(source, destination, excludeFrom)
	// The variable args are safe by construction — see SECURITY.md: no shell
	// (exec.Command, not sh -c), a `--` separator added by rsyncArgs, and path
	// operands validated in cli.Parse. The guard is proven behaviorally by the
	// "treated as a path" scenario in compare-directories.feature, which fails
	// if the `--` is removed. gosec G204 flags the exec pattern regardless.
	out, err := exec.Command("rsync", args...).Output() // #nosec G204 -- justified above
	if err != nil {
		return Result{}, fmt.Errorf("rsync: %w", err)
	}
	actions := parseActions(string(out))
	sortActions(actions)
	// The --exclude-from pre-filter is built from `git ls-files`, which sees only
	// the local tree, so on a pull a remote-only file matching a local ignore rule
	// slips through. Re-check the surviving paths against the local repo's rules and
	// drop any it ignores, folding them into the disclosed count. Skipped when the
	// local side isn't a work tree (gitWorkTree false) — nothing to ask git about.
	if gitWorkTree {
		dir, _ := localSyncDir(source, destination)
		kept, dropped, err := dropIgnoredActions(dir, actions)
		if err != nil {
			return Result{}, err
		}
		actions = kept
		excluded += dropped
	}
	return Result{Actions: actions, Excluded: excluded, GitDirExcluded: gitWorkTree}, nil
}

// rsyncArgs builds the argument vector for the dry-run comparison. The `--`
// end-of-options separator immediately before the paths ensures a source or
// destination beginning with `-` is parsed by rsync as a path, never as an
// option — closing off rsync argument injection (e.g. a path like `-e` or
// `--rsh=…` that would otherwise hijack rsync's remote-shell command).
func rsyncArgs(source, destination, excludeFrom string) []string {
	args := []string{
		"--dry-run",
		"--itemize-changes",
		"--recursive",
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
	// --exclude-from drops gitignored paths from the comparison when the local
	// side is a git work tree (empty otherwise). It's an option, so it precedes
	// the `--`; and it's compare-only — see excludeFile for why transfer omits it.
	if excludeFrom != "" {
		args = append(args, "--exclude-from="+excludeFrom)
	}
	args = append(args,
		"--",
		source+"/",
		destination+"/",
	)
	return args
}
