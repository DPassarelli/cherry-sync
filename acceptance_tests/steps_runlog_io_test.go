// Steps covering the run log as a file rather than as content: where it is
// written, what csync says about it, its permissions, and the degraded path
// where it cannot be written at all.

package acceptance_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// csyncShouldReportWhereItLoggedTheRun asserts csync disclosed the path of the
// run log it wrote. A record nobody can find is not a record, so the disclosure is a
// behavior in its own right — which is why the steps that read a log's contents find
// it by scanning the scenario's state home instead: were they to read through the
// disclosure, breaking it would redden every one of them alongside this.
func csyncShouldReportWhereItLoggedTheRun(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if !out.HasLogPath {
		return fmt.Errorf("csync reported no log path; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	if out.LogPath == "" {
		return fmt.Errorf("csync printed an empty log path; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	return nil
}

// aRunLogShouldExistAtTheReportedPath asserts the path csync disclosed names a
// regular file that is really there. Pairing the two steps is what keeps csync
// honest: reporting a path it never wrote to fails here, and writing a log it
// never mentions fails the step above.
func aRunLogShouldExistAtTheReportedPath(ctx context.Context) error {
	r := captured(ctx)
	path := parseOutput(r.Stdout, r.Stderr).LogPath
	if path == "" {
		return fmt.Errorf("csync reported no log path, so there is none to look for; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("run log reported at %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("run log reported at %q is not a regular file (mode %s)", path, info.Mode())
	}
	return nil
}

// thatAFileHasBeenChangedLocally arranges the one condition the run-log scenarios
// need from a comparison: something to sync, so csync stops at the prompt. Which
// file, and how many, is not what those scenarios are about — they name neither —
// so this composes the existing steps and picks a file from the Background itself.
func thatAFileHasBeenChangedLocally(ctx context.Context) (context.Context, error) {
	ctx, err := allFilesIdenticalBetweenLocalAndRemote(ctx)
	if err != nil {
		return ctx, err
	}
	const changed = "README.md"
	ctx, err = theFileHasBeenChangedLocally(ctx, changed)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, changedFileKey{}, changed), nil
}

// thatCsyncCannotWriteItsLog puts a regular file where csync expects to create its
// state directory, so every attempt to make one fails with ENOTDIR.
//
// A file rather than a chmod-ed directory: root ignores permission bits, so a
// read-only directory would quietly stop testing anything the day this suite runs
// as root (in a container, say). Nothing gets to mkdir inside a regular file.
func thatCsyncCannotWriteItsLog(ctx context.Context) error {
	root := stateHome(ctx)
	if root == "" {
		return fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	err := os.WriteFile(root, []byte("not a directory\n"), 0o600)
	if err != nil {
		return fmt.Errorf("blocking the state directory at %s: %w", root, err)
	}
	return nil
}

// theChangedFileShouldBeIdenticalBetweenLocalAndRemote asserts the sync moved the
// file the parameterless setup step changed — the whole point of a run that logged
// nothing still being a run that worked.
func theChangedFileShouldBeIdenticalBetweenLocalAndRemote(ctx context.Context) error {
	changed, _ := ctx.Value(changedFileKey{}).(string)
	if changed == "" {
		return fmt.Errorf("no file was changed; missing a step that changes one?")
	}
	return theFileShouldBeIdenticalBetweenLocalAndRemote(ctx, changed)
}

// csyncShouldWarnThatItCouldNotWriteARunLog asserts csync said out loud that this
// run went unlogged. Declining silently would be the worst of the three outcomes:
// the user goes looking for the record of a destructive run, finds nothing, and
// cannot tell whether csync failed to write it or they misremembered where it goes.
func csyncShouldWarnThatItCouldNotWriteARunLog(ctx context.Context) error {
	r := captured(ctx)
	warning := parseOutput(r.Stdout, r.Stderr).Warning
	if warning == "" {
		return fmt.Errorf("csync issued no warning; stderr:\n%s", r.Stderr)
	}
	if !strings.Contains(warning, "log") {
		return fmt.Errorf("csync warned about something other than the run log: %q", warning)
	}
	return nil
}

// csyncShouldNotReportWhereItLoggedTheRun asserts csync named no log path. It is
// the counterpart of csyncShouldReportWhereItLoggedTheRun: a path is disclosed when
// there is a file at the end of it, and withheld when there is not. Sending the
// user to a log that was never created is a worse answer than admitting there is
// none.
// It reads the presence of the disclosure line, not the truth of its value: csync
// printing a bare "Log written to" with nothing after it is a disclosure, and a useless one.
func csyncShouldNotReportWhereItLoggedTheRun(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if out.HasLogPath {
		return fmt.Errorf("csync reported a log path %q, but it wrote no log", out.LogPath)
	}
	return nil
}

// csyncShouldSayLastOfAllThatTheRunWasNotLogged asserts the final thing csync
// prints is that it kept no record, and why.
//
// Last of all, not merely somewhere: the warning csync gives before the prompt can
// scroll away behind a long change list, and the whole point of repeating it is
// that the notice survives to where the user is still looking when the run ends. A
// step that only checked the notice was present would pass against the warning
// alone and prove nothing.
func csyncShouldSayLastOfAllThatTheRunWasNotLogged(ctx context.Context) error {
	r := captured(ctx)
	reason := parseOutput(r.Stdout, r.Stderr).NotLogged
	if reason == "" {
		return fmt.Errorf("csync gave no reason for keeping no record; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "Not logged:") {
		return fmt.Errorf("csync's last word was %q, not that the run went unlogged", last)
	}
	return nil
}

// noRunLogShouldHaveBeenWritten asserts nothing was recorded under the scenario's
// state home: no log file anywhere beneath it, the state directory not existing at
// all counting as the same thing. It is the counterpart of iLocateTheLogFile —
// where that insists on exactly one log, this insists on none — and it is how a run
// that never reached rsync proves it left the state directory untouched.
func noRunLogShouldHaveBeenWritten(ctx context.Context) error {
	root := stateHome(ctx)
	if root == "" {
		return fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	logs, err := logsUnder(root)
	if err != nil {
		return fmt.Errorf("searching %s for run logs: %w", root, err)
	}
	if len(logs) != 0 {
		return fmt.Errorf("expected no run log under %s, but found %d: %v", root, len(logs), logs)
	}
	return nil
}

// xdgStateHomeIsSet documents the precondition that holds by default: the Before
// hook points the csync child's XDG_STATE_HOME into the scenario's throwaway home
// (see csyncEnv), so this step changes nothing. It is here so the location scenario
// names the condition it depends on, symmetric with its `... is not set` sibling.
func xdgStateHomeIsSet(ctx context.Context) error {
	return nil
}

// xdgStateHomeIsNotSet arranges for the csync child to run with no XDG_STATE_HOME,
// so its log falls back to ~/.local/state under the throwaway home. csyncEnv reads
// the flag and omits the variable (and filters any ambient one), which is the only
// way the fallback path is reached.
func xdgStateHomeIsNotSet(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, noXdgKey{}, true), nil
}

// theRunLogShouldBeUnderIn asserts the run log sits in the named subdirectory of a
// state-directory base — the one place in the suite that pins the layout. base is
// the symbol the scenario used, resolved to the throwaway home so the test names no
// real path: "$XDG_STATE_HOME" is where the harness points the variable, and
// "~/.local/state" is the spec's fallback under the child's HOME.
func theRunLogShouldBeUnderIn(ctx context.Context, sub, base string) error {
	var root string
	switch base {
	case "$XDG_STATE_HOME":
		root = stateHome(ctx)
	case "~/.local/state":
		home, _ := ctx.Value(homeKey{}).(string)
		if home == "" {
			return fmt.Errorf("no scenario home; the Before hook did not run?")
		}
		root = filepath.Join(home, ".local", "state")
	default:
		return fmt.Errorf("unknown state-directory base %q in scenario", base)
	}
	log, err := singleLogUnder(root)
	if err != nil {
		return err
	}
	wantDir := filepath.Join(root, sub)
	gotDir := filepath.Dir(log)
	if gotDir != wantDir {
		return fmt.Errorf("run log is in %s, want it in %s", gotDir, wantDir)
	}
	return nil
}

// theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner asserts the run log's
// directory is 0700 — the shape of a work tree is nobody else's business.
func theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner(ctx context.Context) error {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return err
	}
	return assertPerm(filepath.Dir(log), 0o700)
}

// theRunLogFileShouldBeAccessibleOnlyByItsOwner asserts the run log file itself is
// 0600, for the same reason its directory is 0700.
func theRunLogFileShouldBeAccessibleOnlyByItsOwner(ctx context.Context) error {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return err
	}
	return assertPerm(log, 0o600)
}

// assertPerm checks that path carries exactly the permission bits want, so a
// location or file whose bits drifted (a 0644 log, a 0755 directory) is caught.
func assertPerm(path string, want fs.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	got := info.Mode().Perm()
	if got != want {
		return fmt.Errorf("%s has permissions %#o, want %#o", path, got, want)
	}
	return nil
}
