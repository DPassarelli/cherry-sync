// Package runlog records what a csync run did, to a file on the local machine.
// The record is written as the run proceeds and without being asked for: once a
// run has removed a file from the destination it cannot be repeated to find out
// what it touched, so anything worth knowing afterwards has to be captured before
// anyone knows it will be needed.
package runlog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Log is an open run log: the record of a single csync invocation. Its path is
// disclosed to the user, since a record nobody can find is not a record.
type Log struct {
	path string
}

// Create opens a run log for this invocation and returns it. The file is named
// for the moment the run started and the process that made it, so concurrent
// runs cannot collide and a reader can order runs without opening them. It is
// created exclusively: a name that already exists is an error rather than a
// silent overwrite of another run's record.
func Create() (*Log, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	// 0700: the log names every path a run touched, which discloses the shape of
	// the user's work tree. Nothing outside the account has cause to read it.
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("run log directory: %w", err)
	}
	name := fmt.Sprintf("run-%s-%d.log", time.Now().UTC().Format("20060102T150405Z"), os.Getpid())
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	err = f.Close()
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	return &Log{path: path}, nil
}

// Path reports where this run's log was written, so the CLI can disclose it.
func (l *Log) Path() string {
	return l.path
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
			return "", fmt.Errorf("run log directory: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "cherry-sync"), nil
}
