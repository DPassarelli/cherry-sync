// parse.go translates the per-file records of rsync's --out-format output into the
// package's Action values — the core code-to-label step, kept width-agnostic so it
// reads both GNU rsync's and openrsync's itemize formats.

package compare

import (
	"strconv"
	"strings"
	"time"
)

// recordFields is how many `|`-separated columns a per-file record carries under
// the --out-format compare.rsyncArgs asks for: the itemize code, the size, the
// modification time, and the path. It is also the split limit, which is what keeps
// a `|` inside a filename part of the path rather than the start of a fifth column.
const recordFields = 4

// modTimeLayout is rsync's %M rendering, e.g. "2026/09/03-17:32:53". Both GNU
// rsync and openrsync format it with this same strftime pattern, and both render
// it in local time, so it is parsed in the local zone.
const modTimeLayout = "2006/01/02-15:04:05"

// actionKey identifies a planned change for the purpose of collapsing duplicates:
// the verb and the path, and deliberately not the metadata. A path cannot take the
// same verb twice, so two records sharing both describe one change however their
// size or timestamp columns read; keying on the whole Action would let a byte of
// drift between two lines defeat the collapse.
type actionKey struct {
	verb string
	path string
}

// parseActions walks rsync's per-file output and returns one Action per change
// record, in the order first seen. Non-record lines (the -vv chatter, the preamble,
// the summary) are skipped naturally because they carry no `|`-delimited record.
// Identical (verb, path) actions are collapsed to one: openrsync's dry-run prints a
// `*deleting` line for the same path twice where GNU rsync prints it once, so a
// repeat is always noise, never a second distinct change.
func parseActions(rsyncOut string) []Action {
	var actions []Action
	seen := make(map[actionKey]bool)
	for line := range strings.SplitSeq(rsyncOut, "\n") {
		action, ok := actionFromLine(line)
		if !ok {
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

// actionFromLine translates one --out-format record into an Action, reporting
// false for any line that is not one. A regular-file transfer in either direction —
// `>f...` (received into the destination: pull or local-to-local) and `<f...` (sent
// to a remote: a real push over SSH) — is an update, with an all-`+` attribute run
// (a brand-new file) a create. The leading byte is the transfer direction, not a
// distinct change, so both map to the same verb.
//
// The record is "<code>|<size>|<mtime>|<path>", split at most recordFields ways so
// the path keeps any `|` of its own. The code's width is implementation-specific —
// GNU rsync emits 11 chars, macOS's openrsync 9 — so nothing here indexes from the
// right; the delimiter, not a fixed offset, is what ends the code.
func actionFromLine(line string) (Action, bool) {
	fields := strings.SplitN(line, "|", recordFields)
	if len(fields) < recordFields {
		return Action{}, false
	}
	// GNU pads `*deleting` out to its 11-char code column, so the code field can
	// carry trailing spaces; openrsync's 9-char column holds it flush.
	code := strings.TrimRight(fields[0], " ")
	path := fields[recordFields-1]
	if path == "" {
		return Action{}, false
	}

	// A removal itemizes as `*deleting`, a word rather than a flag run. Its size and
	// time columns describe nothing — rsync reports 0 and the epoch, since the file
	// is absent from the side being read — so they are not carried onto the Action,
	// where they would render as a real measurement.
	if code == "*deleting" {
		// The transfer applies a deletion through an rsync filter rule, where `*`,
		// `?`, and `[` are glob metacharacters — an unescaped one could match and
		// remove the wrong file. Escaping them safely is deferred, so a delete
		// candidate carrying one is dropped here: it is never reported, so it can't
		// be selected or applied. Only deletion is affected — the same name still
		// transfers on create/update, whose path list is literal (--files-from).
		if hasFilterMeta(path) {
			return Action{}, false
		}
		return Action{Verb: "delete", Path: path}, true
	}

	if len(code) < 2 || (code[0] != '<' && code[0] != '>') || code[1] != 'f' {
		return Action{}, false
	}

	// Metadata is decoded best-effort: an unreadable size or timestamp costs the row
	// its annotation, never the change itself, so a future rsync that formats either
	// field differently degrades to a bare verb rather than making files unsyncable.
	action := Action{Path: path, Size: parseSize(fields[1]), ModTime: parseModTime(fields[2])}

	// A newly created file marks every attribute column '+', however many this rsync
	// emits (9 under GNU, 7 under openrsync). Those markers say the file is new, not
	// that a given attribute differs, so a create carries no Difference: there is no
	// counterpart for it to differ from.
	if isAllPlus(code[2:]) {
		action.Verb = "create"
		return action, true
	}
	action.Verb = "update"
	action.Diff = differenceFromCode(code)
	return action, true
}

// differenceFromCode reads the attribute columns that say what gave a file away.
// The columns are indexed from the left — checksum, size, then time, immediately
// after the two-byte type prefix — which is where both GNU rsync and openrsync
// write them despite emitting different numbers of columns overall. A code too
// short to hold one reports that attribute as unchanged rather than panicking.
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
// rather than the whole record.
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

// parseSize reads the record's length column, returning 0 for anything it cannot
// read — see actionFromLine on why unreadable metadata is not an error. A negative
// length is treated the same way: no file has one, so it is corruption rather than
// a measurement.
func parseSize(field string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// parseModTime reads the record's modification-time column, returning the zero
// Time for anything it cannot read — see actionFromLine on why unreadable metadata
// is not an error.
func parseModTime(field string) time.Time {
	t, err := time.ParseInLocation(modTimeLayout, strings.TrimSpace(field), time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
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
