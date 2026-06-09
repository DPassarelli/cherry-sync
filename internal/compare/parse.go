// parse.go translates rsync's --itemize-changes output into the package's Action
// values — the core code-to-label step, kept width-agnostic so it reads both GNU
// rsync's and openrsync's itemize formats.

package compare

import "strings"

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
