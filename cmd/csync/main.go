// Command csync is cherry-sync's CLI: it compares a source and destination with
// rsync, prints the changes rsync would make, asks which to sync, and transfers
// the chosen files. It is the thin orchestration layer over the internal
// packages (cli, compare, selection, transfer, tui) that do the work.
package main

import (
	"os"
	"strconv"
	"time"

	"golang.org/x/term"
)

// version is csync's release version, injected at build time via
// `-ldflags "-X main.version=…"` (see .goreleaser.yaml). It defaults to "dev"
// for un-injected builds — `go build ./cmd/csync`, `go run`, the test harness —
// which the version line renders as "cherry-sync (dev build)".
var version = "dev"

// compareTimeout is how long a comparison may run before csync stops waiting on
// it (#53). Sixty seconds is the interaction budget: past it the tool is no
// longer doing anything a person is present for, and an unresponsive remote or a
// hung SSH connection would otherwise hang csync indefinitely with no recourse
// short of killing it. It is 59 rather than 60 so the spinner's own count never
// reaches three digits.
const compareTimeout = 59 * time.Second

// defaultStallTimeout is how long a transfer may go without a byte from the
// remote before csync stops waiting on it (#53). Unlike compareTimeout this is a
// silence budget, not an elapsed one: a legitimately large transfer runs as long
// as it needs, and only one that has gone quiet is cut off. Thirty seconds leaves
// room for the receiver to read a very large file while generating its block
// checksums, which is the longest a healthy transfer goes without saying
// anything.
const defaultStallTimeout = 30 * time.Second

// stallTimeoutVar is the environment variable that overrides defaultStallTimeout,
// in whole seconds. It is deliberately undocumented: it exists so the acceptance
// suite can prove the bound without waiting out the real one on every run, not as
// a knob for tuning csync, which has no more business being configurable here
// than the comparison's limit does.
const stallTimeoutVar = "CSYNC_STALL_TIMEOUT"

// stallTimeout returns the silence budget for this run: defaultStallTimeout,
// unless stallTimeoutVar names a positive whole number of seconds. Anything else
// — unset, unparseable, zero, negative — falls back to the default rather than
// failing the run, because a malformed test hook must never be able to leave a
// transfer unbounded, which is the very thing the bound exists to prevent.
func stallTimeout() time.Duration {
	raw := os.Getenv(stallTimeoutVar)
	if raw == "" {
		return defaultStallTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultStallTimeout
	}
	return time.Duration(seconds) * time.Second
}

// main runs csync and exits with the status it reports. It holds no logic of its
// own: os.Exit skips deferred functions, so confining it to this one line is what
// lets run own the resources — the run log above all — and release them on every
// path out.
func main() {
	os.Exit(run())
}

// interactiveTerminal reports whether csync is attached to a real terminal on both
// ends — stdin (to read keys) and stdout (to render). Only then is the Bubble Tea
// picker usable; a piped or redirected run, including the test harness, takes the
// typed-grammar fallback instead.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
