package acceptance_test

import (
	"regexp"
	"strconv"
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
}

// LoggedCommand is one external command csync recorded running: the program, the
// argument vector as the log preserved it (each element a distinct string, so a
// space inside one survives), its exit code, and how long it took as the raw
// duration text.
type LoggedCommand struct {
	Name     string
	Args     []string
	ExitCode int
	Duration string
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
	logExecRE       = regexp.MustCompile(`(?m)^\S+ exec: (\S+) \[(.*)\] exit=(-?\d+) dur=(\S+)\s*$`)
	logArgRE        = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
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
		log.Commands = append(log.Commands, LoggedCommand{
			Name:     m[1],
			Args:     parseLogArgs(m[2]),
			ExitCode: code,
			Duration: m[4],
		})
	}
	return log
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
