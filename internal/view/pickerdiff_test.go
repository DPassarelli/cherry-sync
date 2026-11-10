package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// A file with a copy on only one side has nothing to compare, and every such notice
// opens the same way so it reads as the same kind of answer.
func TestNoDiffNotice_SharesItsPrefix(t *testing.T) {
	for _, verb := range []string{"create", "delete"} {
		notice := noDiffNotice(verb)
		if !strings.HasPrefix(notice, "cannot diff: ") {
			t.Errorf("noDiffNotice(%q) = %q, want the \"cannot diff: \" prefix", verb, notice)
		}
	}
}

// An update has two copies to compare, so it gets no notice.
func TestNoDiffNotice_UpdateHasNone(t *testing.T) {
	notice := noDiffNotice("update")
	if notice != "" {
		t.Errorf("noDiffNotice(update) = %q, want none", notice)
	}
}

// The renderer cuts off any line wider than the terminal, so the diff hint and its
// note are cut to fit rather than left to the renderer, which would wrap them onto
// a row the list doesn't leave room for.
func TestDiffHintLine_FitsTheTerminal(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "delete", Path: "README.md"}})
	m.width, m.height = 40, 24
	m = applyKey(m, runeKey('d'))
	got := ansi.StringWidth(m.diffHintLine())
	if got > 40 {
		t.Errorf("diff hint line is %d columns wide, want at most 40", got)
	}
}

// csync assumes a terminal at least 80 columns wide, and at that width even the
// longest note has to be read in full.
func TestDiffHintLine_LongestNoteFitsEightyColumns(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "create", Path: "README.md"}})
	m.width, m.height = 80, 24
	m = applyKey(m, runeKey('d'))
	if !strings.HasSuffix(ansi.Strip(m.diffHintLine()), noDiffNotice("create")) {
		t.Errorf("note cut short at 80 columns: %q", ansi.Strip(m.diffHintLine()))
	}
}

// A note sits on the diff hint's own row, so it takes no rows from the list and the
// list doesn't jump when one comes and goes.
func TestScrollHeight_UnchangedByANote(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "delete", Path: "README.md"}})
	m.width, m.height = 40, 24
	without := m.scrollHeight()
	m = applyKey(m, runeKey('d'))
	if m.notice == "" || m.scrollHeight() != without {
		t.Errorf("scrollHeight = %d with note %q, want %d", m.scrollHeight(), m.notice, without)
	}
}

// Pressing "d" again during a fetch starts no second one: fetches write to the run
// log, and overlapping them would interleave its records.
func TestUpdate_DWhileFetchingStartsNoSecondFetch(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.differ.Fetch = func(string) (filediff.Result, error) { return filediff.Result{}, nil }
	updated, first := m.Update(runeKey('d'))
	if first == nil {
		t.Fatal("first press started no fetch")
	}
	_, second := updated.Update(runeKey('d'))
	if second != nil {
		t.Error("second press started another fetch")
	}
}

// A taller diff than the terminal must be cut to fit, or the renderer scrolls its
// top off screen and the overlay no longer begins with its title.
func TestView_OverlayFitsTheTerminal(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 12
	updated, _ := m.Update(diffFetchedMsg{text: strings.Repeat("line\n", 50)})
	got := countRows(updated.View(), 80)
	if got > 12 {
		t.Errorf("overlay is %d rows tall, want at most 12", got)
	}
}

// Every notice about a fetched file shares the prefix too.
func TestFetchedNotice_SharesThePrefix(t *testing.T) {
	results := []filediff.Result{
		{Oversized: filediff.SourceCopy},
		{Oversized: filediff.DestinationCopy},
		{Binary: filediff.DestinationCopy},
	}
	for _, r := range results {
		notice := fetchedNotice(r)
		if !strings.HasPrefix(notice, "cannot diff: ") {
			t.Errorf("fetchedNotice(%+v) = %q, want the \"cannot diff: \" prefix", r, notice)
		}
	}
}

// A result with something to show has no notice, so it opens the overlay instead.
func TestFetchedNotice_NoneForADiff(t *testing.T) {
	notice := fetchedNotice(filediff.Result{Text: "--- a\n+++ b\n"})
	if notice != "" {
		t.Errorf("fetchedNotice = %q, want none", notice)
	}
}

// "j" and "k" move the picker's cursor, so they scroll the diff the same way the
// arrows do rather than doing something different on this one screen.
func TestOverlayKey_JAndKScroll(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 5
	updated, _ := m.Update(diffFetchedMsg{text: strings.Repeat("line\n", 50)})
	updated, _ = updated.Update(runeKey('j'))
	if updated.(pickerModel).overlayOffset != 1 {
		t.Fatalf("offset after j = %d, want 1", updated.(pickerModel).overlayOffset)
	}
	updated, _ = updated.Update(runeKey('k'))
	if updated.(pickerModel).overlayOffset != 0 {
		t.Errorf("offset after k = %d, want 0", updated.(pickerModel).overlayOffset)
	}
}

// A second diff starts at its own top, not where the last one was left.
func TestOpenOverlay_StartsAtTheTop(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 5
	m.overlayOffset = 7
	updated, _ := m.Update(diffFetchedMsg{text: "a\nb\n"})
	if updated.(pickerModel).overlayOffset != 0 {
		t.Errorf("offset = %d, want 0", updated.(pickerModel).overlayOffset)
	}
}

// Growing the terminal while scrolled to the end can't leave blank rows under the diff.
func TestOverlayView_ClampsAfterAResize(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 5
	updated, _ := m.Update(diffFetchedMsg{text: strings.Repeat("line\n", 10)})
	m = updated.(pickerModel)
	m.overlayOffset = 6
	m.height = 15
	got := countRows(m.overlayView(), 80)
	if got != 14 {
		t.Errorf("overlay is %d rows tall, want 14", got)
	}
}

// A short diff still puts the hint on the bottom row, where a full one leaves it, so
// it doesn't jump between diffs.
func TestOverlayView_KeepsTheHintOnTheBottomRow(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 12
	updated, _ := m.Update(diffFetchedMsg{text: "a\nb\n"})
	got := countRows(updated.View(), 80)
	if got != 11 {
		t.Errorf("overlay is %d rows tall, want 11", got)
	}
}

// Opening the overlay clears the progress, or it would still be there after
// the overlay closes.
func TestOpenOverlay_ClearsTheProgressNotice(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.differ.Fetch = func(string) (filediff.Result, error) { return filediff.Result{}, nil }
	m, _ = m.pressDiff()
	updated, _ := m.Update(fetchRevealMsg{fetch: m.fetchID})
	updated, _ = updated.Update(diffFetchedMsg{text: "a\n"})
	if updated.(pickerModel).progress != "" {
		t.Errorf("progress = %q after the overlay opened, want none", updated.(pickerModel).progress)
	}
}

// A reveal timed for a fetch that has already finished must not raise the indicator
// over the next one before its own delay is up.
func TestFetchReveal_IgnoresAFinishedFetch(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.differ.Fetch = func(string) (filediff.Result, error) { return filediff.Result{}, nil }
	m, _ = m.pressDiff()
	stale := m.fetchID
	updated, _ := m.Update(diffFetchedMsg{text: "a\n"})
	m = updated.(pickerModel)
	m.overlay = nil
	m, _ = m.pressDiff()
	updated, _ = m.Update(fetchRevealMsg{fetch: stale})
	if updated.(pickerModel).progress != "" {
		t.Errorf("progress = %q from a finished fetch's reveal, want none", updated.(pickerModel).progress)
	}
}
