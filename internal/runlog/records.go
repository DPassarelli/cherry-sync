// records.go writes the individual records a run log is made of — the invocation,
// each command run, the operands, and the changes classified, selected and excluded
// — along with the formatting each record's line needs.

package runlog

import (
	"fmt"
	"strings"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// Invocation records the command line as it was actually run — the program name
// and the raw arguments the user gave — as the second line of the log. It is the
// literal invocation, distinct from the resolved source and destination below it:
// under a saved-target push or pull the operands are derived from .csync.toml and
// only this line still shows the verb the user typed. The parts are joined with
// spaces for a reader; the exec records below preserve exact argument boundaries.
func (l *Log) Invocation(name string, args []string) error {
	return l.record("invocation", strings.Join(append([]string{name}, args...), " "))
}

// Record writes one external command csync ran — what it was, its argument vector,
// its exit code, how long it took, and, when it failed, what it said about the
// failure. It satisfies command.Recorder, so the packages that shell out report
// through it without depending on this one. The argument vector is written as
// space-separated quoted tokens, so a path holding a space stays a single argument
// a reader can pick out. A discarding log ignores it.
//
// The diagnostic is kept only for a command that failed: a successful one has
// nothing to explain, and rsync writes ordinary warnings to stderr that would
// otherwise bury the records worth reading. It is written %q-quoted on the same
// line as the rest, because a multi-line record would break the one-line-per-event
// shape every other record and every reader of this log depends on.
func (l *Log) Record(e command.Execution) error {
	rest := fmt.Sprintf("%s %s exit=%d dur=%s", e.Name, quoteArgs(e.Args), e.ExitCode, roundUpMillis(e.Duration))
	if e.ExitCode != 0 && len(e.Stderr) > 0 {
		rest += fmt.Sprintf(" stderr=%q", e.Stderr)
	}
	return l.record("exec", rest)
}

// roundUpMillis renders d as a whole number of milliseconds, rounding up — "44ms", not
// the unrounded "43.764397ms". The fractional tail is noise at this granularity, and
// rounding up rather than to nearest keeps any command that took time at all from
// reading as 0ms: the ceiling of a positive duration is at least one millisecond.
func roundUpMillis(d time.Duration) string {
	ms := (int64(d) + int64(time.Millisecond) - 1) / int64(time.Millisecond)
	return fmt.Sprintf("%dms", ms)
}

// quoteArgs renders an argument vector as a bracketed list of double-quoted tokens
// — ["--recursive" "src dir/"] — so the boundary between arguments survives in the
// log even when an argument contains a space. %q also escapes an embedded quote,
// so no argument can forge a boundary that isn't there.
func quoteArgs(args []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%q", a)
	}
	b.WriteByte(']')
	return b.String()
}

// Operands records the run's source and destination — what csync compared and
// which way the sync went — the frame every later record hangs on. Each is written
// on its own labeled line, so a value containing a space needs no quoting: the whole
// remainder of the line is the operand. They are the same paths csync echoes in its
// header, recorded up front so an abandoned run still says what it was about.
func (l *Log) Operands(source, destination string) error {
	err := l.record("source", source)
	if err != nil {
		return err
	}
	return l.record("destination", destination)
}

// Action is one change the run log records — a verb (create/update/delete) and the
// path it applies to. It is runlog's own type, not compare's, so the log package owns
// what it records and does not depend on the comparison package that produces them;
// the caller adapts across the two.
type Action struct {
	Verb string
	Path string
}

// Classified records the full change list csync detected — every create, update, and
// delete it found, before the user chooses among them. It is written after the
// comparison and before the prompt, so a run abandoned at the selection still shows
// what was on offer.
func (l *Log) Classified(actions []Action) error {
	return l.record("classified", renderActions(actions))
}

// Selected records the subset of the classified changes the user chose to apply. It is
// written once the selection is made — the counterpart to Classified, kept separate
// because the two can differ, and a removal that was selected (and so applied) is the
// fact the log most exists to preserve.
func (l *Log) Selected(actions []Action) error {
	return l.record("selected", renderActions(actions))
}

// Excluded records what csync held out of the comparison: the gitignored paths by name
// (so the log can answer whether a given file was withheld, not merely how many were),
// the .git directory, and csync's own .csync.toml. Each part appears only when it
// applied; a run that withheld nothing records "nothing", so the record is always
// present and its absence never ambiguous.
func (l *Log) Excluded(gitignored []string, gitDir, csyncToml bool) error {
	return l.record("excluded", renderExclusions(gitignored, gitDir, csyncToml))
}

// renderExclusions assembles the exclusion line from the parts that apply, mirroring the
// header's English list but naming the gitignored paths (%q-quoted, so a space or quote
// in a name survives) rather than counting them. The count still leads the gitignored
// part for a quick read. No part applying renders "nothing".
func renderExclusions(gitignored []string, gitDir, csyncToml bool) string {
	var parts []string
	if len(gitignored) > 0 {
		parts = append(parts, fmt.Sprintf("%d gitignored %s", len(gitignored), quoteList(gitignored)))
	}
	if gitDir {
		parts = append(parts, "the .git directory")
	}
	if csyncToml {
		parts = append(parts, ".csync.toml")
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

// quoteList renders a list of strings as "[\"a\"; \"b\"]", each %q-quoted and
// semicolon-separated — the same boundary-proof quoting quoteArgs uses, for a list that
// carries no verb alongside each entry.
func quoteList(items []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, it := range items {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%q", it)
	}
	b.WriteByte(']')
	return b.String()
}

// renderActions renders an action list as "<count> [<verb> \"<path>\"; ...]" — the
// count first for a quick read, then each action with its path %q-quoted so a space or
// quote in a filename survives, semicolon-separated. An empty list renders "0 []". The
// path is quoted for the same reason quoteArgs quotes an argument: a boundary must not
// be forgeable by the contents of a name.
func renderActions(actions []Action) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d [", len(actions))
	for i, a := range actions {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %q", a.Verb, a.Path)
	}
	b.WriteByte(']')
	return b.String()
}

// record appends one line to the log: the current UTC time in RFC 3339, a label
// naming the fact, and the rest of the line. Every record after "started" shares
// this shape, so the label is what a reader (or a later field type) keys on. On a
// discarding log it does nothing, so no caller needs a second code path. The line
// is written with its own syscall, not buffered — the run may be killed before it
// ends, and the records already on disk are the ones worth having.
func (l *Log) record(label, rest string) error {
	if l.file == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := fmt.Fprintf(l.file, "%s %s: %s\n", now, label, rest)
	if err != nil {
		return fmt.Errorf("could not write to the log file %s: %w", l.path, reason(err))
	}
	return nil
}
