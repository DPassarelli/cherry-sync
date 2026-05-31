// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"fmt"
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
}

// Run invokes rsync to compute the diff between source and destination.
// Both paths get a trailing slash so rsync compares directory contents
// rather than nesting source under destination.
func Run(source, destination string) (Result, error) {
	args := rsyncArgs(source, destination)
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
	return Result{Actions: actions}, nil
}

// rsyncArgs builds the argument vector for the dry-run comparison. The `--`
// end-of-options separator immediately before the paths ensures a source or
// destination beginning with `-` is parsed by rsync as a path, never as an
// option — closing off rsync argument injection (e.g. a path like `-e` or
// `--rsh=…` that would otherwise hijack rsync's remote-shell command).
func rsyncArgs(source, destination string) []string {
	return []string{
		"--dry-run",
		"--itemize-changes",
		"--recursive",
		"--",
		source + "/",
		destination + "/",
	}
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
// v0.1 recognizes `>f...` (file being pushed) as update, with `>f+++++++++`
// (all-new attribute markers) as create. Delete and other verbs land here as
// scenarios drill them.
func actionFromLine(line string) Action {
	if len(line) < 12 {
		return Action{}
	}
	if line[0] != '>' || line[1] != 'f' {
		return Action{}
	}
	code := line[:11]
	path := strings.TrimSpace(line[11:])
	if strings.Contains(code, "+++++++++") {
		return Action{Verb: "create", Path: path}
	}
	return Action{Verb: "update", Path: path}
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

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func startsWithDigit(s string) bool {
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

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
