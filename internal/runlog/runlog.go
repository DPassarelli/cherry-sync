// Package runlog records what a csync run did, to a file on the local machine.
// The record is written as the run proceeds and without being asked for: once a
// run has removed a file from the destination it cannot be repeated to find out
// what it touched, so anything worth knowing afterwards has to be captured before
// anyone knows it will be needed.
package runlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dpassarelli/cherry-sync/internal/command"
)

// Log is an open run log: the record of a single csync invocation. Its path is
// disclosed to the user, since a record nobody can find is not a record. The file
// stays open for the length of the run and every record is written to it as the
// run reaches it — see Create.
//
// The zero Log records nothing and has no path; see Discard.
type Log struct {
	path string
	file *os.File
}

// Discard returns a run log that keeps no record and has no path to disclose. It
// stands in when a log cannot be opened, so a csync that could not write one is
// nonetheless a csync that syncs: the record is a diagnostic, never a precondition.
// Callers need no second code path, and Path returns "" so nothing invites the user
// to go looking for a file that was never created.
func Discard() *Log {
	return &Log{}
}

// Create opens a run log for this invocation, records that the run started and
// which build made it, and returns the open log. version is the first thing a
// troubleshooter needs and the field a bug report most often omits, so it heads the
// log rather than trailing on its own line; it is written verbatim (a dev build
// logs "dev"). The file is named for the moment the run started and the process
// that made it, so concurrent runs cannot collide and a reader can order runs
// without opening them. It is created exclusively: a name that already exists is an
// error rather than a silent overwrite of another run's record.
//
// The started record is written before Create returns, and every later record as
// the run reaches it. Nothing is held back to be flushed on the way out: csync can
// be interrupted at the selection prompt or killed outright, and those are the runs
// a reader most wants. A log assembled in memory would be empty in exactly the
// cases it exists for.
func Create(version string) (*Log, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	// 0700: the log names every path a run touched, which discloses the shape of
	// the user's work tree. Nothing outside the account has cause to read it.
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("could not create the log directory %s: %w", dir, reason(err))
	}
	started := time.Now().UTC()
	name := fmt.Sprintf("run-%s-%d.log", started.Format("20060102T150405Z"), os.Getpid())
	path := filepath.Join(dir, name)
	// #nosec G304 -- path is stateDir() (a trusted base) joined with a
	// program-generated "run-<timestamp>-<pid>.log" name; no external input reaches
	// the filename, so there is no path-traversal surface here.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("could not create the log file %s: %w", path, reason(err))
	}
	l := &Log{path: path, file: f}
	_, err = fmt.Fprintf(f, "%s csync started (version %s)\n", started.Format(time.RFC3339), version)
	if err != nil {
		// The write, not the close, is the failure worth reporting; close what we
		// opened and surface the original error.
		_ = f.Close()
		return nil, fmt.Errorf("could not write to the log file %s: %w", path, reason(err))
	}
	// Pruning happens after this run's log exists, so the file being written is the
	// newest in the directory and never a candidate for its own removal — which is
	// what lets the ceiling be stated as "keep the newest maxLogs" with no special
	// case. A prune that fails does not fail the run: the record of what a sync did
	// is worth more than a tidy directory, and the failure leaves logs behind rather
	// than losing any.
	pruned, _ := prune(dir, maxLogs)
	err = l.record("pruned", renderPruned(pruned))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return l, nil
}

// maxLogs is how many run logs csync keeps; a run that would push the directory past
// it deletes the oldest until it fits. The ceiling exists so the directory stays
// short enough to read through, not to reclaim space — a log runs to a couple of
// kilobytes — which is why the number is fixed here rather than made configurable.
const maxLogs = 25

// runLogNameRE matches the names Create gives run logs, and nothing else. Pruning
// deletes files, and the log directory belongs to the user rather than to csync: a
// file found there that csync did not write is somebody else's and is left alone,
// however old it is. Matching the whole name — not a "run-" prefix or a ".log"
// suffix — is what keeps that promise.
var runLogNameRE = regexp.MustCompile(`^run-\d{8}T\d{6}Z-\d+\.log$`)

// prune deletes the oldest run logs in dir until at most keep remain, returning the
// names it removed, oldest first. Age comes from the timestamp in each name rather
// than the file's modification time: the name records when the run actually started,
// while mtime is rewritten by any backup or copy that touches the file. The names
// lead with that timestamp at a fixed width, so sorting them lexically sorts them
// chronologically.
//
// A log it cannot delete is collected into the returned error but does not stop the
// rest — one stubborn file should not leave the whole directory unpruned.
func prune(dir string, keep int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not read the log directory %s: %w", dir, reason(err))
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && runLogNameRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return nil, nil
	}
	sort.Strings(names)
	var removed []string
	var failures []error
	for _, name := range names[:len(names)-keep] {
		err = os.Remove(filepath.Join(dir, name))
		if err != nil {
			failures = append(failures, err)
			continue
		}
		removed = append(removed, name)
	}
	return removed, errors.Join(failures...)
}

// renderPruned renders the pruned record: the count first for a quick read, then the
// names removed, %q-quoted like every other list the log writes. A run that removed
// nothing records "nothing", so the record is always present and its absence never
// ambiguous.
func renderPruned(names []string) string {
	if len(names) == 0 {
		return "nothing"
	}
	return fmt.Sprintf("%d %s", len(names), quoteList(names))
}

// reason strips a filesystem error down to the part a person still needs. The os
// package returns *fs.PathError, whose message repeats the syscall and the path
// ("mkdir /home/x/.local/state: not a directory"). Every caller here has already
// named the operation in plain words and the path in full, so all that is left to
// add is why it failed. Any other error is returned unchanged. The result is still
// the wrapped errno, so errors.Is keeps working on what Create returns.
func reason(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

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

// Path reports where this run's log was written, so the CLI can disclose it.
func (l *Log) Path() string {
	return l.path
}

// Close releases the log file. The records are already on disk — each is written
// with its own syscall rather than buffered — so closing preserves nothing and
// only returns the descriptor. Closing a discarding log does nothing.
func (l *Log) Close() error {
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}

// stateDir returns the directory csync keeps its run logs in: the XDG state
// directory, which holds data that persists between runs but which the user
// would not miss if it were deleted. It is never the project directory — csync
// withholds only .csync.toml and .git from a comparison, so a log written beside
// the source would be offered for transfer and pushed to the remote.
func stateDir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not locate your home directory, and XDG_STATE_HOME is unset: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "cherry-sync"), nil
}
