// Harness for the acceptance suite: the compiled binary and the fake remote
// shells the scenarios run against, the context keys scenarios pass state
// through, and the entry points that build them before any feature runs.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// localChangeMtime is the modification time stamped on files touched by the
// "changed locally" / "added locally" steps. rsync's quick-check compares mtime
// at whole-second granularity, so if a file were created and synced within the
// same second (as everything in this harness otherwise is), a transfer that
// failed to preserve mtime would still compare equal — masking the bug. Dating
// the source files firmly in the past forces the post-transfer "now" timestamp
// into a different second, so mtime-preservation is actually exercised. The
// assertions only read file bytes, so a fixed past time is invisible to them.
var localChangeMtime = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

// csyncBinary is the path to the compiled csync binary, set by TestMain
// before any scenario runs.
var csyncBinary string

// fakeRsh is the path to a test-only remote shell written by TestMain. Scenarios
// tagged @remote run csync with RSYNC_RSH pointing here so a `fakehost:` operand
// puts rsync into real remote mode without an SSH server. See fakeRshScript.
var fakeRsh string

// fakeRshScript is the body of the fake remote shell. rsync invokes a remote
// shell as `rsh <host> <command...>`; dropping the host and exec-ing the rest
// locally makes a `fakehost:` transfer run on this machine yet still travel
// rsync's remote (sender/receiver) code path — so it emits the `<f`/`>f`
// direction codes a real push/pull would, which local-to-local never does and
// the suite was therefore structurally blind to.
const fakeRshScript = `#!/bin/sh
shift
exec "$@"
`

// failMeasureRsh is the path to a test-only remote shell that answers the
// comparison and fails the pass that measures the destination. Scenarios proving
// the comparison survives an unmeasurable destination point RSYNC_RSH here. See
// failMeasureRshScript.
var failMeasureRsh string

// failMeasureRshScript is the body of that shell. On a push the two passes ask
// opposite things of the far side: the comparison makes it the receiver, while the
// measuring pass reads it and so runs it as `--server --sender`. That flag is
// therefore the pass csync uses to read the destination, and failing on it leaves
// the comparison itself untouched.
//
// Keying on the role rather than on a count of invocations matters for the same
// reason it does in stallRshScript: the number of rsync calls a comparison spends
// is an implementation detail, and a counter would quietly start failing the wrong
// pass the moment it changed.
//
// Exiting non-zero (rather than going silent) is the point: this is a remote that
// answers and refuses, which is what rsync reports as a failed pass.
const failMeasureRshScript = `#!/bin/sh
for a in "$@"; do
	if [ "$a" = "--sender" ]; then
		echo "measurement refused by the test remote" >&2
		exit 13
	fi
done
shift
exec "$@"
`

// stallRsh is the path to a test-only remote shell that answers every rsync the
// comparison runs and goes silent once the transfer starts. Scenarios that need a
// stalled transfer point RSYNC_RSH here instead of at fakeRsh. See stallRshScript.
var stallRsh string

// stallRshScript is the body of the stalling remote shell. It tells the comparison
// from the transfer by what rsync asks the far side to do: a dry run carries `n` in
// the condensed flag bundle it sends the server (`-ntrce.iLsfxCIvu`), and a real
// transfer does not (`-tre.iLsfxCIvu`). Every dry run execs normally; the first
// invocation that is not one sleeps instead of answering.
//
// Keying on the phase rather than on a count of invocations is deliberate. The
// comparison does not always spend exactly one rsync — a push also measures the
// destination to report how each file differs — and a counter would silently
// reclassify that second comparison call as the transfer, stalling the run in the
// wrong phase and testing something the scenario does not claim.
//
// Sleeping rather than exiting is the point — a peer that closes the connection is
// an error rsync reports immediately, while one that holds it open and says nothing
// is the stall being tested. It holds the pipe open by not exec-ing anything, and
// outlives any timeout the suite sets while still reaping itself long before the
// run ends.
//
// Only the short bundles are inspected: a `--` long option is skipped, so an
// innocent `n` inside `--log-format` or `--sender` cannot be mistaken for the
// dry-run flag.
const stallRshScript = `#!/bin/sh
for a in "$@"; do
	case "$a" in
	--*) ;;
	-*) case "$a" in *n*) shift; exec "$@" ;; esac ;;
	esac
done
sleep 60
exit 0
`

