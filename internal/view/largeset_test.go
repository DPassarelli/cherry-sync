package view

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// applyGateKey sends one keypress to the gate model and returns both the updated
// model and whatever command it issued, since the gate's whole behaviour is a
// decision plus a quit.
func applyGateKey(m largeSetModel, key tea.KeyMsg) (largeSetModel, tea.Cmd) {
	updated, cmd := m.Update(key)
	return updated.(largeSetModel), cmd
}

// TestLargeSetWarning pins the warning's wording against the mockup in issue #61.
// It must name the limit rather than the actual count: the point is that csync has
// a stated design boundary, not that this particular run happens to exceed it.
func TestLargeSetWarning(t *testing.T) {
	got := largeSetWarning()
	want := "There are more than 200 files that need to be synced. This program is not designed for working with large file sets."
	if got != want {
		t.Errorf("largeSetWarning() = %q, want %q", got, want)
	}
}

// TestLargeSetWarningTracksTheLimit guards against the two drifting apart: change
// LargeSetLimit and the sentence must follow, or the warning states a boundary
// csync no longer enforces.
func TestLargeSetWarningTracksTheLimit(t *testing.T) {
	if !strings.Contains(largeSetWarning(), "200") {
		t.Fatalf("this test is stale: it assumes LargeSetLimit is 200, but the warning reads %q", largeSetWarning())
	}
	if LargeSetLimit != 200 {
		t.Errorf("LargeSetLimit = %d, but the warning still says 200", LargeSetLimit)
	}
}

// TestGateEnterProceeds pins the one way forward: enter accepts the whole set and
// ends the gate.
func TestGateEnterProceeds(t *testing.T) {
	m, cmd := applyGateKey(largeSetModel{}, tea.KeyMsg{Type: tea.KeyEnter})

	if !m.proceed {
		t.Error("proceed = false after enter, want true")
	}
	if !isQuit(cmd) {
		t.Error("enter did not end the gate")
	}
}

// TestGateCtrlCCancels pins the other: ctrl-c ends the gate without proceeding, so
// main treats it as a cancelled run.
func TestGateCtrlCCancels(t *testing.T) {
	m, cmd := applyGateKey(largeSetModel{}, tea.KeyMsg{Type: tea.KeyCtrlC})

	if m.proceed {
		t.Error("proceed = true after ctrl-c, want false")
	}
	if !isQuit(cmd) {
		t.Error("ctrl-c did not end the gate")
	}
}

// TestGateIgnoresEverythingElse keeps the gate deliberate. Syncing a set this large
// is exactly what the warning exists to make the user think about, so no key but
// the two it advertises may end it — least of all the picker's own 'a' and space.
func TestGateIgnoresEverythingElse(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		runeKey('a'),
		runeKey('q'),
		runeKey('y'),
		{Type: tea.KeySpace},
		{Type: tea.KeyEsc},
		{Type: tea.KeyDown},
	} {
		m, cmd := applyGateKey(largeSetModel{}, key)
		if m.proceed {
			t.Errorf("%q set proceed = true", key.String())
		}
		if isQuit(cmd) {
			t.Errorf("%q ended the gate; only enter and ctrl-c may", key.String())
		}
	}
}

// TestGateViewShowsBothLines checks the gate leaves its warning on screen. Unlike
// the spinner, which erases itself so the picker can draw in the row it vacated,
// nothing follows the gate — so the frame it drew is what the user is left looking
// at, and both the warning and the keys that answer it have to be in it.
func TestGateViewShowsBothLines(t *testing.T) {
	got := largeSetModel{}.View()

	if !strings.Contains(got, largeSetWarning()) {
		t.Errorf("View() = %q, want it to contain the warning", got)
	}
	if !strings.Contains(got, "enter") || !strings.Contains(got, "ctrl-c") {
		t.Errorf("View() = %q, want it to name both keys", got)
	}
}

// TestGateWrapsToTerminalWidth is the test a real terminal forced: Bubble Tea
// truncates every frame line to the terminal width rather than wrapping it, so an
// unwrapped warning loses its tail on an 80-column screen — the half that names the
// limitation. Each line the gate emits must fit.
func TestGateWrapsToTerminalWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100} {
		for _, line := range largeSetLines(width) {
			if lipgloss.Width(line) > width {
				t.Errorf("at width %d, line %q occupies %d cells", width, line, lipgloss.Width(line))
			}
		}
	}
}

// TestGateWrapKeepsEveryWord checks the wrapping drops nothing: the warning is a
// statement about what csync will not do, and a wrap that swallowed a clause would
// leave a sentence that still reads as English but no longer says it.
func TestGateWrapKeepsEveryWord(t *testing.T) {
	joined := strings.Join(strings.Fields(strings.Join(largeSetLines(40), " ")), " ")
	for _, word := range strings.Fields(largeSetWarning()) {
		if !strings.Contains(joined, word) {
			t.Errorf("wrapping at width 40 lost %q; got %q", word, joined)
		}
	}
}

// TestGateUnknownWidthStillRenders covers the frame Bubble Tea may ask for before
// the first WindowSizeMsg arrives, when the gate has no width to wrap to.
func TestGateUnknownWidthStillRenders(t *testing.T) {
	lines := largeSetLines(0)
	if len(lines) == 0 {
		t.Fatal("largeSetLines(0) returned nothing")
	}
	if !strings.Contains(strings.Join(lines, " "), "large file sets") {
		t.Errorf("largeSetLines(0) = %q, want the unwrapped warning", lines)
	}
}

// TestGateLearnsTerminalWidth checks the gate takes the width Bubble Tea hands it,
// which is the only way it can know what to wrap to.
func TestGateLearnsTerminalWidth(t *testing.T) {
	updated, _ := largeSetModel{}.Update(tea.WindowSizeMsg{Width: 72, Height: 24})

	m, ok := updated.(largeSetModel)
	if !ok {
		t.Fatalf("Update returned %T, want largeSetModel", updated)
	}
	if m.width != 72 {
		t.Errorf("width = %d, want 72", m.width)
	}
}
