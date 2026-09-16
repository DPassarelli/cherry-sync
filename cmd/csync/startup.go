// startup.go covers everything settled before any work happens: the flags that
// ask csync about itself rather than about a directory, and working out which two
// paths the run moves files between.

package main

import (
	"fmt"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/config"
	"github.com/dpassarelli/cherry-sync/internal/license"
	"github.com/dpassarelli/cherry-sync/internal/operand"
	"github.com/dpassarelli/cherry-sync/internal/view"
)

// infoMode answers the flags that ask csync about itself rather than about any
// directory — --help, --version, --license — reporting the status to exit with and
// whether it handled the run at all. All three short-circuit in cli.Parse, so any
// trailing operands are ignored, and all three print to stdout: this is requested
// output, not a diagnostic.
func infoMode(a cli.Args) (int, bool) {
	switch a.Mode {
	case cli.Help:
		fmt.Println(view.Usage(version))
	case cli.Version:
		fmt.Println(view.VersionReport(version))
	case cli.License:
		// The embedded text ends in a newline, so Print — not Println — avoids a
		// trailing blank line. A distributed bare binary carries its own notice this
		// way, with nothing required alongside it.
		fmt.Print(license.Text())
	default:
		return 0, false
	}
	return 0, true
}

// operands holds the two paths a run moves files between, in the single shape the
// header echo, the comparison and the transfer all read. Each From carries the
// original path portion when normalization rewrote it, so the rewrite can be
// disclosed rather than applied silently.
type operands struct {
	Source          string
	SourceFrom      string
	Destination     string
	DestinationFrom string
}

// resolveOperands works out which two paths this run works on. With a saved-target
// verb they aren't on the command line at all: they come from the project's
// .csync.toml in the current directory, where push sends the project (".") to the
// saved remote and pull brings the saved remote down to it — the direction being
// the only difference, which side is source.
//
// Both are normalized before anything reads them. The load-bearing case is a remote
// path with a leading "~": modern rsync passes it literally, so `host:~/x` resolves
// to `/home/user/~/x` and the transfer fails with exit 12 (#50). Normalize turns it
// into a relative path (rsync interprets that against the login home) and reports
// the rewrite. A `~user` form has no relative equivalent, so this errors rather than
// letting rsync fail confusingly mid-transfer.
func resolveOperands(a cli.Args) (operands, error) {
	source, destination := a.Source, a.Destination
	if a.Mode != cli.Explicit {
		cfg, err := config.Load(".")
		if err != nil {
			return operands{}, err
		}
		switch a.Mode {
		case cli.Push:
			source, destination = ".", cfg.Remote
		case cli.Pull:
			source, destination = cfg.Remote, "."
		}
	}

	srcN, err := operand.Normalize(source)
	if err != nil {
		return operands{}, err
	}
	dstN, err := operand.Normalize(destination)
	if err != nil {
		return operands{}, err
	}
	return operands{
		Source:          srcN.Path,
		SourceFrom:      srcN.From,
		Destination:     dstN.Path,
		DestinationFrom: dstN.From,
	}, nil
}
