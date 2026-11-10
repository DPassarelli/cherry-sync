// Driving csync through a real terminal: the pseudo-terminal it runs under and the
// emulated screen a scenario reads back. The picker only appears when both stdin
// and stdout are terminals, so piped runs can never reach it.

package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// terminalRows and terminalCols fix the terminal's size, so that what fits on
// screen doesn't depend on who runs the suite.
const (
	terminalRows = 24
	terminalCols = 100
)

// terminalWait bounds how long a step waits for the screen to show what it expects.
// It is a deadlock guard, not a timing assumption: the wait ends as soon as the
// screen matches, so a slow machine costs nothing.
const terminalWait = 20 * time.Second

// settleWindow is how long the screen must stay unchanged before a step treats a
// keypress as fully handled. Checking that something did not happen has no event to
// wait for, so a quiet screen is the only signal available.
const settleWindow = 100 * time.Millisecond

// terminalKeys maps the key names scenarios use to the bytes a terminal sends.
var terminalKeys = map[string]string{
	"a":      "a",
	"ctrl-c": "\x03",
	"d":      "d",
	"down":   "\x1b[B",
	"enter":  "\r",
	"esc":    "\x1b",
	"space":  " ",
	"up":     "\x1b[A",
}

// terminalSessionKey stashes the scenario's running terminalSession.
type terminalSessionKey struct{}

// terminalSession is a csync child attached to a pseudo-terminal, with the screen
// it has drawn so far. readerDone closes once the child's output is exhausted, and
// exited once the child has been reaped.
type terminalSession struct {
	cmd        *exec.Cmd
	ptmx       *os.File
	screen     vt10x.Terminal
	readerDone chan struct{}
	exited     chan struct{}
}

// startTerminalSession starts csync with args under a pseudo-terminal and begins
// feeding its output to the emulated screen.
func startTerminalSession(ctx context.Context, args []string) (*terminalSession, error) {
	cmd := exec.Command(csyncBinary, args...)
	// The styling library disables all text attributes when these are set, even
	// under a real terminal, and CI runners set CI. Scenarios that read attributes
	// back from the screen must not depend on who runs the suite.
	env := withoutVars(csyncEnv(ctx), "CI", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE")
	cmd.Env = append(env, "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: terminalRows, Cols: terminalCols})
	if err != nil {
		return nil, fmt.Errorf("start under a pty: %w", err)
	}
	session := &terminalSession{
		cmd:        cmd,
		ptmx:       ptmx,
		screen:     vt10x.New(vt10x.WithSize(terminalCols, terminalRows), vt10x.WithWriter(ptmx)),
		readerDone: make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go func() {
		defer close(session.exited)
		_ = cmd.Wait()
	}()
	go func() {
		defer close(session.readerDone)
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				_, _ = session.screen.Write(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}()
	return session, nil
}

// press sends the named key to csync.
func (s *terminalSession) press(key string) error {
	seq, ok := terminalKeys[key]
	if !ok {
		return fmt.Errorf("no terminal sequence known for key %q", key)
	}
	_, err := s.ptmx.Write([]byte(seq))
	return err
}

// waitFor polls the screen until ok accepts it, naming what it wanted and showing
// the final screen if it never does.
func (s *terminalSession) waitFor(want string, ok func(TerminalScreen) bool) (TerminalScreen, error) {
	deadline := time.Now().Add(terminalWait)
	for {
		screen := parseTerminalScreen(s.screen.String())
		if ok(screen) {
			return screen, nil
		}
		if time.Now().After(deadline) {
			return screen, fmt.Errorf("screen never showed %s within %s; screen was:\n%s", want, terminalWait, screen)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// cursorRow returns the cells of the row carrying the picker's caret, or nil while
// no such row is on screen.
func (s *terminalSession) cursorRow() []vt10x.Glyph {
	s.screen.Lock()
	defer s.screen.Unlock()
	for y := 0; y < terminalRows; y++ {
		row := make([]vt10x.Glyph, terminalCols)
		found := false
		for x := range row {
			row[x] = s.screen.Cell(x, y)
			found = found || row[x].Char == '❯'
		}
		if found {
			return row
		}
	}
	return nil
}

// rowBeginning returns the cells of the first row whose text opens with prefix, or
// nil while no such row is on screen.
func (s *terminalSession) rowBeginning(prefix string) []vt10x.Glyph {
	s.screen.Lock()
	defer s.screen.Unlock()
	for y := 0; y < terminalRows; y++ {
		row := make([]vt10x.Glyph, terminalCols)
		for x := range row {
			row[x] = s.screen.Cell(x, y)
		}
		text, _ := unframe(glyphText(row))
		if strings.HasPrefix(text, prefix) {
			return row
		}
	}
	return nil
}

// rowContaining returns the cells of the first row whose text holds text, or nil
// while no such row is on screen.
func (s *terminalSession) rowContaining(text string) []vt10x.Glyph {
	s.screen.Lock()
	defer s.screen.Unlock()
	for y := 0; y < terminalRows; y++ {
		row := make([]vt10x.Glyph, terminalCols)
		for x := range row {
			row[x] = s.screen.Cell(x, y)
		}
		if strings.Contains(glyphText(row), text) {
			return row
		}
	}
	return nil
}

// settle waits for the screen to stop changing, and returns it.
func (s *terminalSession) settle() (TerminalScreen, error) {
	deadline := time.Now().Add(terminalWait)
	last := s.screen.String()
	quietSince := time.Now()
	for time.Since(quietSince) < settleWindow {
		if time.Now().After(deadline) {
			return parseTerminalScreen(last), fmt.Errorf("screen never settled within %s", terminalWait)
		}
		time.Sleep(10 * time.Millisecond)
		current := s.screen.String()
		if current != last {
			last = current
			quietSince = time.Now()
		}
	}
	return parseTerminalScreen(last), nil
}

// close stops csync and releases the terminal, so a failed scenario can't leave a
// child waiting for keys that will never come.
func (s *terminalSession) close() {
	_ = s.cmd.Process.Kill()
	<-s.exited
	_ = s.ptmx.Close()
	<-s.readerDone
}
