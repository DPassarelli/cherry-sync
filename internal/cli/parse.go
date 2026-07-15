// Package cli parses csync's command-line arguments into a validated Args value
// — the source and destination paths, or a saved-target verb — leaving how to
// report a parse error to the caller.
package cli

import (
	"fmt"
	"slices"
	"strings"
)

// Mode is how csync resolves its source and destination operands.
type Mode int

const (
	// Explicit takes the source and destination from the command line. It is the
	// zero value, so a two-operand invocation needs no mode set.
	Explicit Mode = iota
	// Push syncs the current project to the remote saved in .csync.toml.
	Push
	// Pull syncs the remote saved in .csync.toml down to the current project.
	Pull
	// Version reports csync's version and exits; no operands are resolved.
	Version
	// License prints csync's license text and exits; no operands are resolved.
	License
	// Help prints csync's usage summary and exits; no operands are resolved.
	Help
)

// Args is the parsed result of a csync command-line invocation. In Explicit mode
// it carries the source and destination paths from the command line; in Push or
// Pull mode the operands are left empty for the caller to resolve from the
// project's saved-target file.
type Args struct {
	Source      string
	Destination string
	Mode        Mode
}

// Parse turns the positional arguments (typically os.Args[1:]) into an Args
// value. It accepts either two positional operands in source/destination order,
// or a single saved-target verb (`push` or `pull`, which resolve operands from
// .csync.toml); anything else returns an error. The caller (main.go) decides how
// to surface that error to the user.
func Parse(argv []string) (Args, error) {
	// --help (and its "-h" alias) short-circuits ahead of everything: it is the
	// escape hatch a lost user reaches for, so it wins over operands and over the
	// other informational flags. `tool --help <anything>` printing usage and
	// exiting is the near-universal convention.
	if slices.Contains(argv, "--help") || slices.Contains(argv, "-h") {
		return Args{Mode: Help}, nil
	}
	// --version short-circuits: it wins over any operands (matching the near-
	// universal CLI convention that `tool --version <anything>` still reports the
	// version and exits) rather than tripping the two-operand check below.
	if slices.Contains(argv, "--version") {
		return Args{Mode: Version}, nil
	}
	// --license short-circuits the same way --version does, for the same reason:
	// an informational flag reports and exits regardless of trailing operands.
	// --version is checked first, so it wins if both are present.
	if slices.Contains(argv, "--license") {
		return Args{Mode: License}, nil
	}
	if len(argv) >= 1 {
		switch argv[0] {
		case "push", "pull":
			// The verbs take no operands — they resolve from .csync.toml — so a
			// trailing argument is a mistake, not an explicit source/destination
			// pair whose source happens to be named "push". Reject it rather than
			// fall through to the two-operand path below.
			if len(argv) != 1 {
				return Args{}, fmt.Errorf("'%s' takes no arguments", argv[0])
			}
			if argv[0] == "pull" {
				return Args{Mode: Pull}, nil
			}
			return Args{Mode: Push}, nil
		}
	}
	if len(argv) != 2 {
		// A lone argument that can't be a path is far more likely a mistyped verb
		// than half of an operand pair, so name it as such and point at the real
		// commands — a clearer nudge than "wrong argument count". Anything
		// path-shaped, or the wrong count outright, falls to the operand message.
		if len(argv) == 1 && !looksLikePath(argv[0]) {
			return Args{}, fmt.Errorf("'%s' is not a command. Did you mean 'push' or 'pull'?", argv[0])
		}
		return Args{}, fmt.Errorf("csync needs two paths: a source and a destination")
	}
	if argv[0] == "" {
		return Args{}, fmt.Errorf("the source path is empty")
	}
	if argv[1] == "" {
		return Args{}, fmt.Errorf("the destination path is empty")
	}
	return Args{
		Source:      argv[0],
		Destination: argv[1],
	}, nil
}

// looksLikePath reports whether a lone argument is shaped like a filesystem or
// remote path rather than a mistyped subcommand. A path carries a marker a bare
// command word never would: a "/" component, a "host:" remote colon, or a
// leading "." or "~". This only picks which error message a single bad argument
// gets — a lone operand is an error either way — so a false guess costs wording,
// not correctness.
func looksLikePath(s string) bool {
	if strings.ContainsAny(s, "/:") {
		return true
	}
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "~")
}
