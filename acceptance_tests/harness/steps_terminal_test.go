// Step definitions for scenarios that drive csync through a real terminal.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
