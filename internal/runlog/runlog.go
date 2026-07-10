// Package runlog records what a csync run did, to a file on the local machine.
// The record is written as the run proceeds and without being asked for: once a
// run has removed a file from the destination it cannot be repeated to find out
// what it touched, so anything worth knowing afterwards has to be captured before
// anyone knows it will be needed.
package runlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Log is an open run log: the record of a single csync invocation. Its path is
// disclosed to the user, since a record nobody can find is not a record. The file
// stays open for the length of the run and every record is written to it as the
// run reaches it — see Create.
//
// The zero Log records nothing and has no path; see Discard.
type Log struct {
	path string
	file *os.File
}

// Discard returns a run log that keeps no record and has no path to disclose. It
// stands in when a log cannot be opened, so a csync that could not write one is
// nonetheless a csync that syncs: the record is a diagnostic, never a precondition.
// Callers need no second code path, and Path returns "" so nothing invites the user
// to go looking for a file that was never created.
func Discard() *Log {
	return &Log{}
}

// Create opens a run log for this invocation, records that the run started, and
// returns the open log. The file is named for the moment the run started and the
// process that made it, so concurrent runs cannot collide and a reader can order
// runs without opening them. It is created exclusively: a name that already exists
// is an error rather than a silent overwrite of another run's record.
//
// The started record is written before Create returns, and every later record as
// the run reaches it. Nothing is held back to be flushed on the way out: csync can
// be interrupted at the selection prompt or killed outright, and those are the runs
// a reader most wants. A log assembled in memory would be empty in exactly the
// cases it exists for.
func Create() (*Log, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	// 0700: the log names every path a run touched, which discloses the shape of
	// the user's work tree. Nothing outside the account has cause to read it.
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("could not create the log directory %s: %w", dir, reason(err))
	}
	started := time.Now().UTC()
	name := fmt.Sprintf("run-%s-%d.log", started.Format("20060102T150405Z"), os.Getpid())
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("could not create the log file %s: %w", path, reason(err))
	}
	l := &Log{path: path, file: f}
	_, err = fmt.Fprintf(f, "%s started\n", started.Format(time.RFC3339))
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("could not write to the log file %s: %w", path, reason(err))
	}
	return l, nil
}

// reason strips a filesystem error down to the part a person still needs. The os
// package returns *fs.PathError, whose message repeats the syscall and the path
// ("mkdir /home/x/.local/state: not a directory"). Every caller here has already
// named the operation in plain words and the path in full, so all that is left to
// add is why it failed. Any other error is returned unchanged. The result is still
// the wrapped errno, so errors.Is keeps working on what Create returns.
func reason(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// Path reports where this run's log was written, so the CLI can disclose it.
func (l *Log) Path() string {
	return l.path
}

// Close releases the log file. The records are already on disk — each is written
// with its own syscall rather than buffered — so closing preserves nothing and
// only returns the descriptor. Closing a discarding log does nothing.
func (l *Log) Close() error {
	if l.file == nil {
		return nil
	}
	return l.file.Close()
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
			return "", fmt.Errorf("could not locate your home directory, and XDG_STATE_HOME is unset: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "cherry-sync"), nil
}
