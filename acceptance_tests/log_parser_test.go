package acceptance_test

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedLog mirrors the records csync writes to a run log. Like ReportedOutput for
// stdout, it is the single point of translation between the log's on-disk format
// and the test assertions: when the format changes, parseLog is the only thing that
// changes with it, rather than a substring match in every step that reads a log.
type ParsedLog struct {
	Started     bool
	Version     string
	Invocation  string
	Source      string
	Destination string
	Commands    []LoggedCommand
	// Classified/Selected are the changes csync recorded detecting and the subset the
	// user chose. Has* distinguishes "recorded an empty list" (0 changes, list present)
	// from "recorded nothing at all" (no such record), and *Count is the leading count
	// the log states, kept apart from the list length so a test can catch the two
	// disagreeing.
	HasClassified   bool
	ClassifiedCount int
	Classified      []LoggedAction
	HasSelected     bool
	SelectedCount   int
	Selected        []LoggedAction
	// Excluded* mirror the record of what csync held out of the comparison: the named
	// gitignored paths (the value #82 adds over a bare count), and whether the .git
	// directory and csync's own .csync.toml were withheld. HasExcluded distinguishes a
	// recorded "nothing" from no record at all.
	HasExcluded        bool
	ExcludedGitignored []string
	ExcludedGitDir     bool
	ExcludedCsyncToml  bool
	// Pruned names the run logs this run deleted to stay under the retention ceiling.
	// HasPruned distinguishes a run that recorded pruning nothing from one that
	// recorded no such line, which is the difference between "there was nothing to
	// delete" and "pruning never happened".
	HasPruned bool
	Pruned    []string
}

// LoggedAction is one classified or selected change the log recorded: a verb
// (create/update/delete) and the path it applies to, the path unquoted so a space or
// quote in a filename is restored rather than read as a separator.
type LoggedAction struct {
	Verb string
	Path string
}

// has reports whether a list of recorded actions contains the given verb and path —
// the action-list equivalent of asking "does the log say csync deleted that file?".
func (p ParsedLog) has(actions []LoggedAction, verb, path string) bool {
	for _, a := range actions {
		if a.Verb == verb && a.Path == path {
			return true
		}
	}
	return false
}

// LoggedCommand is one external command csync recorded running: the program, the
// argument vector as the log preserved it (each element a distinct string, so a
// space inside one survives), its exit code, and how long it took as the raw
// duration text, and what it wrote to stderr when it failed (empty when it did not,
// or when it failed silently).
type LoggedCommand struct {
	Name     string
	Args     []string
	ExitCode int
	Duration string
	Stderr   string
}

// command returns the first recorded command with the given name, and whether one
// was found — the log-side equivalent of asking "did csync run rsync?".
func (p ParsedLog) command(name string) (LoggedCommand, bool) {
	for _, c := range p.Commands {
		if c.Name == name {
			return c, true
		}
	}
	return LoggedCommand{}, false
}

