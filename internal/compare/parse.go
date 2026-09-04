// parse.go translates rsync's --itemize-changes output into the package's Action
// values — the core code-to-label step, kept width-agnostic so it reads both GNU
// rsync's and openrsync's itemize formats.

package compare

import "strings"

// actionKey identifies a planned change for the purpose of collapsing duplicates:
// the verb and the path, and deliberately not what differs about the file. A path
// cannot take the same verb twice, so two lines sharing both describe one change
// however their attribute columns read; keying on the whole Action would let a
// byte of drift between two lines defeat the collapse.
type actionKey struct {
	verb string
	path string
}

// parseActions walks rsync's --itemize-changes output and returns one
// Action per change line, in the order first seen. Non-itemize lines
// (preamble, summary) are skipped naturally because they don't match the
// `>f`/`<f`/`*deleting` shapes. Identical (verb, path) actions are collapsed to
// one: openrsync's dry-run itemize prints a `*deleting` line for the same path
// twice where GNU rsync prints it once, and a planned action is unique by
// definition (a path can't take the same verb twice), so a repeat is always
// noise, never a second distinct change.
func parseActions(rsyncOut string) []Action {
	var actions []Action
	seen := make(map[actionKey]bool)
	for line := range strings.SplitSeq(rsyncOut, "\n") {
		action := actionFromLine(line)
		if action.Verb == "" {
			continue
		}
		key := actionKey{verb: action.Verb, path: action.Path}
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, action)
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
	// A removal itemizes as `*deleting <path>` — the code is a word, not a
	// fixed-width flag run, so it's matched by name rather than by prefix byte.
	// The path follows the code column's padding (GNU pads `*deleting` to its
	// 11-char column, openrsync's 9-char column holds it flush), so the leading
	// pad spaces are trimmed. A leading-space or glob-metachar name is handled by
	// a later stage, not here.
	if code == "*deleting" {
		p := strings.TrimLeft(path, " ")
		if p == "" {
			return Action{}
		}
		// The transfer applies a deletion through an rsync filter rule, where `*`,
		// `?`, and `[` are glob metacharacters — an unescaped one could match and
		// remove the wrong file. Escaping them safely is deferred, so a delete
		// candidate carrying one is dropped here: it is never reported, so it can't
		// be selected or applied. Only deletion is affected — the same name still
		// transfers on create/update, whose path list is literal (--files-from).
		if hasFilterMeta(p) {
			return Action{}
		}
		return Action{Verb: "delete", Path: p}
	}
	if len(code) < 2 || (code[0] != '<' && code[0] != '>') || code[1] != 'f' || path == "" {
		return Action{}
	}
	// A newly created file marks every attribute column '+', however many this
	// rsync emits (9 under GNU, 7 under openrsync). An all-`+` tail after the
	// 2-char type prefix means create; anything else is an in-place update. Those
	// markers say the file is new rather than naming an attribute that differs, so
	// a create carries no Difference: there is no counterpart to differ from.
	if isAllPlus(code[2:]) {
		return Action{Verb: "create", Path: path}
	}
	return Action{Verb: "update", Path: path, Diff: differenceFromCode(code)}
}

// differenceFromCode reads the attribute columns that say what gave a file away.
// The columns are indexed from the left — checksum, size, then time, immediately
// after the two-byte type prefix — which is where both GNU rsync and openrsync
// write them despite emitting different numbers of columns overall (verified
// against openrsync's own source, which builds the same offsets). A code too short
// to hold one reports that attribute as unchanged rather than panicking.
func differenceFromCode(code string) Difference {
	return Difference{
		Content: columnIs(code, 2, 'c'),
		Size:    columnIs(code, 3, 's'),
		// rsync writes an uppercase `T` where it would stamp the destination with the
		// transfer time rather than copy the source's (as under --size-only), and a
		// lowercase `t` where it would copy it. Both mean the timestamps do not
		// currently agree, which is the only thing this field claims.
		ModTime: columnIs(code, 4, 't', 'T'),
	}
}

// columnIs reports whether code's byte at index i is one of want. A code shorter
// than i reports false, so a narrower itemize format loses a column's meaning
// rather than the whole line.
func columnIs(code string, i int, want ...byte) bool {
	if i >= len(code) {
		return false
	}
	for _, w := range want {
		if code[i] == w {
			return true
		}
	}
	return false
}

// hasFilterMeta reports whether s contains a byte rsync's filter-rule matcher
// treats as a glob metacharacter — `*`, `?`, or `[` (the character-class opener).
// A path holding one can't be handed to the deletion filter as a literal yet, so
// such delete candidates are dropped until escaping is implemented.
func hasFilterMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
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
