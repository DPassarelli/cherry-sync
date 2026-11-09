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

// noticePrefix opens every notice the picker shows in place of a diff.
const noticePrefix = "no diff: "

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
// removed, the picker's rows when any are on screen, and Unwrapped, every row run
// together without its wrap indicator, so a line wrapped across rows reads whole.
type TerminalScreen struct {
	Lines     []string
	Rows      []PickerRow
	Unwrapped string
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

// OverlayHeader returns the lines between the overlay's title and the rule that
// closes its header, or nil when no overlay is on screen.
func (s TerminalScreen) OverlayHeader() []string {
	if !s.HasOverlay() {
		return nil
	}
	end := s.firstRule()
	return s.Lines[1:end]
}

// OverlayBody returns the lines below the rule that closes the overlay's header,
// or nil when no overlay is on screen.
func (s TerminalScreen) OverlayBody() []string {
	if !s.HasOverlay() {
		return nil
	}
	return s.Lines[min(s.firstRule()+1, len(s.Lines)):]
}

// firstRule returns the index of the first line drawn entirely in the overlay's
// rule, or the number of lines when there is none.
func (s TerminalScreen) firstRule() int {
	for i, line := range s.Lines {
		if isRule(line) {
			return i
		}
	}
	return len(s.Lines)
}

// isRule reports whether line is drawn entirely in the overlay's rule.
func isRule(line string) bool {
	return line != "" && strings.Trim(line, overlayRule) == ""
}

// HasNotice reports whether the picker is showing a notice.
func (s TerminalScreen) HasNotice() bool {
	for _, line := range s.Lines {
		if strings.HasPrefix(line, noticePrefix) {
			return true
		}
	}
	return false
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
	for line := range strings.SplitSeq(strings.TrimRight(raw, " \n"), "\n") {
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
