// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"context"

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
	// GitDirExcluded reports whether a .git was held out of the comparison, on
	// EITHER side — git never lists .git/ as ignored, so it is excluded explicitly,
	// and the exclude applies to whichever side rsync reads. It is read back from
	// rsync's own report of what it withheld rather than from a check of the local
	// side, so a pull from a remote repository into a plain directory discloses the
	// remote .git it held back (#103). The CLI discloses it separately from the
	// gitignored names (it can be true with Excluded empty).
	GitDirExcluded bool
	// CsyncTomlExcluded reports whether the local side's own .csync.toml was held
	// out of the comparison — true whenever that file is present, independent of
	// git. The CLI discloses it like the .git directory, since csync withholds it
	// with no opt-out.
	CsyncTomlExcluded bool
}

// Progress receives the name of each stage of the comparison as that stage
// begins, so a caller can caption a spinner with what csync is currently waiting
// on rather than leaving the wait unexplained (#62). A nil Progress is valid and
// reports nothing; compare never depends on anyone listening.
type Progress func(stage string)

// report calls p when there is one, so the stages below need no nil check apiece.
func (p Progress) report(stage string) {
	if p != nil {
		p(stage)
	}
}

// Run invokes rsync to compute the diff between source and destination, running it
// through r so the invocation lands in the run log. Both paths get a trailing slash
// so rsync compares directory contents rather than nesting source under destination.
// Each stage is announced through progress as it starts; the rsync call is the slow
// one, and the only reason the other two are announced at all is that a caption
// that never changes reads as a hung spinner.
func Run(ctx context.Context, r *command.Runner, source, destination string, progress Progress) (Result, error) {
	progress.report("reading ignore rules")
	exc, err := localExclusions(ctx, r, source, destination)
	if err != nil {
		return Result{}, err
	}

	// A remote operand means the wait is network-bound, which is worth naming: it
	// tells the user the delay is the far end, not csync spinning locally.
	stage := "comparing files"
	if isRemote(source) || isRemote(destination) {
		stage = "querying remote"
	}
	progress.report(stage)

	args := rsyncArgs(source, destination, exc.patterns)
	// The variable args are safe by construction — see SECURITY.md: no shell (the
	// runner uses exec.Command, not sh -c), a `--` separator added by rsyncArgs, and
	// path operands validated in cli.Parse. The guard is proven behaviorally by the
	// "treated as a path" scenario in compare-directories.feature, which fails if the
	// `--` is removed.
	out, err := r.Run(ctx, "rsync", args, nil)
	if err != nil {
		// Returned as the Runner framed it, which already names rsync and carries
		// what rsync said. Re-wrapping here would only repeat the program's name.
		return Result{}, err
	}
	stdout := string(out.Stdout)
	actions := parseActions(stdout)
	sortActions(actions)
	excluded := exc.gitignored
	// The --exclude pre-filter is built from `git ls-files`, which sees only the
	// local tree, so on a pull a remote-only file matching a local ignore rule
	// slips through. Re-check the surviving paths against the local repo's rules and
	// drop any it ignores, folding their names into the disclosed set. Skipped when the
	// local side isn't a work tree (inWorkTree false) — nothing to ask git about.
	if exc.inWorkTree {
		progress.report("building the list")
		dir, _ := localSyncDir(source, destination)
		kept, dropped, err := dropIgnoredActions(ctx, r, dir, actions)
		if err != nil {
			return Result{}, err
		}
		actions = kept
		excluded = append(excluded, dropped...)
	}
	return Result{Actions: actions, Excluded: excluded, GitDirExcluded: gitDirHidden(stdout), CsyncTomlExcluded: exc.csyncToml}, nil
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
		// -vv makes rsync name each path an --exclude held back, on stdout: GNU
		// emits `[sender] hiding directory X because of pattern P` (and
		// `[generator] protecting directory X ...` for the receiving side),
		// openrsync `rsync(PID): : hiding file X because of pattern` and
		// `rsync(PID): : skip excluded file X`. That output is what lets csync
		// disclose a .git it withheld from EITHER side — including a remote one,
		// which no local check can see (#103). Verified that a remote GNU sender's
		// message is relayed intact to an openrsync client, so the evidence
		// survives the wire in both directions. The cost is a handful of extra
		// lines per run, not per file: rsync names a hidden directory once and
		// doesn't enumerate its contents. None of the added lines can be mistaken
		// for an itemize line — actionFromLine requires a first token of
		// `*deleting` or `[<>]f...` — so parseActions is unaffected.
		"-vv",
	}
	// Each gitignored path (and ".git") drops out of the comparison via its own
	// --exclude; the patterns are options, so they precede the `--`. Never empty —
	// ".git" is always among them. Passing them as args rather than an
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
