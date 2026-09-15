// Steps for the per-file difference detail: setting up copies that differ in
// size or modification time, and asserting how csync explains the difference.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// sideDir resolves the "local" or "remote" side of a scenario to its directory, so
// the mtime steps can address either without a step apiece.
func sideDir(ctx context.Context, side string) (string, error) {
	key := localPathKey{}
	var dir string
	if side == "remote" {
		dir, _ = ctx.Value(remotePathKey{}).(string)
	} else {
		dir, _ = ctx.Value(key).(string)
	}
	if dir == "" {
		return "", fmt.Errorf("%s path not set; missing Background step?", side)
	}
	return dir, nil
}

// theLocalCopyIsLarger rewrites the local file so it holds the remote copy's bytes
// plus the stated number of kibibytes, making the two differ in size by exactly
// that much. Padding the existing content (rather than writing a fresh file of some
// absolute size) is what lets the scenario state the gap it cares about without
// also having to know what the fixture happened to put there.
func theLocalCopyIsLarger(ctx context.Context, relPath string, kb int) (context.Context, error) {
	local, err := sideDir(ctx, "local")
	if err != nil {
		return ctx, err
	}
	remote, err := sideDir(ctx, "remote")
	if err != nil {
		return ctx, err
	}
	base, err := os.ReadFile(filepath.Join(remote, relPath))
	if err != nil {
		return ctx, fmt.Errorf("read remote %s: %w", relPath, err)
	}
	padded := append(base, bytes.Repeat([]byte("x"), kb*1024)...)
	err = os.WriteFile(filepath.Join(local, relPath), padded, 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write local %s: %w", relPath, err)
	}
	return ctx, nil
}

// theTwoCopiesDifferInContentButNotSize gives each side its own bytes at the same
// length, so only a content hash can tell the copies apart. It writes both sides
// because the state it establishes belongs to the pair: the fixture's files start
// empty, so changing one alone would change the size along with the content and
// describe a different case entirely.
func theTwoCopiesDifferInContentButNotSize(ctx context.Context, relPath string) (context.Context, error) {
	// Distinct bytes of equal length, long enough that the two cannot collide.
	content := map[string][]byte{
		"local":  bytes.Repeat([]byte("L"), 512),
		"remote": bytes.Repeat([]byte("R"), 512),
	}
	for _, side := range []string{"local", "remote"} {
		dir, err := sideDir(ctx, side)
		if err != nil {
			return ctx, err
		}
		full := filepath.Join(dir, relPath)
		err = os.WriteFile(full, content[side], 0o644)
		if err != nil {
			return ctx, fmt.Errorf("write %s %s: %w", side, full, err)
		}
	}
	return ctx, nil
}

// theCopyWasLastModified stamps one side's copy of a file with an age relative to
// now, so a scenario can state the age it expects the report to render. The ages
// used are coarse (minutes and up) because csync renders them in whole units, and a
// scenario that pinned seconds would race the clock it is measured against.
func theCopyWasLastModified(ctx context.Context, side, relPath string, count int, unit string) (context.Context, error) {
	dir, err := sideDir(ctx, side)
	if err != nil {
		return ctx, err
	}
	units := map[string]time.Duration{
		"second": time.Second,
		"minute": time.Minute,
		"hour":   time.Hour,
		"day":    24 * time.Hour,
	}
	span, ok := units[unit]
	if !ok {
		return ctx, fmt.Errorf("unknown unit %q", unit)
	}
	when := time.Now().Add(-time.Duration(count) * span)
	full := filepath.Join(dir, relPath)
	err = os.Chtimes(full, when, when)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// bothCopiesCarryTheSameModificationTime stamps the two copies of a file with one
// timestamp, so nothing outward separates them and rsync's size+mtime quick check
// reports nothing about the file. It is what puts a differing file beyond the
// destination measurement, leaving the row to say the content is the only
// difference.
func bothCopiesCarryTheSameModificationTime(ctx context.Context, relPath string) (context.Context, error) {
	when := time.Now().Add(-time.Hour)
	for _, side := range []string{"local", "remote"} {
		dir, err := sideDir(ctx, side)
		if err != nil {
			return ctx, err
		}
		full := filepath.Join(dir, relPath)
		err = os.Chtimes(full, when, when)
		if err != nil {
			return ctx, fmt.Errorf("chtimes %s %s: %w", side, full, err)
		}
	}
	return ctx, nil
}

// aRemoteThatFailsTheMeasurement points this scenario's remote shell at the one
// that answers the comparison and refuses the pass reading the destination. See
// failMeasureRshScript.
func aRemoteThatFailsTheMeasurement(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, failMeasureModeKey{}, true), nil
}

// reportedAction finds the reported action for a path, so the detail assertions can
// speak about one row.
func reportedAction(ctx context.Context, relPath string) (Action, error) {
	out, ok := ctx.Value(outputKey{}).(runResult)
	if !ok {
		return Action{}, fmt.Errorf("no run output; missing When step?")
	}
	for _, a := range parseOutput(out.Stdout, out.Stderr).Actions {
		if a.Path == relPath {
			return a, nil
		}
	}
	return Action{}, fmt.Errorf("no action reported for %q; reported: %s", relPath, out.Stdout)
}

// theReportedDetailForShouldBe asserts the annotation csync printed for one file.
func theReportedDetailForShouldBe(ctx context.Context, relPath, want string) error {
	action, err := reportedAction(ctx, relPath)
	if err != nil {
		return err
	}
	if action.Detail != want {
		return fmt.Errorf("detail for %q was %q, want %q", relPath, action.Detail, want)
	}
	return nil
}

// noDetailShouldBeReportedFor asserts a row carries no annotation, which is how a
// create and a delete are reported: neither has a second copy to be compared with.
func noDetailShouldBeReportedFor(ctx context.Context, relPath string) error {
	action, err := reportedAction(ctx, relPath)
	if err != nil {
		return err
	}
	if action.Detail != "" {
		return fmt.Errorf("detail for %q was %q, want none", relPath, action.Detail)
	}
	return nil
}
