// Package cli parses csync's command-line arguments into a validated Args value
// — the source and destination paths, or a saved-target verb — leaving how to
// report a parse error to the caller.
package cli

import "fmt"

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
	if len(argv) >= 1 {
		switch argv[0] {
		case "push", "pull":
			// The verbs take no operands — they resolve from .csync.toml — so a
			// trailing argument is a mistake, not an explicit source/destination
			// pair whose source happens to be named "push". Reject it rather than
			// fall through to the two-operand path below.
			if len(argv) != 1 {
				return Args{}, fmt.Errorf("%s takes no arguments", argv[0])
			}
			if argv[0] == "pull" {
				return Args{Mode: Pull}, nil
			}
			return Args{Mode: Push}, nil
		}
	}
	if len(argv) != 2 {
		return Args{}, fmt.Errorf("expected 2 arguments, got %d", len(argv))
	}
	if argv[0] == "" {
		return Args{}, fmt.Errorf("source path is empty")
	}
	if argv[1] == "" {
		return Args{}, fmt.Errorf("destination path is empty")
	}
	return Args{
		Source:      argv[0],
		Destination: argv[1],
	}, nil
}
