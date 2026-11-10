// The parsing facade for csync's terminal screen: the single place that knows how
// the picker lays out its rows.

package acceptance_test

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// pickerHintFragment appears on the picker's key hint, and so marks the picker as
// on screen and ready for keys.
const pickerHintFragment = "enter sync"

// overlayTitlePrefix opens the title on the first line of a diff overlay. It stops
// short of the file's name, so an overlay missing that name still reads as open.
const overlayTitlePrefix = "Diff of"

// pickerPromptPrefix opens the picker's question, which the hint line follows.
const pickerPromptPrefix = "? "

// overlayRule is the character the overlay's rule above its hint line is drawn with.
const overlayRule = "─"

// noticePrefix opens every note the picker shows in place of a diff.
const noticePrefix = "cannot diff:"

// diffHintPrefix opens the picker's line about diffs, which any note follows.
const diffHintPrefix = "d diff"

// pickerRowRE matches a change row: the cursor marker, the checkbox, and the
// basename, which runs up to the two spaces before the verb.
var pickerRowRE = regexp.MustCompile(`^(❯| ) \[([ x])\] (.+?)  +(?:create|update|delete)\b`)

// pickerHeadingRE matches the directory heading printed above a group of rows.
var pickerHeadingRE = regexp.MustCompile(`^\./(.*)$`)

// PickerRow is one change row on screen.
type PickerRow struct {
	Path    string
	Cursor  bool
	Checked bool
}

// wrapIndicator opens each row that continues a line too wide for the terminal.
const wrapIndicator = "↪ "

// TerminalScreen is what the terminal is showing: its lines with trailing padding
// and the overlay's box removed, the picker's rows when any are on screen, and
// Unwrapped, every row run together without its wrap indicator, so a line wrapped
// across rows reads whole. Framed keeps the lines as drawn, box included.
type TerminalScreen struct {
	Lines     []string
	Rows      []PickerRow
	Unwrapped string
	Framed    []string
}

// Box-drawing pieces of the overlay's frame. A divider inside the box reads as a
// rule once its ends are removed, and the box's top and bottom are dropped.
const (
	frameSide       = "│"
	frameThumb      = "┃"
	frameDividerL   = "├"
	frameDividerR   = "┤"
	frameTopLeft    = "┌"
	frameBottomLeft = "└"
)

// unframe returns line as it reads inside the overlay's box, untrimmed, and false
// for a line that is the box's top or bottom. A line outside any box is unchanged.
func unframe(line string) (string, bool) {
	trimmed := strings.TrimRight(line, " ")
	switch {
	case strings.HasPrefix(trimmed, frameTopLeft), strings.HasPrefix(trimmed, frameBottomLeft):
		return "", false
	case strings.HasPrefix(trimmed, frameDividerL) && strings.HasSuffix(trimmed, frameDividerR):
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, frameDividerL), frameDividerR), true
	case strings.HasPrefix(trimmed, frameSide+" ") && strings.HasSuffix(trimmed, " "+frameSide):
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, frameSide+" "), " "+frameSide), true
	case strings.HasPrefix(trimmed, frameSide+" ") && strings.HasSuffix(trimmed, " "+frameThumb):
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, frameSide+" "), " "+frameThumb), true
	default:
		return line, true
	}
}

// String renders the screen for a failure message.
func (s TerminalScreen) String() string {
	return strings.Join(s.Lines, "\n")
}

// HasPicker reports whether the picker is on screen.
func (s TerminalScreen) HasPicker() bool {
	return strings.Contains(s.String(), pickerHintFragment)
}

// HasOverlay reports whether a diff overlay is on screen.
func (s TerminalScreen) HasOverlay() bool {
	return len(s.Lines) > 0 && strings.HasPrefix(s.Lines[0], overlayTitlePrefix)
}

// OverlayTitle returns the diff overlay's title, or "" when no overlay is on screen.
func (s TerminalScreen) OverlayTitle() string {
	if !s.HasOverlay() {
		return ""
	}
	return s.Lines[0]
}

// OverlayHeader returns the lines between the rule under the overlay's title and the
// rule that closes its header, or nil when no overlay is on screen. A line wrapped across rows
// is joined back into one, since how far a path wraps depends on where the
// scenario's directories happen to live.
func (s TerminalScreen) OverlayHeader() []string {
	if !s.HasOverlay() {
		return nil
	}
	var lines []string
	start := s.ruleAfter(0) + 1
	for _, row := range s.Lines[min(start, len(s.Lines)):s.ruleAfter(start)] {
		continued, wrapped := strings.CutPrefix(row, wrapIndicator)
		if wrapped && len(lines) > 0 {
			lines[len(lines)-1] += continued
			continue
		}
		lines = append(lines, row)
	}
	return lines
}

// OverlayBody returns the lines below the rule that closes the overlay's header,
// or nil when no overlay is on screen.
func (s TerminalScreen) OverlayBody() []string {
	if !s.HasOverlay() {
		return nil
	}
	end := s.ruleAfter(s.ruleAfter(0) + 1)
	return s.Lines[min(end+1, len(s.Lines)):]
}

