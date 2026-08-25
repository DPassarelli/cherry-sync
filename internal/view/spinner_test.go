package view

import (
	"strings"
	"testing"
	"time"
)

// visibleSpinner is a spinner past its reveal delay with a caption already in
// hand, which is the only state whose View draws anything — every assertion below
// starts from it and varies nothing but how long the wait has run.
func visibleSpinner(elapsed time.Duration) spinnerModel {
	return spinnerModel{shown: true, caption: "comparing files", elapsed: elapsed}
}

func TestSpinnerHidesElapsedUnderASecond(t *testing.T) {
	got := visibleSpinner(900 * time.Millisecond).View()
	if !strings.Contains(got, "comparing files") {
		t.Fatalf("caption missing from %q", got)
	}
	if strings.Contains(got, "(") {
		t.Errorf("under a second the wait should carry no count, got %q", got)
	}
}

func TestSpinnerCountsWholeSeconds(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		want    string
	}{
		{time.Second, "(1s)"},
		{1900 * time.Millisecond, "(1s)"},
		{4 * time.Second, "(4s)"},
		{97 * time.Second, "(97s)"},
	} {
		got := visibleSpinner(tc.elapsed).View()
		if !strings.Contains(got, tc.want) {
			t.Errorf("after %s want %s in %q", tc.elapsed, tc.want, got)
		}
	}
}

func TestSpinnerWithholdsHintUntilTheDelay(t *testing.T) {
	early := visibleSpinner(spinnerHintDelay - time.Millisecond).View()
	if strings.Contains(early, spinnerHint) {
		t.Errorf("hint arrived before %s: %q", spinnerHintDelay, early)
	}
	late := visibleSpinner(spinnerHintDelay).View()
	if !strings.Contains(late, spinnerHint) {
		t.Errorf("hint missing at %s: %q", spinnerHintDelay, late)
	}
}

// The hint has to land in the column every other key hint csync prints lands in,
// or the spinner reads as a different program than the picker it hands off to.
func TestSpinnerHintSitsInThePickerColumn(t *testing.T) {
	for _, line := range strings.Split(visibleSpinner(10*time.Second).View(), "\n") {
		if strings.Contains(line, spinnerHint) && !strings.HasPrefix(line, hintIndent+spinnerHint) {
			t.Errorf("hint line %q is not indented like the picker's", line)
		}
	}
}

// The tick's timestamp is the spinner's only clock, so a wait advances in a test
// exactly the way it does in a terminal: by frames arriving.
func TestSpinnerTakesElapsedFromTheTick(t *testing.T) {
	start := time.Now()
	m := spinnerModel{shown: true, caption: "comparing files", start: start}
	updated, _ := m.Update(frameMsg(start.Add(7 * time.Second)))
	got := updated.(spinnerModel).View()
	if !strings.Contains(got, "(7s)") || !strings.Contains(got, spinnerHint) {
		t.Errorf("after seven seconds of frames, got %q", got)
	}
}

// A blank final frame is what clears the spinner off the screen; leave anything
// in it and the stale caption and hint sit above the picker that draws next.
func TestSpinnerFinalFrameIsBlank(t *testing.T) {
	m := visibleSpinner(10 * time.Second)
	updated, cmd := m.Update(resultMsg{})
	if !isQuit(cmd) {
		t.Fatal("the finished comparison should end the spinner")
	}
	if got := updated.(spinnerModel).View(); got != "" {
		t.Errorf("final frame should be empty, got %q", got)
	}
}
