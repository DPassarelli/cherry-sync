// Step definitions for how the picker styles a row's detail, which only the
// emulated screen's cell attributes can show.

package acceptance_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hinshun/vt10x"
)

// glyphBold is vt10x's bold attribute bit, which the package keeps unexported.
const glyphBold = 1 << 2

// detailComplete recognizes a row whose detail has finished drawing, so a check for
// the absence of bold can't pass against a half-painted frame.
var detailComplete = regexp.MustCompile(`update\s+.*(ago|mtime)\s*$`)

// awaitDetailBold waits for the cursor row's detail to draw and reports each unbroken
// run of bold text in it. Only cells after the verb count, since the caret is bold on
// every row and is not part of the detail.
func awaitDetailBold(ctx context.Context) ([]string, error) {
	session, err := runningTerminal(ctx)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(terminalWait)
	for {
		row := session.cursorRow()
		text := glyphText(row)
		if detailComplete.MatchString(text) {
			return boldRuns(row, text), nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("cursor row never showed a complete detail; row was %q", text)
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

// boldRuns collects the unbroken runs of bold characters following a row's verb.
func boldRuns(row []vt10x.Glyph, text string) []string {
	verbEnd := len([]rune(text[:strings.Index(text, "update")+len("update")]))
	var runs []string
	run := ""
	for _, cell := range row[verbEnd:] {
		if cell.Mode&glyphBold != 0 {
			run += string(cell.Char)
			continue
		}
		if run != "" {
			runs = append(runs, run)
			run = ""
		}
	}
	return runs
}

// assertBoldRuns checks that the detail shows exactly the wanted runs in bold.
func assertBoldRuns(ctx context.Context, want ...string) error {
	got, err := awaitDetailBold(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("bold text was %q, want %q", got, want)
	}
	return nil
}

// theBoldTextInTheDetailShouldBe checks for a single unbroken run of bold text.
func theBoldTextInTheDetailShouldBe(ctx context.Context, want string) error {
	return assertBoldRuns(ctx, want)
}

// theBoldTextInTheDetailShouldBeBoth checks for two separate runs of bold text.
func theBoldTextInTheDetailShouldBeBoth(ctx context.Context, first, second string) error {
	return assertBoldRuns(ctx, first, second)
}

// noPartOfTheDetailShouldBeBold checks that the detail draws with no bold at all.
func noPartOfTheDetailShouldBeBold(ctx context.Context) error {
	return assertBoldRuns(ctx)
}

// theNoteShouldBeBold asserts every character of the note after the picker's diff
// hint is drawn bold.
func theNoteShouldBeBold(ctx context.Context, note string) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = session.waitFor("the note "+note, func(s TerminalScreen) bool { return s.Note() == note })
	if err != nil {
		return err
	}
	row := session.rowContaining(note)
	text := []rune(glyphText(row))
	start := strings.Index(glyphText(row), note)
	if start < 0 {
		return fmt.Errorf("no row contains the note %q", note)
	}
	first := len([]rune(glyphText(row)[:start]))
	for i := first; i < first+len([]rune(note)); i++ {
		if row[i].Mode&glyphBold == 0 {
			return fmt.Errorf("%q in the note %q isn't bold", text[i], note)
		}
	}
	return nil
}

// theDiffOverlaysScrollbarThumbShouldBeBold asserts every cell of the thumb on screen
// is bold, and that there is a thumb to check.
func theDiffOverlaysScrollbarThumbShouldBeBold(ctx context.Context) error {
	session, err := runningTerminal(ctx)
	if err != nil {
		return err
	}
	_, err = settledOverlay(ctx)
	if err != nil {
		return err
	}
	cells := session.cellsOf([]rune(frameThumb)[0])
	if len(cells) == 0 {
		return fmt.Errorf("no scrollbar thumb is on screen")
	}
	for _, cell := range cells {
		if cell.Mode&glyphBold == 0 {
			return fmt.Errorf("a cell of the scrollbar thumb isn't bold")
		}
	}
	return nil
}