// commands returns every recorded command with the given name, in the order csync
// ran them. It distinguishes repeated uses of one program where command's first-match
// cannot: the dry-run comparison and the real transfer are both rsync, so a completed
// run yields two "rsync" entries and this is what tells them apart by count.
func (p ParsedLog) commands(name string) []LoggedCommand {
	var out []LoggedCommand
	for _, c := range p.Commands {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// The record shapes csync writes, each led by an RFC 3339 timestamp (\S+, no
// spaces): the run header that opens the log, the two operand lines, and an exec
// line per external command. logArgRE pulls the %q-quoted tokens out of an exec
// line's bracketed vector.
var (
	logStartedRE    = regexp.MustCompile(`(?m)^\S+ csync started \(version (.+)\)\s*$`)
	logInvocationRE = regexp.MustCompile(`(?m)^\S+ invocation: (.+?)\s*$`)
	logSourceRE     = regexp.MustCompile(`(?m)^\S+ source: (.+?)\s*$`)
	logDestRE       = regexp.MustCompile(`(?m)^\S+ destination: (.+?)\s*$`)
	logExecRE       = regexp.MustCompile(`(?m)^\S+ exec: (\S+) \[(.*)\] exit=(-?\d+) dur=(\S+)(?: stderr=("(?:[^"\\]|\\.)*"))?\s*$`)
	logArgRE        = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	logClassifiedRE = regexp.MustCompile(`(?m)^\S+ classified: (\d+) \[(.*)\]\s*$`)
	logSelectedRE   = regexp.MustCompile(`(?m)^\S+ selected: (\d+) \[(.*)\]\s*$`)
	logActionRE     = regexp.MustCompile(`(\w+) ("(?:[^"\\]|\\.)*")`)
	logExcludedRE   = regexp.MustCompile(`(?m)^\S+ excluded: (.+?)\s*$`)
	logExclGitRE    = regexp.MustCompile(`\d+ gitignored \[(.*?)\]`)
	logPrunedRE     = regexp.MustCompile(`(?m)^\S+ pruned: (.+?)\s*$`)
	logPrunedListRE = regexp.MustCompile(`^\d+ \[(.*)\]$`)
)

// parseLog translates a run log's contents into a structured ParsedLog. It is the
// single parsing facade the log assertions read through, so a change to the log
// format only has to be absorbed here.
func parseLog(content string) ParsedLog {
	var log ParsedLog

	sm := logStartedRE.FindStringSubmatch(content)
	if sm != nil {
		log.Started = true
		log.Version = sm[1]
	}
	if m := logInvocationRE.FindStringSubmatch(content); m != nil {
		log.Invocation = m[1]
	}
	if m := logSourceRE.FindStringSubmatch(content); m != nil {
		log.Source = m[1]
	}
	if m := logDestRE.FindStringSubmatch(content); m != nil {
		log.Destination = m[1]
	}
	for _, m := range logExecRE.FindAllStringSubmatch(content, -1) {
		code, _ := strconv.Atoi(m[3])
		stderr := ""
		if m[5] != "" {
			stderr, _ = strconv.Unquote(m[5])
		}
		log.Commands = append(log.Commands, LoggedCommand{
			Name:     m[1],
			Args:     parseLogArgs(m[2]),
			ExitCode: code,
			Duration: m[4],
			Stderr:   stderr,
		})
	}
	if m := logClassifiedRE.FindStringSubmatch(content); m != nil {
		log.HasClassified = true
		log.ClassifiedCount, _ = strconv.Atoi(m[1])
		log.Classified = parseLogActions(m[2])
	}
	if m := logSelectedRE.FindStringSubmatch(content); m != nil {
		log.HasSelected = true
		log.SelectedCount, _ = strconv.Atoi(m[1])
		log.Selected = parseLogActions(m[2])
	}
	if m := logExcludedRE.FindStringSubmatch(content); m != nil {
		log.HasExcluded = true
		body := m[1]
		if g := logExclGitRE.FindStringSubmatch(body); g != nil {
			log.ExcludedGitignored = parseLogArgs(g[1])
		}
		log.ExcludedGitDir = strings.Contains(body, "the .git directory")
		log.ExcludedCsyncToml = strings.Contains(body, ".csync.toml")
	}
	if m := logPrunedRE.FindStringSubmatch(content); m != nil {
		log.HasPruned = true
		if g := logPrunedListRE.FindStringSubmatch(m[1]); g != nil {
			log.Pruned = parseLogArgs(g[1])
		}
	}
	return log
}

// parseLogActions turns the bracketed body of a classified/selected record —
// `create "src/new.go"; update "README.md"` — into its actions, pairing each verb with
// its unquoted path. It mirrors parseLogArgs: the path is a %q token, so a boundary the
// log escaped (a space, a semicolon, a quote) is restored rather than read as a
// separator between entries.
func parseLogActions(s string) []LoggedAction {
	var out []LoggedAction
	for _, m := range logActionRE.FindAllStringSubmatch(s, -1) {
		path, err := strconv.Unquote(m[2])
		if err != nil {
			path = m[2]
		}
		out = append(out, LoggedAction{Verb: m[1], Path: path})
	}
	return out
}

// parseLogArgs turns the bracketed, %q-quoted argument vector of an exec line back
// into a slice — unquoting each token, so a boundary the log escaped (a space, an
// embedded quote) is restored rather than read as a separator. This is what lets a
// test assert that a path with a space came back out as one argument.
func parseLogArgs(s string) []string {
	var args []string
	for _, tok := range logArgRE.FindAllString(s, -1) {
		v, err := strconv.Unquote(tok)
		if err != nil {
			v = tok
		}
		args = append(args, v)
	}
	return args
}
