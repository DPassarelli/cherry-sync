// Step definitions for scenarios that drive csync through a real terminal.

package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// aRemoteThatIsSlowToSendFiles has the scenario's remote delay every file it sends.
func aRemoteThatIsSlowToSendFiles(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, slowSendModeKey{}, true), nil
}

// aRemoteThatFailsToSendFiles has the scenario's remote refuse every file it is asked
// to send.
func aRemoteThatFailsToSendFiles(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, failSendModeKey{}, true), nil
}

// longFailMessage is a refusal too long for one row of the terminal.
var longFailMessage = "the test remote refused to send this file, " + strings.Repeat("and it keeps explaining why ", terminalCols/20)

// aRemoteThatFailsToSendFilesWithALongMessage has the scenario's remote refuse every
// file with longFailMessage.
func aRemoteThatFailsToSendFilesWithALongMessage(ctx context.Context) (context.Context, error) {
	ctx = context.WithValue(ctx, failSendModeKey{}, true)
	return context.WithValue(ctx, failSendMessageKey{}, longFailMessage), nil
}

// theDiffOverlayShouldIncludeAllOfWhatTheRemoteSaid asserts the whole refusal is on
// screen, however many rows it takes.
func theDiffOverlayShouldIncludeAllOfWhatTheRemoteSaid(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("all of what the remote said", func(s TerminalScreen) bool {
		return !s.HasPicker() && strings.Contains(s.Unwrapped, strings.TrimSpace(longFailMessage))
	})
	return err
}

// wideLine is a line of content too wide for one row of the terminal, made of
// distinct characters so a row lost or repeated in wrapping can't still match.
var wideLine = func() string {
	var b strings.Builder
	for i := 0; b.Len() < terminalCols*5/2; i++ {
		fmt.Fprintf(&b, "%03d.", i)
	}
	return b.String()
}()

// theFileHasBeenChangedLocallyToASingleWideLine replaces relPath's local content with
// wideLine, so its diff adds a line that has to wrap.
func theFileHasBeenChangedLocallyToASingleWideLine(ctx context.Context, relPath string) (context.Context, error) {
	return theFileHasBeenChangedLocallyFromTo(ctx, relPath, "short line", wideLine)
}

// theFileHasBeenChangedLocallyToTheLine replaces relPath's local content with line,
// reading "<tab>" as a tab, since a scenario can't easily hold a literal one.
func theFileHasBeenChangedLocallyToTheLine(ctx context.Context, relPath, line string) (context.Context, error) {
	return theFileHasBeenChangedLocallyFromTo(ctx, relPath, "short line", strings.ReplaceAll(line, "<tab>", "\t"))
}

// theDiffOverlayShouldIncludeTheLine asserts a row of the overlay is exactly want.
func theDiffOverlayShouldIncludeTheLine(ctx context.Context, want string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the line "+want, func(s TerminalScreen) bool { return s.HasLine(want) })
	return err
}

// theDiffOverlayShouldIncludeAllOfThatLine asserts the added wide line is on screen
// in full, however many rows it takes.
func theDiffOverlayShouldIncludeAllOfThatLine(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("all of the wide line", func(s TerminalScreen) bool {
		return s.HasOverlay() && strings.Contains(s.Unwrapped, "+"+wideLine)
	})
	return err
}

// theRowAfterThatLinesFirstRowShouldBeginWith asserts how the wide line's second row
// opens.
func theRowAfterThatLinesFirstRowShouldBeginWith(ctx context.Context, prefix string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	first := "+" + wideLine[:10]
	_, err = session.waitFor("a row beginning "+first+" followed by one beginning "+prefix, func(s TerminalScreen) bool {
		for i, line := range s.Lines[:max(len(s.Lines)-1, 0)] {
			if strings.HasPrefix(line, first) {
				return strings.HasPrefix(s.Lines[i+1], prefix)
			}
		}
		return false
	})
	return err
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

// thatIHavePressedOnTheRowFor presses key on relPath's row and waits for the overlay
// or a notice, since a fetch finishes in its own time and a later key mustn't overtake it.
func thatIHavePressedOnTheRowFor(ctx context.Context, key, relPath string) error {
	err := iPressOnTheRowFor(ctx, key, relPath)
	if err != nil {
		return err
	}
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff overlay or a notice", func(s TerminalScreen) bool {
		return s.HasOverlay() || s.HasNotice()
	})
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

// theNoticeShouldNoLongerBeVisible asserts the notice has gone from the picker.
func theNoticeShouldNoLongerBeVisible(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the picker without a notice", func(s TerminalScreen) bool {
		return s.HasPicker() && !s.HasNotice()
	})
	return err
}

// theCursorShouldBeOnTheRowFor asserts the cursor has reached relPath's row.
func theCursorShouldBeOnTheRowFor(ctx context.Context, relPath string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the cursor on the row for "+relPath, func(s TerminalScreen) bool {
		return s.RowIndex(relPath) >= 0 && s.CursorIndex() == s.RowIndex(relPath)
	})
	return err
}

