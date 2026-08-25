// spinner.go renders the wait before the picker: an animated spinner captioned
// with the stage of the comparison that is currently running, how long it has
// been running, and — once the wait is long enough to want out of — the key that
// stops it. It occupies the rows the picker's prompt will later fill, and blanks
// its own last frame when the comparison finishes so the picker can draw in the
// slot it vacated.

package view

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

// spinnerFrames is a braille cycle: every frame is one cell wide in every font
// that has the glyphs, so the caption beside it never shifts left or right as it
// turns.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerInterval is the delay between frames — fast enough to read as motion,
// slow enough not to shimmer.
const spinnerInterval = 80 * time.Millisecond

// spinnerDelay is how long the comparison is given to finish before the spinner
// appears at all. Most comparisons are quick, and a spinner that flashes up and
// vanishes inside a couple hundred milliseconds reads as a glitch rather than as
// reassurance; the freeze this exists to explain (#62) only registers past about
// a second.
const spinnerDelay = 200 * time.Millisecond

// spinnerHintDelay is how long the comparison runs before the spinner also
// names the key that stops it. A wait that has gone on this long is one the
// user may want out of; below it the hint is noise on a screen that is about
// to change anyway.
const spinnerHintDelay = 3 * time.Second

// spinnerHint names the only key the spinner answers to, in the picker's own
// hint format so the two read as parts of the same program.
const spinnerHint = "ctrl-c to cancel"

// frameMsg advances the spinner to its next frame.
type frameMsg time.Time

// revealMsg is delivered once spinnerDelay has elapsed and marks the point at
// which a comparison has run long enough to be worth showing a spinner for.
type revealMsg struct{}

// phaseMsg carries the name of the comparison stage now running, which becomes
// the spinner's caption.
type phaseMsg string

// resultMsg delivers the finished comparison, and is what ends the program.
type resultMsg struct {
	result compare.Result
	err    error
}

// spinnerModel animates a spinner in the row the picker's prompt will later
// occupy, captioned with whichever stage of the comparison is running. It holds
// the comparison's outcome once it arrives, so RunSpinner can hand it back.
type spinnerModel struct {
	frame   int
	caption string
	shown   bool
	start   time.Time
	elapsed time.Duration
	phases  <-chan string
	work    func() (compare.Result, error)
	cancel  context.CancelFunc

	result    compare.Result
	err       error
	cancelled bool
	done      bool
}

// Init starts the frame timer, arms the reveal delay, launches the comparison,
// and begins listening for stage names — the four things that run concurrently
// for as long as the spinner is up.
func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		tea.Tick(spinnerDelay, func(time.Time) tea.Msg { return revealMsg{} }),
		func() tea.Msg {
			result, err := m.work()
			return resultMsg{result: result, err: err}
		},
		waitPhase(m.phases),
	)
}

// tick schedules the next frame of the animation.
func tick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(t time.Time) tea.Msg { return frameMsg(t) })
}

// waitPhase blocks on the next stage name and delivers it as a message. Update
// re-issues it after each one, which is what turns a channel into a stream of
// messages Bubble Tea can interleave with its own.
func waitPhase(phases <-chan string) tea.Cmd {
	return func() tea.Msg {
		caption, ok := <-phases
		if !ok {
			return nil
		}
		return phaseMsg(caption)
	}
}

// Update handles one message: a frame advance, the reveal, a new caption, the
// finished comparison, or the one keypress the spinner honors.
func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case frameMsg:
		m.frame = (m.frame + 1) % len(spinnerFrames)
		// The tick's own timestamp is the clock. Taking elapsed from the message
		// rather than from time.Now() is what lets a test drive the wait forward by
		// handing the model frames, with nothing to sleep on and no clock to inject.
		m.elapsed = time.Time(msg).Sub(m.start)
		return m, tick()
	case revealMsg:
		m.shown = true
		return m, nil
	case phaseMsg:
		m.caption = string(msg)
		return m, waitPhase(m.phases)
	case resultMsg:
		m.result, m.err = msg.result, msg.err
		// done blanks the final frame, which is what actually clears the spinner off
		// the screen: Bubble Tea erases the rows it drew, but only the rows the last
		// frame still claims. Leave the caption in that frame and it stays put above
		// whatever draws next, stale elapsed time and all.
		m.done = true
		return m, tea.Quit
	case tea.KeyMsg:
		// Ctrl-C is the only key the spinner answers to. Bubble Tea has the terminal
		// in raw mode while the program runs, so the keystroke arrives here as a
		// message instead of becoming a SIGINT — cancelling the context is what
		// passes it on to the rsync that is still running.
		if msg.String() == "ctrl+c" {
			m.cancelled = true
			m.cancel()
			return m, tea.Quit
		}
	}
	return m, nil
}

// View draws the spinner, or nothing at all — before the reveal delay has passed,
// and again once the work is done. Returning an empty final frame is what makes
// the handoff work: Bubble Tea erases the rows it drew, leaving the row free for
// the exclusion line and the picker that follow. That erase covers the hint row
// too, so the frame is free to grow taller partway through a long wait.
func (m spinnerModel) View() string {
	if !m.shown || m.done || m.err != nil || m.cancelled || m.caption == "" {
		return ""
	}
	dim := lipgloss.NewStyle().Faint(true)

	caption := m.caption
	// Under a second there is no elapsed time worth reporting, and a "(0s)" sitting
	// there for the better part of a second reads as a stuck counter rather than a
	// running one. Seconds are the only unit: an unpadded count is unambiguous at
	// any length, where a clock format invites reading "1:07" as an ETA.
	if m.elapsed >= time.Second {
		caption += fmt.Sprintf(" (%ds)", int(m.elapsed.Seconds()))
	}

	out := "\n" + spinnerFrames[m.frame] + " " + dim.Render(caption) + "\n"
	if m.elapsed >= spinnerHintDelay {
		// hintIndent is the picker's own, so every key hint csync prints — the
		// picker's, the large-set gate's, and this one — sits in the same column.
		out += hintIndent + dim.Render(spinnerHint) + "\n"
	}
	return out
}

// Comparison is what RunSpinner observed: the comparison's own result, and
// whether the user gave up on it with Ctrl-C before it finished. The two travel
// together because a cancelled run carries no meaningful result and the caller
// must check the one before reading the other.
type Comparison struct {
	Result    compare.Result
	Cancelled bool
}

// RunSpinner performs work while showing a spinner in the slot the picker's
// prompt will occupy, captioning it with each stage name that arrives on phases.
// It drives a Bubble Tea program and so needs a terminal: main calls it only on
// an interactive run, and compares directly otherwise.
func RunSpinner(cancel context.CancelFunc, phases <-chan string, work func() (compare.Result, error)) (Comparison, error) {
	m := spinnerModel{phases: phases, work: work, cancel: cancel, start: time.Now()}
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return Comparison{}, err
	}
	done, ok := final.(spinnerModel)
	if !ok {
		return Comparison{}, nil
	}
	return Comparison{Result: done.result, Cancelled: done.cancelled}, done.err
}
