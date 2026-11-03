package view

import (
	"strings"
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/compare"
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
	m.fetchDiff = func(string) (string, error) { return "", nil }
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