// tallDiffPairs is how many lines differ in a diff taller than the terminal: each
// shows once removed and once added, so half this count would already overflow it.
const tallDiffPairs = terminalRows * 2

// tallDiffLength is an upper bound on the lines in that diff, headers included.
const tallDiffLength = tallDiffPairs*2 + 3

// tallDiffLastLine is the last line of that diff, since its additions follow its
// removals.
var tallDiffLastLine = fmt.Sprintf("+new line %d", tallDiffPairs)

// theFileHasBeenChangedSoThatItsDiffIsTaller makes every line differ between the two
// copies, sized from the terminal so the overflow doesn't depend on a magic number.
func theFileHasBeenChangedSoThatItsDiffIsTaller(ctx context.Context, relPath string) (context.Context, error) {
	var before, after strings.Builder
	for i := 1; i <= tallDiffPairs; i++ {
		fmt.Fprintf(&before, "old line %d\n", i)
		fmt.Fprintf(&after, "new line %d\n", i)
	}
	return theFileHasBeenChangedLocallyFromTo(ctx, relPath, strings.TrimSuffix(before.String(), "\n"), strings.TrimSuffix(after.String(), "\n"))
}

// iPressMoreTimesThanTheDiffHasLines presses key enough to pass the diff's end.
func iPressMoreTimesThanTheDiffHasLines(ctx context.Context, key string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	for range tallDiffLength + 1 {
		err = session.press(key)
		if err != nil {
			return err
		}
	}
	return nil
}

// thatIHavePressedMoreTimesThanTheDiffHasLines presses key past the diff's end and
// waits for csync to finish handling every press.
func thatIHavePressedMoreTimesThanTheDiffHasLines(ctx context.Context, key string) error {
	err := iPressMoreTimesThanTheDiffHasLines(ctx, key)
	if err != nil {
		return err
	}
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.settle()
	return err
}

// theDiffOverlayShouldShowItsLastLine asserts the end of the diff has come into view.
func theDiffOverlayShouldShowItsLastLine(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff's last line", func(s TerminalScreen) bool { return s.HasLine(tallDiffLastLine) })
	return err
}

// theDiffOverlayShouldNotShowItsLastLine asserts the end of the diff is out of view.
func theDiffOverlayShouldNotShowItsLastLine(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	if screen.HasPicker() {
		return fmt.Errorf("the diff overlay isn't open; screen was:\n%s", screen)
	}
	if screen.HasLine(tallDiffLastLine) {
		return fmt.Errorf("the diff's last line is on screen:\n%s", screen)
	}
	return nil
}

// theHintLineShouldRead asserts the key hint on screen is exactly want.
func theHintLineShouldRead(ctx context.Context, want string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the hint line "+want, func(s TerminalScreen) bool { return s.HintLine() == want })
	return err
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

// theDiffOverlaysTitleShouldRead asserts the overlay's title is exactly want once
// csync has handled every key sent, since the title is on screen before a scroll
// that might wrongly take it away.
func theDiffOverlaysTitleShouldRead(ctx context.Context, want string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff overlay", TerminalScreen.HasOverlay)
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	if screen.OverlayTitle() != want {
		return fmt.Errorf("title was %q, want %q; screen was:\n%s", screen.OverlayTitle(), want, screen)
	}
	return nil
}

// theDiffOverlaysHeaderShouldRead asserts the overlay's header is exactly want once
// the scenario's real paths are read back as the placeholders it was written with.
func theDiffOverlaysHeaderShouldRead(ctx context.Context, want *godog.DocString) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the header:\n"+want.Content, func(s TerminalScreen) bool {
		return withPlaceholders(ctx, strings.Join(s.OverlayHeader(), "\n")) == want.Content
	})
	return err
}

// theDiffOverlayShouldBeginWithText asserts the overlay's body opens with prefix.
func theDiffOverlayShouldBeginWithText(ctx context.Context, prefix string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("an overlay beginning "+prefix, func(s TerminalScreen) bool {
		body := s.OverlayBody()
		return len(body) > 0 && strings.HasPrefix(body[0], prefix)
	})
	return err
}

// theDiffOverlayShouldIncludeWhatTheRemoteSaid asserts the refusing remote's own
// words reached the overlay.
func theDiffOverlayShouldIncludeWhatTheRemoteSaid(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("what the remote said", func(s TerminalScreen) bool {
		return !s.HasPicker() && strings.Contains(s.String(), failSendMessage)
	})
	return err
}

// thePickerShouldNote asserts the picker is on screen with exactly want as the note
// after its diff hint.
func thePickerShouldNote(ctx context.Context, want string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the note "+want, func(s TerminalScreen) bool {
		return s.HasPicker() && s.Note() == want
	})
	return err
}

// thePickersNoteShouldContain asserts the note after the picker's diff hint holds
// phrase.
func thePickersNoteShouldContain(ctx context.Context, phrase string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("a note containing "+phrase, func(s TerminalScreen) bool {
		return s.HasPicker() && strings.Contains(s.Note(), phrase)
	})
	return err
}

