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
	args := []string{
		"--dry-run",
		"--itemize-changes",
		"--recursive",
		source + "/",
		destination + "/",
	}
	out, err := exec.Command("rsync", args...).Output()
	if err != nil {
		return Result{}, fmt.Errorf("rsync: %w", err)
	}
	return Result{Actions: parseActions(string(out))}, nil
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
