package view

import (
	"strings"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
	"github.com/dpassarelli/cherry-sync/internal/filediff"
)

// A file with a copy on only one side has nothing to compare, and every such notice
// opens the same way so it reads as the same kind of answer.
func TestNoDiffNotice_SharesItsPrefix(t *testing.T) {
	for _, verb := range []string{"create", "delete"} {
		notice := noDiffNotice(verb)
		if !strings.HasPrefix(notice, "no diff: ") {
			t.Errorf("noDiffNotice(%q) = %q, want the \"no diff: \" prefix", verb, notice)
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

// The renderer cuts off any line wider than the terminal, so a long notice must wrap
// rather than lose the words at its end.
func TestView_LongNoticeWrapsWithinTheTerminal(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "delete", Path: "README.md"}})
	m.width, m.height = 40, 24
	m = applyKey(m, runeKey('d'))
	flat := strings.Join(strings.Fields(m.View()), " ")
	if !strings.Contains(flat, "if selected") {
		t.Errorf("notice lost its ending at width 40:\n%s", m.View())
	}
}

// The rows a wrapped notice takes come out of the list, or the frame would outgrow
// the terminal and push the preamble off the top.
func TestScrollHeight_ReservesTheNoticeRows(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "delete", Path: "README.md"}})
	m.width, m.height = 40, 24
	without := m.scrollHeight()
	m = applyKey(m, runeKey('d'))
	if m.scrollHeight() >= without {
		t.Errorf("scrollHeight = %d with a notice, want less than %d", m.scrollHeight(), without)
	}
}

// Pressing "d" again during a fetch starts no second one: fetches write to the run
// log, and overlapping them would interleave its records.
func TestUpdate_DWhileFetchingStartsNoSecondFetch(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.fetchDiff = func(string) (filediff.Result, error) { return filediff.Result{}, nil }
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
// top off screen and the overlay no longer begins with its header.
func TestView_OverlayFitsTheTerminal(t *testing.T) {
	m := newModel([]compare.Action{{Verb: "update", Path: "README.md"}})
	m.width, m.height = 80, 5
	updated, _ := m.Update(diffFetchedMsg{text: strings.Repeat("line\n", 50)})
	got := countRows(updated.View(), 80)
	if got > 5 {
		t.Errorf("overlay is %d rows tall, want at most 5", got)
	}
}

// Every notice about a fetched file shares the prefix too.
func TestFetchedNotice_SharesThePrefix(t *testing.T) {
	results := []filediff.Result{
		{Oversized: filediff.SourceCopy},
		{Binary: filediff.DestinationCopy},
	}
	for _, r := range results {
		notice := fetchedNotice(r)
		if !strings.HasPrefix(notice, "no diff: ") {
			t.Errorf("fetchedNotice(%+v) = %q, want the \"no diff: \" prefix", r, notice)
		}
	}
}

// An oversized copy is never read, so whether it is binary is unknown, and the size
// is the only thing that can be said for certain about the pair.
func TestFetchedNotice_SizeOutranksBinary(t *testing.T) {
	notice := fetchedNotice(filediff.Result{Oversized: filediff.SourceCopy, Binary: filediff.DestinationCopy})
	if !strings.Contains(notice, "over the 200 KiB limit") {
		t.Errorf("fetchedNotice = %q, want the size notice", notice)
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
	m.height = 8
	got := countRows(m.overlayView(), 80)
	if got != 7 {
		t.Errorf("overlay is %d rows tall, want 7", got)
	}
}
