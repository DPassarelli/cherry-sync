// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
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
	return Result{Actions: actions, Excluded: excluded, GitDirExcluded: gitWorkTree}, nil
}

// excludeFile writes the local side's exclude patterns to a temp file and returns
// its path (for rsync's --exclude-from), how many *gitignored* paths it holds (for
// disclosure), whether the local side is a git work tree, a cleanup func to remove
// the file, and any error. When the local side is not a git work tree it returns an
// empty path, a zero count, false, and a no-op cleanup, so compare runs exactly as
// before — no repo, no exclusions.
//
// When the local side IS a work tree, the list always begins with "/.git/": git
// never reports its own metadata directory as ignored (it special-cases .git/), so
// without an explicit, anchored exclude a push/pull from a repo would offer every
// .git/ object for transfer — noise that would also clobber the other side's git
// state. The returned count covers only the gitignored paths, not this .git/ entry,
// which the CLI discloses separately. A leading "/" anchors it to the transfer root
// (see the floating-".git" TODO in honor-gitignore.feature for the submodule case).
//
// The list is applied to the comparison ONLY. The transfer uses --files-from, and
// rsync ignores --exclude-from when --files-from is present, so an exclude there
// would be a no-op on some rsync builds and active on others — inconsistent. The
// comparison is the single gate: excluded files never appear, so they can't be
// selected, so --files-from never lists one.
func excludeFile(source, destination string) (string, int, bool, func(), error) {
	noop := func() {}
	dir, ok := localSyncDir(source, destination)
	if !ok {
		return "", 0, false, noop, nil
	}
	if !isGitWorkTree(dir) {
		return "", 0, false, noop, nil
	}
	gitignored, err := gitignoreExcludes(dir)
	if err != nil {
		return "", 0, false, noop, err
	}
	patterns := append([]string{"/.git/"}, gitignored...)
	f, err := os.CreateTemp("", "csync-exclude-*")
	if err != nil {
		return "", 0, false, noop, fmt.Errorf("create exclude file: %w", err)
	}
	cleanup := func() { os.Remove(f.Name()) }
	_, err = f.WriteString(strings.Join(patterns, "\n") + "\n")
	if err != nil {
		f.Close()
		cleanup()
		return "", 0, false, noop, fmt.Errorf("write exclude file: %w", err)
	}
	err = f.Close()
	if err != nil {
		cleanup()
		return "", 0, false, noop, fmt.Errorf("close exclude file: %w", err)
	}
	return f.Name(), len(gitignored), true, cleanup, nil
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

// parseActions walks rsync's --itemize-changes output and returns one
// Action per change line. Non-itemize lines (preamble, summary) are
// skipped naturally because they don't match the `>f` prefix.
func parseActions(rsyncOut string) []Action {
	var actions []Action
	for line := range strings.SplitSeq(rsyncOut, "\n") {
		action := actionFromLine(line)
		if action.Verb != "" {
			actions = append(actions, action)
		}
	}
	return actions
}

// actionFromLine translates one rsync --itemize-changes line into an Action.
// v0.1 recognizes a regular-file transfer in either direction — `>f...` (received
// into the destination: pull or local-to-local) and `<f...` (sent to a remote:
// a real push over SSH) — as update, with an all-`+` attribute run (a brand-new
// file) as create. The leading byte is the transfer direction, not a distinct
// change, so both map to the same verb. Delete and other verbs land here as
// scenarios drill them.
//
// An itemize line is "<code> <path>": a whitespace-free change code, one space,
// then the path. The code's width is implementation-specific — GNU rsync emits
// 11 chars, macOS's openrsync 9 — so we split on the first space rather than a
// fixed offset, which would otherwise eat the path's first byte under openrsync.
// The path is taken verbatim (no trim), preserving a filename's own leading or
// trailing spaces.
func actionFromLine(line string) Action {
	code, path, found := strings.Cut(line, " ")
	if !found {
		return Action{}
	}
	if len(code) < 2 || (code[0] != '<' && code[0] != '>') || code[1] != 'f' || path == "" {
		return Action{}
	}
	// A newly created file marks every attribute column '+', however many this
	// rsync emits (9 under GNU, 7 under openrsync). An all-`+` tail after the
	// 2-char type prefix means create; anything else is an in-place update.
	if isAllPlus(code[2:]) {
		return Action{Verb: "create", Path: path}
	}
	return Action{Verb: "update", Path: path}
}

// isAllPlus reports whether s is non-empty and every byte is '+'. It identifies
// rsync's "newly created" attribute run independent of how many columns the
// running rsync emits.
func isAllPlus(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '+' {
			return false
		}
	}
	return true
}

// sortActions orders actions the way a file tree presents them, so the
// displayed list — and the numbering the user selects against — is stable and
// predictable rather than rsync's directory-grouped emit order. See the
// contract documented in features/order-reported-actions.feature.
func sortActions(actions []Action) {
	slices.SortFunc(actions, func(a, b Action) int {
		return comparePaths(a.Path, b.Path)
	})
}

// comparePaths compares two relative paths segment by segment, applying these
// keys in order at each level: (1) dot entries before non-dot, (2) files before
// subdirectories, (3) the segment ordering in compareSegment.
func comparePaths(a, b string) int {
	as := strings.Split(a, "/")
	bs := strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		aSeg, bSeg := as[i], bs[i]
		aIsFile := i == len(as)-1
		bIsFile := i == len(bs)-1

		ad, bd := isDotSegment(aSeg), isDotSegment(bSeg)
		if ad != bd {
			return firstIf(ad)
		}
		if aIsFile != bIsFile {
			return firstIf(aIsFile)
		}
		if aSeg != bSeg {
			return compareSegment(aSeg, bSeg)
		}
		// identical segment: descend into the shared subdirectory
	}
	// One path is a prefix of the other (a file sharing a directory's name);
	// the shallower one sorts first.
	return len(as) - len(bs)
}

// compareSegment orders two distinct same-kind, same-dotness segments:
// number-leading names before letter-leading, numbers by value, everything
// else alphabetically (case-insensitive) with byte order breaking ties.
func compareSegment(a, b string) int {
	aNum, bNum := startsWithDigit(a), startsWithDigit(b)
	if aNum != bNum {
		return firstIf(aNum)
	}
	if aNum {
		numCmp := compareNumericRun(a, b)
		if numCmp != 0 {
			return numCmp
		}
		return strings.Compare(a, b)
	}
	nameCmp := strings.Compare(strings.ToLower(a), strings.ToLower(b))
	if nameCmp != 0 {
		return nameCmp
	}
	return strings.Compare(a, b)
}

// compareNumericRun compares the leading digit runs of a and b by numeric
// value, tolerant of leading zeros and arbitrarily long numbers (it compares
// zero-trimmed runs by length then lexically, so it never overflows).
func compareNumericRun(a, b string) int {
	na := strings.TrimLeft(leadingDigits(a), "0")
	nb := strings.TrimLeft(leadingDigits(b), "0")
	if len(na) != len(nb) {
		return len(na) - len(nb)
	}
	return strings.Compare(na, nb)
}

// leadingDigits returns the longest prefix of s made up of ASCII digits.
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// startsWithDigit reports whether s begins with an ASCII digit.
func startsWithDigit(s string) bool {
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// isDotSegment reports whether a path segment begins with a dot.
func isDotSegment(s string) bool {
	return strings.HasPrefix(s, ".")
}

// firstIf maps "should this one sort first?" to the int a comparator returns.
func firstIf(first bool) int {
	if first {
		return -1
	}
	return 1
}