// stallTestTimeout is the value of CSYNC_STALL_TIMEOUT given to a csync child in
// a stall scenario, in seconds. It exists so the scenario does not wait out the
// real 30-second bound on every PR; rsync overshoots a small timeout by a few
// seconds, so the scenario costs well under ten.
const stallTestTimeout = "3"

// runWait bounds how long runCsync will wait for the csync child to exit. It is a
// deadlock guard rather than a timing assumption — a run that ends on its own
// costs nothing — and it is what keeps a stall scenario from hanging the whole
// suite while csync is still missing the bound the scenario is there to prove.
const runWait = 45 * time.Second

// outputKey is the context key used to stash the captured runResult from
// `When I run "..."` so the following Then steps can assert on it.
type outputKey struct{}

// localPathKey stashes the per-scenario local tempdir path set up by `Given a
// local directory ...`. iRun reads it to substitute the Gherkin placeholder
// `./project` with the real path before invoking csync.
type localPathKey struct{}

// remotePathKey stashes the per-scenario remote tempdir path set up by the
// `... identical between local and remote` and `empty remote directory` steps.
// iRun reads it to substitute `user@host:/project` before invoking csync.
type remotePathKey struct{}

// remoteModeKey flags a scenario (via the @remote tag) as needing a real remote
// transport: runCsync then resolves the remote placeholder to a `fakehost:` path
// and sets RSYNC_RSH to fakeRsh, so rsync runs in sender/receiver mode rather
// than local-to-local — the only way the suite exercises the `<` push direction.
type remoteModeKey struct{}

// stallModeKey flags a scenario (via the `goes silent once the comparison is
// done` step) as needing the stalling remote shell rather than the plain one, so
// csyncEnv points RSYNC_RSH at stallRsh and gives the child its counter file.
type stallModeKey struct{}

// failMeasureModeKey flags a scenario (via the `a remote that answers the
// comparison but fails the measurement` step) as needing the refusing remote shell
// rather than the plain one.
type failMeasureModeKey struct{}

// homeKey stashes a per-scenario throwaway home directory, created by the Before
// hook and removed by the After hook. runCsync points the csync child's HOME and
// XDG_STATE_HOME into it so a run log lands inside the scenario's own tempdir
// rather than the developer's real ~/.local/state.
type homeKey struct{}

// startedKey stashes a runningCsync — a csync child left alive and blocked at the
// selection prompt — so a later step can read the log it is midway through
// writing, then answer the prompt and let it finish.
type startedKey struct{}

// foundLogKey stashes the path of the log file located under the scenario's
// XDG_STATE_HOME while csync was still running. The reconciliation step compares
// it against the path csync discloses on its way out.
type foundLogKey struct{}

// changedFileKey stashes which file the parameterless `a file has been changed
// locally` step picked, so a later step can assert the sync moved it without the
// scenario having to name it. Which file it is was never the point.
type changedFileKey struct{}

// invokedArgsKey stashes the argument vector runCsync actually passed to the csync
// child — after the `./project`/`user@host:/project` placeholders are substituted —
// so the invocation-line step can reconcile the log against what was really run
// rather than reconstruct the substitution itself.
type invokedArgsKey struct{}

// noXdgKey flags a scenario (via `Given the environment variable XDG_STATE_HOME is
// not set`) as needing the csync child to run without XDG_STATE_HOME, so its log
// falls back to ~/.local/state under the throwaway home. csyncEnv reads it.
type noXdgKey struct{}

// seededLogsKey stashes the names of the run logs a pruning scenario planted before
// csync ran. Knowing exactly which files were already there is what lets a later
// step pick out the log this run wrote without sorting by age — the very ordering
// the pruning scenarios are there to test — and what lets it name the logs that
// should have survived.
type seededLogsKey struct{}

