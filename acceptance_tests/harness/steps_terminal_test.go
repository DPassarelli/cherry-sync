// Step definitions for scenarios that drive csync through a real terminal.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// theFileHasBeenChangedLocallyFromTo gives the remote copy of relPath the content
// before and the local copy the content after, so a scenario can name the lines its
// diff must show.
func theFileHasBeenChangedLocallyFromTo(ctx context.Context, relPath, before, after string) (context.Context, error) {
	ctx, err := theFileHasBeenChangedLocally(ctx, relPath)
	if err != nil {
		return ctx, err
	}
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)
	err = os.WriteFile(filepath.Join(remote, relPath), []byte(before+"\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write remote %s: %w", relPath, err)
	}
	err = os.WriteFile(filepath.Join(local, relPath), []byte(after+"\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write local %s: %w", relPath, err)
	}
	return ctx, nil
}

// theCopiesHaveBeenChangedTo rewrites the named copies of relPath. Each copy gets its
// own content from fill, so that "both copies" still differ and the file keeps its row.
func theCopiesHaveBeenChangedTo(ctx context.Context, which, relPath string, fill func(side string) []byte) (context.Context, error) {
	ctx, err := mirrorRemote(ctx)
	if err != nil {
		return ctx, err
	}
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)
	if which != "the remote copy" {
		err = os.WriteFile(filepath.Join(local, relPath), fill("local"), 0o644)
		if err != nil {
			return ctx, fmt.Errorf("write local %s: %w", relPath, err)
		}
	}
	if which != "the local copy" {
		err = os.WriteFile(filepath.Join(remote, relPath), fill("remote"), 0o644)
		if err != nil {
			return ctx, fmt.Errorf("write remote %s: %w", relPath, err)
		}
	}
	return ctx, nil
}

// theCopiesHaveBeenChangedToSize makes the named copies the given size, written as
// the scenarios write it, with a thousands separator.
func theCopiesHaveBeenChangedToSize(ctx context.Context, which, relPath, size string) (context.Context, error) {
	byteCount, err := strconv.Atoi(strings.ReplaceAll(size, ",", ""))
	if err != nil {
		return ctx, fmt.Errorf("size %q: %w", size, err)
	}
	return theCopiesHaveBeenChangedTo(ctx, which, relPath, func(side string) []byte {
		return bytes.Repeat([]byte(side[:1]), byteCount)
	})
}

// theCopiesHaveBeenChangedToContainANulByte puts a NUL byte in the named copies.
func theCopiesHaveBeenChangedToContainANulByte(ctx context.Context, which, relPath string) (context.Context, error) {
	return theCopiesHaveBeenChangedTo(ctx, which, relPath, func(side string) []byte {
		return []byte(side + "\x00\n")
	})
}

// thatIHaveRunInATerminal starts csync under a pseudo-terminal and waits for the
// picker to be ready for keys.
func thatIHaveRunInATerminal(ctx context.Context, command string) (context.Context, error) {
	ctx, args, err := resolveCsyncArgs(ctx, command)
	if err != nil {
		return ctx, err
	}
	session, err := startTerminalSession(ctx, args)
	if err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, terminalSessionKey{}, session)
	remote, _ := ctx.Value(remotePathKey{}).(string)
	before, err := treeContents(remote)
	if err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, remoteBeforeKey{}, before)
	_, err = session.waitFor("the picker", TerminalScreen.HasPicker)
	return ctx, err
}

// runningTerminal returns the scenario's terminal session.
func runningTerminal(ctx context.Context) (*terminalSession, error) {
	session, _ := ctx.Value(terminalSessionKey{}).(*terminalSession)
	if session == nil {
		return nil, fmt.Errorf("csync isn't running in a terminal; missing Given step?")
	}
	return session, nil
}

// iPressOnTheRowFor moves the cursor to relPath's row, then presses key. Each move
// waits for the cursor to land, so a key can't overtake the redraw it depends on.
func iPressOnTheRowFor(ctx context.Context, key, relPath string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	screen, err := session.waitFor("the row for "+relPath, func(s TerminalScreen) bool { return s.RowIndex(relPath) >= 0 })
	if err != nil {
		return err
	}
	target := screen.RowIndex(relPath)
	for screen.CursorIndex() != target {
		move := "down"
		next := screen.CursorIndex() + 1
		if screen.CursorIndex() > target {
			move = "up"
			next = screen.CursorIndex() - 1
		}
		err = session.press(move)
		if err != nil {
			return err
		}
		screen, err = session.waitFor("the cursor on row "+relPath, func(s TerminalScreen) bool { return s.CursorIndex() == next })
		if err != nil {
			return err
		}
	}
	return session.press(key)
}

