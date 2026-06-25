// Package cli parses csync's command-line arguments into a validated Args value
// — for v0.1, the source and destination paths — leaving how to report a parse
// error to the caller.
package cli

import "fmt"

// Args is the parsed result of a csync command-line invocation. With two
// positional operands it carries the source and destination paths; with the
// `push` verb it sets Push, signalling the caller to resolve the operands from
// the project's saved-target file instead.
type Args struct {
	Source      string
	Destination string
	Push        bool
}

// Parse turns the positional arguments (typically os.Args[1:]) into an Args
// value. It accepts either two positional operands in source/destination order,
// or the single verb `push` (resolve operands from .csync.toml); anything else
// returns an error. The caller (main.go) decides how to surface that error to
// the user.
func Parse(argv []string) (Args, error) {
	if len(argv) == 1 && argv[0] == "push" {
		return Args{Push: true}, nil
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
