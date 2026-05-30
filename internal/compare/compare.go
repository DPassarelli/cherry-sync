// Package compare runs rsync in dry-run mode against two paths and turns its
// --itemize-changes output into a structured list of planned actions.
package compare

import (
	"fmt"
	"os/exec"
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
	return Result{Actions: parseActions(string(out))}, nil
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