// remoteBeforeKey stashes the remote tree's contents from when csync started, so a
// step can tell whether anything has been written to it since.
type remoteBeforeKey struct{}

// treeContents reads every file under root, keyed by its path relative to root.
func treeContents(root string) (map[string]string, error) {
	contents := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		contents[rel] = string(data)
		return err
	})
	return contents, err
}

// thatIHavePressedOnTheRowFor presses key on relPath's row and waits for the overlay,
// since the fetch behind it finishes in its own time and a later key mustn't overtake it.
func thatIHavePressedOnTheRowFor(ctx context.Context, key, relPath string) error {
	err := iPressOnTheRowFor(ctx, key, relPath)
	if err != nil {
		return err
	}
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff overlay", TerminalScreen.HasOverlay)
	return err
}

// iPress presses key.
func iPress(ctx context.Context, key string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	return session.press(key)
}

// thatIHavePressed presses key and waits for csync to finish handling it, so the next
// key can't overtake it.
func thatIHavePressed(ctx context.Context, key string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	err = session.press(key)
	if err != nil {
		return err
	}
	_, err = session.settle()
	return err
}

// theDiffOverlayShouldNoLongerBeVisible asserts the picker has come back.
func theDiffOverlayShouldNoLongerBeVisible(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the picker", func(s TerminalScreen) bool { return s.HasPicker() && !s.HasOverlay() })
	return err
}

// theSourceAndDestinationShouldBeOnScreen asserts the picker came back on the screen
// it started on, the one holding what csync printed above it.
func theSourceAndDestinationShouldBeOnScreen(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the picker", TerminalScreen.HasPicker)
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	if !screen.HasEndpoints() {
		return fmt.Errorf("the source and destination lines are gone; screen was:\n%s", screen)
	}
	return nil
}

// theDiffOverlayShouldStillBeOpen asserts the last key changed nothing visible.
func theDiffOverlayShouldStillBeOpen(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	if !screen.HasOverlay() {
		return fmt.Errorf("the diff overlay closed; screen was:\n%s", screen)
	}
	return nil
}

// noFilesShouldHaveBeenTransferred compares the remote's bytes with what they were
// when csync started, because those are what a real sync would change.
func noFilesShouldHaveBeenTransferred(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.settle()
	if err != nil {
		return err
	}
	remote, _ := ctx.Value(remotePathKey{}).(string)
	after, err := treeContents(remote)
	if err != nil {
		return err
	}
	before, _ := ctx.Value(remoteBeforeKey{}).(map[string]string)
	if !maps.Equal(before, after) {
		return fmt.Errorf("the remote changed: had %d files, now %d, or their contents differ", len(before), len(after))
	}
	return nil
}

// theRowShouldBeChecked asserts relPath's checkbox is ticked once the picker is back.
func theRowShouldBeChecked(ctx context.Context, relPath string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the row for "+relPath, func(s TerminalScreen) bool { return s.RowIndex(relPath) >= 0 })
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	index := screen.RowIndex(relPath)
	if index < 0 || !screen.Rows[index].Checked {
		return fmt.Errorf("row for %s isn't checked; screen was:\n%s", relPath, screen)
	}
	return nil
}

// csyncShouldHaveExited asserts the process ended, as opposed to only leaving no sign
// of itself on screen.
func csyncShouldHaveExited(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	select {
	case <-session.exited:
		return nil
	case <-time.After(terminalWait):
		return fmt.Errorf("csync was still running after %s", terminalWait)
	}
}

// theDiffOverlayShouldInclude asserts the expected lines appear together on screen.
func theDiffOverlayShouldInclude(ctx context.Context, want *godog.DocString) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the lines:\n"+want.Content, func(s TerminalScreen) bool {
		return strings.Contains(s.String(), want.Content)
	})
	return err
}

// theDiffOverlayShouldBeginWith asserts the screen opens with the expected lines,
// since the overlay takes the whole terminal.
func theDiffOverlayShouldBeginWith(ctx context.Context, want *godog.DocString) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("an overlay beginning:\n"+want.Content, func(s TerminalScreen) bool {
		return strings.HasPrefix(s.String(), want.Content)
	})
	return err
}

// thePickerShouldShowANoticeContaining asserts the picker is on screen with the
// phrase, so a notice can't be confused with the same words in an overlay.
func thePickerShouldShowANoticeContaining(ctx context.Context, phrase string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("a notice containing "+phrase, func(s TerminalScreen) bool {
		return s.HasPicker() && strings.Contains(s.String(), phrase)
	})
	return err
}