// theDiffHintShouldRead asserts the picker's line about diffs is exactly want.
func theDiffHintShouldRead(ctx context.Context, want string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff hint "+want, func(s TerminalScreen) bool { return s.DiffHint() == want })
	return err
}

// aRemoteThatRecordsEachFileItSends points the scenario's remote at the recording
// shell, logging into the scenario's throwaway home so the log goes with it.
func aRemoteThatRecordsEachFileItSends(ctx context.Context) (context.Context, error) {
	home, _ := ctx.Value(homeKey{}).(string)
	if home == "" {
		return ctx, fmt.Errorf("no scenario home to keep the send log in")
	}
	return context.WithValue(ctx, sendLogKey{}, filepath.Join(home, "sends")), nil
}

// theRemoteShouldNotHaveBeenAskedForACopy waits for the picker's note, which comes
// once the diff's fetch is over, then checks the remote logged no send.
func theRemoteShouldNotHaveBeenAskedForACopy(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("a note", TerminalScreen.HasNotice)
	if err != nil {
		return err
	}
	sendLog, _ := ctx.Value(sendLogKey{}).(string)
	data, err := os.ReadFile(sendLog)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("the remote sent %d time(s)", strings.Count(string(data), "\n"))
}

// theDiffOverlayShouldBeDrawnInsideABox asserts the overlay's first and last lines
// close a box whose every other line meets both of its sides.
func theDiffOverlayShouldBeDrawnInsideABox(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the diff overlay", TerminalScreen.HasOverlay)
	if err != nil {
		return err
	}
	screen, err := session.settle()
	if err != nil {
		return err
	}
	if !isBox(withoutLeadingBlanks(screen.Framed)) {
		return fmt.Errorf("the overlay isn't drawn inside a box; screen was:\n%s", strings.Join(screen.Framed, "\n"))
	}
	return nil
}

// isBox reports whether lines form one closed box the width of the terminal.
func isBox(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	last := len(lines) - 1
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) != terminalCols {
			return false
		}
		left, right := string(runes[0]), string(runes[len(runes)-1])
		switch i {
		case 0:
			if left != "┌" || right != "┐" {
				return false
			}
		case last:
			if left != "└" || right != "┘" {
				return false
			}
		default:
			if !(left == "│" && (right == "│" || right == "┃")) && !(left == "├" && right == "┤") {
				return false
			}
		}
	}
	return true
}

// withoutLeadingBlanks drops the blank lines that open lines.
func withoutLeadingBlanks(lines []string) []string {
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	return lines
}

// settledOverlay waits for the overlay, then for csync to finish handling every key
// sent, and returns the screen it settles on.
func settledOverlay(ctx context.Context) (TerminalScreen, error) {
	session, err := runningTerminal(ctx)
	if err != nil {
		return TerminalScreen{}, err
	}
	_, err = session.waitFor("the diff overlay", TerminalScreen.HasOverlay)
	if err != nil {
		return TerminalScreen{}, err
	}
	return session.settle()
}

// theDiffOverlaysBoxShouldHaveABlankRowAboveIt asserts the screen opens with one
// blank row and then the box's top.
func theDiffOverlaysBoxShouldHaveABlankRowAboveIt(ctx context.Context) error {
	screen, err := settledOverlay(ctx)
	if err != nil {
		return err
	}
	f := screen.Framed
	if len(f) < 2 || f[0] != "" || !strings.HasPrefix(f[1], frameTopLeft) {
		return fmt.Errorf("the box doesn't sit under one blank row; screen was:\n%s", strings.Join(f, "\n"))
	}
	return nil
}

// theDiffOverlaysScrollbarShouldBeAt asserts the thumb touches the named end of the
// diff's rows and not the other.
func theDiffOverlaysScrollbarShouldBeAt(ctx context.Context, end string) error {
	screen, err := settledOverlay(ctx)
	if err != nil {
		return err
	}
	rows := screen.ScrollbarRows()
	if len(rows) < 2 {
		return fmt.Errorf("the diff has %d rows, too few to place a scrollbar", len(rows))
	}
	top, bottom := rows[0], rows[len(rows)-1]
	if (end == "top" && (!top || bottom)) || (end == "bottom" && (top || !bottom)) {
		return fmt.Errorf("scrollbar isn't at the %s (thumb by row: %v); screen was:\n%s", end, rows, strings.Join(screen.Framed, "\n"))
	}
	return nil
}

// theDiffOverlayShouldShowNoScrollbar asserts no row of the diff has the thumb.
func theDiffOverlayShouldShowNoScrollbar(ctx context.Context) error {
	screen, err := settledOverlay(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(screen.ScrollbarRows(), true) {
		return fmt.Errorf("a scrollbar is showing; screen was:\n%s", strings.Join(screen.Framed, "\n"))
	}
	return nil
}
