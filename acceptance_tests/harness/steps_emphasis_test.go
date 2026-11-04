// Step definitions for how the picker styles a row's detail, which only the
// emulated screen's cell attributes can show.

package acceptance_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hinshun/vt10x"
)

// glyphBold is vt10x's bold attribute bit, which the package keeps unexported.
const glyphBold = 1 << 2

// lastUpdatedPhrase is the whole phrase a highlighted copy must carry in bold.
var lastUpdatedPhrase = regexp.MustCompile(`^last updated \d+[smhd] ago$`)

// detailComplete recognizes a row whose detail has finished drawing, so a check for
// the absence of bold can't pass against a half-painted frame.
var detailComplete = regexp.MustCompile(`update\s+.*(ago|mtime)\s*$`)

// detailBold holds the bold text in each half of a row's detail.
type detailBold struct {
	source string
	dest   string
}

// awaitDetailBold waits for the cursor row's detail to draw and reports what it
// shows in bold. Only cells after the verb count, since the caret is bold on every
// row and is not part of the detail.
func awaitDetailBold(ctx context.Context) (detailBold, error) {
	session, err := runningTerminal(ctx)
	if err != nil {
		return detailBold{}, err
	}
	deadline := time.Now().Add(terminalWait)
	for {
		row := session.cursorRow()
		text := glyphText(row)
		if detailComplete.MatchString(text) {
			return boldInDetail(row, text), nil
		}
		if time.Now().After(deadline) {
			return detailBold{}, fmt.Errorf("cursor row never showed a complete detail; row was %q", text)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// glyphText returns the characters of a row of cells.
func glyphText(row []vt10x.Glyph) string {
	var b strings.Builder
	for _, cell := range row {
		b.WriteRune(cell.Char)
	}
	return b.String()
}

// boldInDetail splits a row at the middot that divides the two copies and collects
// the bold characters on each side of it.
func boldInDetail(row []vt10x.Glyph, text string) detailBold {
	verbEnd := len([]rune(text[:strings.Index(text, "update")+len("update")]))
	var bold detailBold
	half := &bold.source
	for _, cell := range row[verbEnd:] {
		if cell.Char == '·' {
			half = &bold.dest
		}
		if cell.Mode&glyphBold != 0 {
			*half += string(cell.Char)
		}
	}
	return bold
}

// onlyThePhraseInTheDetailShouldBeBold checks that the named copy's "last updated"
// phrase is bold in full and that nothing else in the detail is.
func onlyThePhraseInTheDetailShouldBeBold(ctx context.Context, side string) error {
	bold, err := awaitDetailBold(ctx)
	if err != nil {
		return err
	}
	phrase, other := bold.source, bold.dest
	if side == "dest" {
		phrase, other = bold.dest, bold.source
	}
	if !lastUpdatedPhrase.MatchString(phrase) || other != "" {
		return fmt.Errorf("bold text was %+v, want only the %s phrase", bold, side)
	}
	return nil
}

// noPartOfTheDetailShouldBeBold checks that the detail draws with no bold at all.
func noPartOfTheDetailShouldBeBold(ctx context.Context) error {
	bold, err := awaitDetailBold(ctx)
	if err != nil {
		return err
	}
	if bold != (detailBold{}) {
		return fmt.Errorf("bold text was %+v, want none", bold)
	}
	return nil
}
