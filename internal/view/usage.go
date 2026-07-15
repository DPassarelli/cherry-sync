// usage.go renders the full help text shown by the --help / -h flag: the same
// version report --version prints, then heroku-style sections (USAGE, EXAMPLES,
// COMMANDS, FLAGS) and a closing note on saved targets. main prints it to stdout
// and exits 0; the parse-error path prints its own one-line reason and a pointer
// here instead of this block.

package view

import (
	"fmt"
	"strings"
)

// helpColumn is one row of an aligned two-column help section: a left-hand term
// (an invocation, command, or flag) and the gloss printed beside it.
type helpColumn struct {
	term  string
	gloss string
}

// usageForms lists the invocation grammars shown under USAGE — the bare
// synopsis, without glosses. EXAMPLES and the reference sections carry the
// explanation, so USAGE stays a terse statement of shape.
var usageForms = []string{
	"csync SOURCE DESTINATION",
	"csync push | pull",
	"csync --version | --license | --help",
}

// helpExamples pairs each concrete invocation shown under EXAMPLES with its
// one-line gloss. Examples lead the reference sections because a lost user reads
// the shape fastest from a worked case.
var helpExamples = []helpColumn{
	{"csync ./site user@host:/srv/site", "push local files up to the server"},
	{"csync user@host:/srv/site ./site", "pull the server's files down"},
	{"csync pull", "update from the saved remote"},
}

// helpCommands lists the saved-target verbs, each with a one-line gloss, for the
// COMMANDS section.
var helpCommands = []helpColumn{
	{"push", "sync the current project TO the remote saved in .csync.toml"},
	{"pull", "sync the current project FROM the remote saved in .csync.toml"},
}

// helpFlags lists every flag csync accepts, each with a one-line gloss, for the
// FLAGS section.
var helpFlags = []helpColumn{
	{"--version", "print version information and exit"},
	{"--license", "print the license text and exit"},
	{"-h, --help", "show this help and exit"},
}

// Usage returns csync's full help text: the --version report as a header, then
// the USAGE, EXAMPLES, COMMANDS, and FLAGS sections, and a closing note on saved
// targets. raw is the build version (main.version); the report header is reused
// verbatim (VersionReport) so --help and --version cannot drift. main prints it
// to stdout for --help.
func Usage(raw string) string {
	var b strings.Builder
	b.WriteString(VersionReport(raw))
	b.WriteString("\n\n")

	b.WriteString("USAGE\n")
	for _, form := range usageForms {
		fmt.Fprintf(&b, "  %s\n", form)
	}

	b.WriteString("\nEXAMPLES\n")
	b.WriteString(renderHelpColumns(helpExamples))

	b.WriteString("\nCOMMANDS\n")
	b.WriteString(renderHelpColumns(helpCommands))

	b.WriteString("\nFLAGS\n")
	b.WriteString(renderHelpColumns(helpFlags))

	// Saved targets read their remote from a file the help can't show inline, so
	// close by pointing at the README section that documents the format.
	fmt.Fprintf(&b, "\nSaved targets (push, pull) read the remote from a .csync.toml in the current\ndirectory. See %s#saved-targets", versionURL)
	return b.String()
}

// renderHelpColumns formats a two-column section: each term left-justified to
// the width of the longest, then three spaces, then its gloss, the whole block
// indented two spaces under its heading.
func renderHelpColumns(rows []helpColumn) string {
	width := 0
	for _, r := range rows {
		if len(r.term) > width {
			width = len(r.term)
		}
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-*s   %s\n", width, r.term, r.gloss)
	}
	return b.String()
}
