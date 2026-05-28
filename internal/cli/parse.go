package cli

import "fmt"

// Args is the parsed result of a csync command-line invocation: the source
// and destination paths the user wants to sync between.
type Args struct {
	Source      string
	Destination string
}

// Parse turns the positional arguments (typically os.Args[1:]) into an Args
// value. v0.1 accepts exactly two positional arguments in source/destination
// order; anything else returns an error. The caller (main.go) decides how to
// surface that error to the user.
func Parse(argv []string) (Args, error) {
	if len(argv) != 2 {
		return Args{}, fmt.Errorf("expected 2 arguments, got %d", len(argv))
	}
	return Args{
		Source:      argv[0],
		Destination: argv[1],
	}, nil
}
