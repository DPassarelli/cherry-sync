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

// diffHeaderPrefix opens the first line of a diff overlay.
const diffHeaderPrefix = "--- destination: "

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

// TerminalScreen is what the terminal is showing: its lines with trailing padding
// removed, and the picker's rows when any are on screen.
type TerminalScreen struct {
	Lines []string
	Rows  []PickerRow
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
	return len(s.Lines) > 0 && strings.HasPrefix(s.Lines[0], diffHeaderPrefix)
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
	for line := range strings.SplitSeq(strings.TrimRight(raw, " \n"), "\n") {
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
	return screen
}