// plantedFileKey stashes the path of the not-a-run-log file a scenario planted in
// the log directory, so the step that checks it survived does not have to know the
// name the planting step chose.
type plantedFileKey struct{}

// runResult holds everything the test world cares about after a csync
// invocation: the two output streams kept separate so step funcs can assert
// against the right one, plus the process exit code.
type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// selectionPrompt is the fragment csync writes to stderr when it blocks on stdin
// waiting to be told what to sync. Seeing it means the comparison has finished and
// no transfer has begun, which is the only moment a test can read a half-written
// log without racing the process that is writing it.
const selectionPrompt = "Press Enter"

// promptWait bounds how long a step will wait for csync to reach the selection
// prompt before giving up. It is a deadlock guard, not a timing assumption: the
// wait ends as soon as the prompt appears, so a slow machine costs nothing.
const promptWait = 20 * time.Second

// syncBuffer is a bytes.Buffer safe for a reader and a writer at once. A live
// csync child has os/exec goroutines copying its streams in while a step polls
// them for the prompt, which a bare bytes.Buffer does not allow.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends to the buffer under the lock, satisfying io.Writer for exec.Cmd.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything written so far, safe to call while writes continue.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runningCsync is a csync child that has been started but not yet reaped, held
// across steps so one can observe it mid-run and the next can finish it. The
// After hook kills any child a scenario leaves behind, so a failed assertion
// cannot strand a process blocked on a pipe nobody will ever write to.
//
// done carries the result of Wait, which runs in its own goroutine from the moment
// the child starts. That is what lets a step waiting for the prompt notice a csync
// that exited instead of asking, and say so, rather than waiting out the timeout
// and reporting the symptom. reaped records that done has been drained, since a
// channel can only deliver that result once.
type runningCsync struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *syncBuffer
	stderr *syncBuffer
	done   chan error
	reaped bool
}

// TestMain builds the csync binary into a temp dir once, records its path in
// csyncBinary for the scenarios to invoke, and removes it when the suite ends.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "csync-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempdir:", err)
		os.Exit(2)
	}
	defer os.RemoveAll(tmpDir)

	csyncBinary = filepath.Join(tmpDir, "csync")
	// Inject a known version the same way a release build does (-X main.version),
	// so the report-version scenarios can assert an exact string and, more to the
	// point, exercise the real ldflags injection seam end to end rather than a
	// hardcoded default.
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.0.0-test", "-o", csyncBinary, "github.com/dpassarelli/cherry-sync/cmd/csync")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	err = build.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(2)
	}

	fakeRsh = filepath.Join(tmpDir, "fakersh")
	err = os.WriteFile(fakeRsh, []byte(fakeRshScript), 0o755)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakersh:", err)
		os.Exit(2)
	}

	failMeasureRsh = filepath.Join(tmpDir, "failmeasurersh")
	err = os.WriteFile(failMeasureRsh, []byte(failMeasureRshScript), 0o755)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failmeasurersh:", err)
		os.Exit(2)
	}

	stallRsh = filepath.Join(tmpDir, "stallrsh")
	err = os.WriteFile(stallRsh, []byte(stallRshScript), 0o755)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stallrsh:", err)
		os.Exit(2)
	}

	os.Exit(m.Run())
}

// TestFeatures runs the godog feature suite under `go test`, excluding
// @wip-tagged features. A non-zero suite result fails the test.
func TestFeatures(t *testing.T) {
	// Exclude @wip (scenarios drafted ahead of their step definitions and
	// production code). @git scenarios set up a real git work tree; when git
	// isn't on PATH, exclude them too rather than fail — they run wherever git
	// is available.
	tags := "~@wip"
	_, err := exec.LookPath("git")
	if err != nil {
		tags = "~@wip && ~@git"
	}

	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Strict:   true,
			Tags:     tags,
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// captured returns the runResult stashed by iRun, or a zero value if the run
// step hasn't executed.
func captured(ctx context.Context) runResult {
	r, _ := ctx.Value(outputKey{}).(runResult)
	return r
}
