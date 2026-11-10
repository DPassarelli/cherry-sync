// Step definitions for the colors the diff overlay draws its lines in, which only
// the emulated screen's cell attributes can show.

package acceptance_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/hinshun/vt10x"
)

// namedColors are the colors a scenario can name, as the emulator records them.
var namedColors = map[string]vt10x.Color{
	"red":   vt10x.Red,
	"green": vt10x.Green,
}

// theLinesBeginningShouldBe asserts that every visible character of the rows opening
// with each prefix is drawn in the named color.
func theLinesBeginningShouldBe(ctx context.Context, first, second, color string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the lines beginning "+first+" and "+second, func(s TerminalScreen) bool {
		return s.HasOverlay() && hasLineBeginning(s, first) && hasLineBeginning(s, second)
	})
	if err != nil {
		return err
	}
	for _, prefix := range []string{first, second} {
		err = assertRowColor(session, prefix, namedColors[color])
		if err != nil {
			return err
		}
	}
	return nil
}

// hasLineBeginning reports whether a line on screen opens with prefix.
func hasLineBeginning(s TerminalScreen, prefix string) bool {
	for _, line := range s.Lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// assertRowColor checks that the row opening with prefix draws every non-blank
// character in want, apart from the overlay's box and scrollbar around it.
func assertRowColor(session *terminalSession, prefix string, want vt10x.Color) error {
	row := session.rowBeginning(prefix)
	if row == nil {
		return fmt.Errorf("no row begins with %q", prefix)
	}
	for _, cell := range row {
		if cell.Char != ' ' && cell.Char != 0 && string(cell.Char) != frameSide && string(cell.Char) != frameThumb && cell.FG != want {
			return fmt.Errorf("in the row beginning %q, %q is color %d, want %d", prefix, cell.Char, cell.FG, want)
		}
	}
	return nil
}
