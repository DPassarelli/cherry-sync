// Driving csync through a real terminal: the pseudo-terminal it runs under and the
// emulated screen a scenario reads back. The picker only appears when both stdin
// and stdout are terminals, so piped runs can never reach it.

package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// terminalKeys maps the key names scenarios use to the bytes a terminal sends.
var terminalKeys = map[string]string{
	"d":    "d",
	"down": "\x1b[B",
	"up":   "\x1b[A",
}

// terminalSessionKey stashes the scenario's running terminalSession.
type terminalSessionKey struct{}

// terminalSession is a csync child attached to a pseudo-terminal, with the screen
// it has drawn so far. readerDone closes once the child's output is exhausted.
type terminalSession struct {
	cmd        *exec.Cmd
	ptmx       *os.File
	screen     vt10x.Terminal
	readerDone chan struct{}
}

// startTerminalSession starts csync with args under a pseudo-terminal and begins
// feeding its output to the emulated screen.
func startTerminalSession(ctx context.Context, args []string) (*terminalSession, error) {
	cmd := exec.Command(csyncBinary, args...)
	cmd.Env = append(csyncEnv(ctx), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: terminalRows, Cols: terminalCols})
	if err != nil {
		return nil, fmt.Errorf("start under a pty: %w", err)
	}
	session := &terminalSession{
		cmd:        cmd,
		ptmx:       ptmx,
		screen:     vt10x.New(vt10x.WithSize(terminalCols, terminalRows), vt10x.WithWriter(ptmx)),
		readerDone: make(chan struct{}),
	}
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

// close stops csync and releases the terminal, so a failed scenario can't leave a
// child waiting for keys that will never come.
func (s *terminalSession) close() {
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	_ = s.ptmx.Close()
	<-s.readerDone
}
