// pickerview.go renders the picker: the frame View draws each update, the change
// rows and their styling, and the prompt and scroll indicators around them.

package view

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/selection"
)

// cursorBG is the background bar drawn across the cursor's row. AdaptiveColor keeps
// it legible on both light and dark terminals; it pairs with the caret, which
// carries the cursor cue on terminals where the bar is hard to see.
var cursorBG = lipgloss.AdaptiveColor{Light: "252", Dark: "236"}

// View renders the picker: a directions header that stays fixed, then the changes
// grouped by directory, scrolled so the cursor's row is always visible. The window
// is always bracketed by an indicator line top and bottom — an arrow where content
// is hidden on that side, a blank where it isn't. Rendering both lines
// unconditionally keeps a fixed blank line under the header and stops the list from
// jumping when an arrow appears or disappears. The list itself is built by
// contentLines; the scroll math and the cursor-line mapping are pinned by tests, so
// only the styling is untested.
func (m pickerModel) View() string {
	prompt := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	// A leading blank line separates the picker from the Source/Destination header
	// main prints above it; the bold "? " prompt and dimmed directions mirror the
	// mockup (and npm-check-updates' "?"-led question). This block is pickerHeaderLines
	// tall and is held out of the scroll region so it never scrolls away.
	out := []string{
		"",
		"? " + prompt.Render(pickerPrompt(len(m.actions))),
		"   " + dim.Render("↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel"),
	}

	vp := m.visible()
	// The "above" indicator line doubles as the fixed blank under the header when
	// nothing is hidden above, so the prompt-to-list gap never changes height.
	out = append(out, scrollIndicator(dim, "▲ more above", vp.HiddenAbove))
	out = append(out, vp.Lines...)
	out = append(out, scrollIndicator(dim, "▼ more below", vp.HiddenBelow))
	return strings.Join(out, "\n") + "\n"
}

// contentLines renders the change list — the per-directory headings and their rows —
// into one string per line, and returns the index of the cursor's row within them.
// It is the unwindowed list: View slices it through scroll. A blank line separates
// each group from the one before, but the first heading has none — the fixed "above"
// indicator line View draws is the gap under the header, and a leading blank here
// would scroll away and shift the list. A single flat index walks the actions in
// display order; GroupByDir preserves that order, so the index matches the cursor
// and the Selection one-for-one. Each row is a cursor marker, a "[x]"/"[ ]"
// checkbox, the basename, and the verb; a checked row is verb-colored, an unchecked
// row dimmed, and the cursor's row carries the caret and a background bar across all
// segments.
func (m pickerModel) contentLines() ([]string, int) {
	dim := lipgloss.NewStyle().Faint(true)
	dirHeading := lipgloss.NewStyle().Bold(true)

	// Align the verb column by padding every basename to the widest one.
	// lipgloss.Width measures display cells, so multi-byte names line up too.
	width := 0
	for _, a := range m.actions {
		w := lipgloss.Width(path.Base(a.Path))
		if w > width {
			width = w
		}
	}

	var lines []string
	cursorLine := 0
	flat := 0
	for _, g := range selection.GroupByDir(m.actions) {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, dirHeading.Render(g.Dir))
		for _, a := range g.Actions {
			cursorRow := flat == m.cursor
			checked := m.sel.IsChecked(flat)
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			base := path.Base(a.Path)
			pad := strings.Repeat(" ", width-lipgloss.Width(base))
			textStyle := dim
			if checked {
				textStyle = verbStyle(a.Verb)
			}
			caretStyle := lipgloss.NewStyle().Bold(true)
			boxStyle := lipgloss.NewStyle()
			if cursorRow {
				textStyle = textStyle.Background(cursorBG)
				caretStyle = caretStyle.Background(cursorBG)
				boxStyle = boxStyle.Background(cursorBG)
			}
			marker := "  "
			if cursorRow {
				marker = caretStyle.Render("❯ ")
			}
			row := fmt.Sprintf("%s%s%s", marker, boxStyle.Render(box+" "), textStyle.Render(base+pad+"  "+a.Verb))
			// The annotation is shown for the cursor's row alone. It runs to a sentence
			// now that it reports both copies, and repeating that on every row would
			// bury the filenames the list exists to be scanned by — so it reads as a
			// detail pane attached to the selection rather than as a column.
			if cursorRow {
				detailStyle := lipgloss.NewStyle().Background(cursorBG)
				detail := fitDetail(actionDetail(a, m.now), lipgloss.Width(row), m.width)
				if detail != "" {
					row += detailStyle.Render(detailGap + detail)
				}
			}
			lines = append(lines, row)
			if cursorRow {
				cursorLine = len(lines) - 1
			}
			flat++
		}
	}
	return lines, cursorLine
}

// pickerPrompt returns the picker's question line for a list of count files. It
// names the count so a list that renders and then waits for input reads as "these
// are all of them," not a freeze or a bug (issue #64). The count is the plain total
// on offer, not the currently-checked subset, and the phrasing is not pluralized —
// a lone file reads "(1 available)". Styling is applied by the caller; this returns
// plain text so it can be pinned by a test without a terminal.
func pickerPrompt(count int) string {
	return fmt.Sprintf("Choose which files to sync (%d available):", count)
}

// scrollIndicator renders one bracket line of the scroll window: the dimmed label
// (e.g. "▲ more above") when hidden rows exist on that side, or an empty line when
// none do. The blank holds the row so the frame's height doesn't change as the
// cursor crosses the list's ends.
func scrollIndicator(style lipgloss.Style, label string, hidden int) string {
	if hidden == 0 {
		return ""
	}
	return "  " + style.Render(label)
}

// verbStyle returns the lipgloss style that colors a change by its verb: green for
// create, yellow for update, red for delete. An unknown verb renders unstyled.
func verbStyle(verb string) lipgloss.Style {
	switch verb {
	case "create":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case "update":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case "delete":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	default:
		return lipgloss.NewStyle()
	}
}
