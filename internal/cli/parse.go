package cli

// Args is the parsed result of a csync command-line invocation: the source
// and destination paths the user wants to sync between.
type Args struct {
	Source      string
	Destination string
}

// Parse turns the positional arguments (typically os.Args[1:]) into an Args
// value. v0.1 accepts exactly two positional arguments in source/destination
// order; richer parsing (flags, validation, usage errors) will be layered on
// later as the corresponding scenarios in features/invoke-command.feature
// move out of the TODO block.
func Parse(argv []string) Args {
	return Args{
		Source:      argv[0],
		Destination: argv[1],
	}
}