// ruleAfter returns the index of the first line from start on drawn entirely in the
// overlay's rule, or the number of lines when there is none.
func (s TerminalScreen) ruleAfter(start int) int {
	for i := start; i < len(s.Lines); i++ {
		if isRule(s.Lines[i]) {
			return i
		}
	}
	return len(s.Lines)
}

// isRule reports whether line is drawn entirely in the overlay's rule.
func isRule(line string) bool {
	return line != "" && strings.Trim(line, overlayRule) == ""
}

// HasNotice reports whether the picker is showing a note about a file it can't diff.
func (s TerminalScreen) HasNotice() bool {
	return strings.HasPrefix(s.Note(), noticePrefix)
}

// DiffHint returns the picker's line about diffs without its indent or any note, or
// "" when it isn't on screen.
func (s TerminalScreen) DiffHint() string {
	hint, _ := s.diffHintParts()
	return hint
}

// Note returns what follows the picker's diff hint, or "" when nothing does.
func (s TerminalScreen) Note() string {
	_, note := s.diffHintParts()
	return note
}

// diffHintParts splits the picker's line about diffs at the parenthesis that closes
// its fixed text, since the note after it is free to contain any characters.
func (s TerminalScreen) diffHintParts() (string, string) {
	for _, line := range s.Lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, diffHintPrefix) {
			continue
		}
		hint, note, _ := strings.Cut(trimmed, ")")
		return hint + ")", strings.TrimSpace(note)
	}
	return "", ""
}

// HasLine reports whether a line on screen is exactly want.
func (s TerminalScreen) HasLine(want string) bool {
	return slices.Contains(s.Lines, want)
}

// HintLine returns the key hint on screen, without its indent: in the overlay, the
// line under its rule; in the picker, the line under its question.
func (s TerminalScreen) HintLine() string {
	for i := len(s.Lines) - 2; i >= 0; i-- {
		if isRule(s.Lines[i]) {
			return strings.TrimSpace(s.Lines[i+1])
		}
	}
	for i, line := range s.Lines[:max(len(s.Lines)-1, 0)] {
		if strings.HasPrefix(line, pickerPromptPrefix) {
			return strings.TrimSpace(s.Lines[i+1])
		}
	}
	return ""
}

// HasEndpoints reports whether the source and destination lines printed above the
// picker are on screen.
func (s TerminalScreen) HasEndpoints() bool {
	var source, destination bool
	for _, line := range s.Lines {
		source = source || strings.HasPrefix(line, "Source:")
		destination = destination || strings.HasPrefix(line, "Destination:")
	}
	return source && destination
}

// CursorIndex returns the position in Rows of the row under the cursor, or -1 when
// no row has it.
func (s TerminalScreen) CursorIndex() int {
	for i, row := range s.Rows {
		if row.Cursor {
			return i
		}
	}
	return -1
}

// RowIndex returns the position in Rows of the row for relPath, or -1 when there is
// none on screen.
func (s TerminalScreen) RowIndex(relPath string) int {
	for i, row := range s.Rows {
		if row.Path == relPath {
			return i
		}
	}
	return -1
}

// parseTerminalScreen translates the emulator's rendered screen into a
// TerminalScreen. It is the single parsing facade the terminal steps read through,
// so a change to the picker's layout only has to be absorbed here.
func parseTerminalScreen(raw string) TerminalScreen {
	var screen TerminalScreen
	dir := ""
	var unwrapped strings.Builder
	for drawn := range strings.SplitSeq(strings.TrimRight(raw, " \n"), "\n") {
		screen.Framed = append(screen.Framed, strings.TrimRight(drawn, " "))
		line, inside := unframe(drawn)
		if !inside || (len(screen.Lines) == 0 && strings.TrimSpace(line) == "") {
			continue
		}
		unwrapped.WriteString(strings.TrimPrefix(line, wrapIndicator))
		trimmed := strings.TrimRight(line, " ")
		screen.Lines = append(screen.Lines, trimmed)
		heading := pickerHeadingRE.FindStringSubmatch(trimmed)
		if heading != nil {
			dir = strings.TrimSuffix(heading[1], "/")
			continue
		}
		row := pickerRowRE.FindStringSubmatch(trimmed)
		if row != nil {
			screen.Rows = append(screen.Rows, PickerRow{Path: path.Join(dir, row[3]), Cursor: row[1] == "❯", Checked: row[2] == "x"})
		}
	}
	screen.Unwrapped = unwrapped.String()
	return screen
}

// ScrollbarRows returns, for each row of the overlay's diff, whether the scrollbar's
// thumb is drawn beside it, or nil when no overlay is on screen. The diff's rows are
// the ones between the box's second and third dividers.
func (s TerminalScreen) ScrollbarRows() []bool {
	if !s.HasOverlay() {
		return nil
	}
	var rows []bool
	dividers := 0
	for _, line := range s.Framed {
		if strings.HasPrefix(line, frameDividerL) {
			dividers++
			continue
		}
		if dividers == 2 {
			rows = append(rows, strings.HasSuffix(line, frameThumb))
		}
	}
	return rows
}
