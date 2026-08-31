package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
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

// stallRsh is the path to a test-only remote shell that answers the first rsync
// that runs and goes silent for every one after it. Scenarios that need a stalled
// transfer point RSYNC_RSH here instead of at fakeRsh. See stallRshScript.
var stallRsh string

// stallRshScript is the body of the stalling remote shell. csync runs one rsync
// to compare and another to transfer, each spawning its own remote shell, so a
// counter kept in the file named by CSYNC_TEST_RSH_STATE is what tells the two
// apart: invocation 1 (the comparison) execs normally, and every later one sleeps
// instead of answering. Sleeping rather than exiting is the point — a peer that
// closes the connection is an error rsync reports immediately, while one that
// holds it open and says nothing is the stall being tested. It holds the pipe
// open by not exec-ing anything, and outlives any timeout the suite sets while
// still reaping itself long before the run ends.
const stallRshScript = `#!/bin/sh
n=$(cat "$CSYNC_TEST_RSH_STATE" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > "$CSYNC_TEST_RSH_STATE"
if [ "$n" -gt 1 ]; then
	sleep 60
	exit 0
fi
shift
exec "$@"
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

// InitializeScenario registers each Gherkin step with its step function and
// installs an After hook that removes the per-scenario local and remote
// tempdirs.
func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^I run "([^"]*)"$`, iRun)
	ctx.Step(`^I run "([^"]*)" a second time$`, iRun)
	ctx.Step(`^I run "([^"]*)" and respond with "([^"]*)"$`, iRunAndRespond)
	ctx.Step(`^I run "([^"]*)" from the project directory$`, iRunFromTheProjectDirectory)
	ctx.Step(`^I run "([^"]*)" from the project directory and respond with "([^"]*)"$`, iRunFromTheProjectDirectoryAndRespond)
	ctx.Step(`^the reported source should be "([^"]*)"$`, theReportedSourceShouldBe)
	ctx.Step(`^the reported destination should be "([^"]*)"$`, theReportedDestinationShouldBe)
	ctx.Step(`^csync should return exit code (\d+)$`, csyncShouldReturnExitCode)
	ctx.Step(`^csync should return a non-zero exit code$`, csyncShouldReturnANonZeroExitCode)
	ctx.Step(`^the help text should contain "([^"]*)"$`, theHelpTextShouldContain)
	ctx.Step(`^the reported message should begin with "([^"]*)"$`, theReportedMessageShouldBeginWith)
	ctx.Step(`^the reported version should be "([^"]*)"$`, theReportedVersionShouldBe)
	ctx.Step(`^the reported license should contain "([^"]*)"$`, theReportedLicenseShouldContain)
	ctx.Step(`^csync should report where it logged the run$`, csyncShouldReportWhereItLoggedTheRun)
	ctx.Step(`^a run log should exist at the reported path$`, aRunLogShouldExistAtTheReportedPath)
	ctx.Step(`^that a file has been changed locally$`, thatAFileHasBeenChangedLocally)
	ctx.Step(`^a remote that goes silent once the comparison is done$`, aRemoteThatGoesSilentOnceTheComparisonIsDone)
	ctx.Step(`^I have started csync but not yet answered the prompt$`, iHaveStartedCsyncButNotYetAnsweredThePrompt)
	// Two phrasings of the same act, kept apart because Gherkin reads better when a
	// Given narrates in the past and a When in the present. Both locate the log.
	ctx.Step(`^I look for the log file$`, iLocateTheLogFile)
	ctx.Step(`^I have taken note of where the log file is$`, iLocateTheLogFile)
	ctx.Step(`^the log file should already have content$`, theLogFileShouldAlreadyHaveContent)
	ctx.Step(`^the log should record that the version was "([^"]*)"$`, theLogShouldRecordThatTheVersionWas)
	ctx.Step(`^the log should record running "([^"]*)" for the comparison$`, theLogShouldRecordRunningForTheComparison)
	ctx.Step(`^the log should record the transfer that ran$`, theLogShouldRecordTheTransferThatRan)
	ctx.Step(`^the log should record the removal that ran$`, theLogShouldRecordTheRemovalThatRan)
	ctx.Step(`^the log should record running "([^"]*)" for the ignore rules$`, theLogShouldRecordRunningForTheIgnoreRules)
	ctx.Step(`^a local directory whose path contains a space$`, aLocalDirectoryWhosePathContainsASpace)
	ctx.Step(`^a local directory whose path contains a double quote$`, aLocalDirectoryWhosePathContainsADoubleQuote)
	ctx.Step(`^a local source path that does not exist$`, aLocalSourcePathThatDoesNotExist)
	ctx.Step(`^the log should record the comparison's failing exit code$`, theLogShouldRecordTheComparisonsFailingExitCode)
	ctx.Step(`^the logged duration should be a positive whole number of milliseconds$`, theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds)
	ctx.Step(`^the log should record (\d+) classified changes?$`, theLogShouldRecordNClassifiedChanges)
	ctx.Step(`^the log should record (\d+) selected changes?$`, theLogShouldRecordNSelectedChanges)
	ctx.Step(`^the classified changes should include "([^"]*)" of "([^"]*)"$`, theClassifiedChangesShouldInclude)
	ctx.Step(`^the selected changes should include "([^"]*)" of "([^"]*)"$`, theSelectedChangesShouldInclude)
	ctx.Step(`^the log should record "([^"]*)" among the excluded paths$`, theLogShouldRecordAmongTheExcludedPaths)
	ctx.Step(`^the log should record that the \.git directory was excluded$`, theLogShouldRecordThatTheGitDirectoryWasExcluded)
	ctx.Step(`^the log should record that source path as one argument$`, theLogShouldRecordThatSourcePathAsOneArgument)
	ctx.Step(`^the log should record the command line that was run$`, theLogShouldRecordTheCommandLineThatWasRun)
	ctx.Step(`^the log should name the source and destination csync reported$`, theLogShouldNameTheSourceAndDestinationReported)
	ctx.Step(`^I answer the prompt$`, iAnswerThePrompt)
	// A restatement of `csync should return exit code 0` in the vocabulary of a
	// scenario that has no interest in the number, only in csync having finished
	// what it was doing rather than falling over partway.
	ctx.Step(`^csync should exit normally$`, csyncShouldExitNormally)
	ctx.Step(`^the reported log path should be the one I found earlier$`, theReportedLogPathShouldBeTheOneIFoundEarlier)
	ctx.Step(`^that csync cannot write its log$`, thatCsyncCannotWriteItsLog)
	ctx.Step(`^the changed file is deleted before I answer$`, theChangedFileIsDeletedBeforeIAnswer)
	ctx.Step(`^the changed file should be identical between local and remote$`, theChangedFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^csync should warn that it could not write a run log$`, csyncShouldWarnThatItCouldNotWriteARunLog)
	ctx.Step(`^csync should not report where it logged the run$`, csyncShouldNotReportWhereItLoggedTheRun)
	ctx.Step(`^csync should say last of all that the run was not logged$`, csyncShouldSayLastOfAllThatTheRunWasNotLogged)
	ctx.Step(`^no run log should have been written$`, noRunLogShouldHaveBeenWritten)
	ctx.Step(`^the environment variable XDG_STATE_HOME is set$`, xdgStateHomeIsSet)
	ctx.Step(`^the environment variable XDG_STATE_HOME is not set$`, xdgStateHomeIsNotSet)
	ctx.Step(`^the run log should be under "([^"]*)" in (.+)$`, theRunLogShouldBeUnderIn)
	ctx.Step(`^the run log directory should be accessible only by its owner$`, theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner)
	ctx.Step(`^the run log file should be accessible only by its owner$`, theRunLogFileShouldBeAccessibleOnlyByItsOwner)
	ctx.Step(`^(\d+) run logs already exist$`, nRunLogsAlreadyExist)
	ctx.Step(`^a file that is not a run log in the log directory$`, aFileThatIsNotARunLogInTheLogDirectory)
	ctx.Step(`^the log directory should hold (\d+) run logs?$`, theLogDirectoryShouldHoldNRunLogs)
	ctx.Step(`^the surviving run logs should be the newest ones$`, theSurvivingRunLogsShouldBeTheNewestOnes)
	ctx.Step(`^that file should still be there$`, thatFileShouldStillBeThere)
	ctx.Step(`^the log should name the run logs it pruned$`, theLogShouldNameTheRunLogsItPruned)
	ctx.Step(`^the log should record that nothing was pruned$`, theLogShouldRecordThatNothingWasPruned)
	ctx.Step(`^the reported error should mention "([^"]*)"$`, theReportedErrorShouldMention)
	ctx.Step(`^csync should report that it rewrote "([^"]*)"$`, csyncShouldReportThatItRewrote)
	ctx.Step(`^a local directory containing these files:$`, aLocalDirectoryContainingTheseFiles)
	ctx.Step(`^a "\.csync\.toml" in the project directory containing:$`, aCsyncTomlInTheProjectDirectoryContaining)
	ctx.Step(`^a local git repository containing these files:$`, aLocalGitRepositoryContainingTheseFiles)
	ctx.Step(`^the repository's "([^"]*)" contains:$`, theLocalFileContains)
	ctx.Step(`^the directory's "([^"]*)" contains:$`, theLocalFileContains)
	ctx.Step(`^that all of the files are identical between local and remote$`, allFilesIdenticalBetweenLocalAndRemote)
	ctx.Step(`^an empty remote directory$`, anEmptyRemoteDirectory)
	ctx.Step(`^that the file "([^"]*)" has been changed locally$`, theFileHasBeenChangedLocally)
	ctx.Step(`^that the file "([^"]*)" has a different modification time but identical content$`, theFileHasADifferentMtimeButIdenticalContent)
	ctx.Step(`^that the file "([^"]*)" has been added locally$`, theFileHasBeenAddedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added on the remote$`, theFileHasBeenAddedOnTheRemote)
	ctx.Step(`^that the file "([^"]*)" has been deleted locally$`, theFileHasBeenDeletedLocally)
	ctx.Step(`^no actions should be reported$`, noActionsShouldBeReported)
	ctx.Step(`^the reported actions should be:$`, theReportedActionsShouldBe)
	ctx.Step(`^the reported actions should be, in order:$`, theReportedActionsShouldBeInOrder)
	ctx.Step(`^the reported changes should be numbered, in order:$`, theReportedChangesShouldBeNumberedInOrder)
	ctx.Step(`^the reported change count should be (\d+)$`, theReportedChangeCountShouldBe)
	ctx.Step(`^the reported excluded count should be (\d+)$`, theReportedExcludedCountShouldBe)
	ctx.Step(`^no gitignored paths should be reported as excluded$`, noGitignoredPathsShouldBeReportedAsExcluded)
	ctx.Step(`^the \.git directory should be reported as excluded$`, theGitDirectoryShouldBeReportedAsExcluded)
	ctx.Step(`^the \.csync\.toml file should be reported as excluded$`, theCsyncTomlShouldBeReportedAsExcluded)
	ctx.Step(`^the \.git directory should not be reported as excluded$`, theGitDirectoryShouldNotBeReportedAsExcluded)
	ctx.Step(`^the reported sync count should be (\d+)$`, theReportedSyncCountShouldBe)
	ctx.Step(`^the reported removed count should be (\d+)$`, theReportedRemovedCountShouldBe)
	ctx.Step(`^the file "([^"]*)" should be identical between local and remote$`, theFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^the file "([^"]*)" should not exist on the remote$`, theFileShouldNotExistOnTheRemote)
	ctx.Step(`^the file "([^"]*)" should still exist on the remote$`, theFileShouldStillExistOnTheRemote)
	ctx.Step(`^the file "([^"]*)" should still differ between local and remote$`, theFileShouldStillDifferBetweenLocalAndRemote)

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		home, err := os.MkdirTemp("", "csync-home-*")
		if err != nil {
			return ctx, fmt.Errorf("mktempdir: %w", err)
		}
		ctx = context.WithValue(ctx, homeKey{}, home)
		for _, tag := range sc.Tags {
			if tag.Name == "@remote" {
				ctx = context.WithValue(ctx, remoteModeKey{}, true)
			}
		}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		// Reap a csync left blocked at the prompt — by a scenario that means to leave
		// it there, or by one that failed before it could answer. Without this, the
		// tempdirs below are removed out from under a live process and `go test` waits
		// forever on a child waiting for a stdin nobody will write to.
		run, _ := ctx.Value(startedKey{}).(*runningCsync)
		if run != nil && !run.reaped {
			run.stdin.Close()
			run.cmd.Process.Kill()
			<-run.done
		}
		localPath, _ := ctx.Value(localPathKey{}).(string)
		if localPath != "" {
			os.RemoveAll(localPath)
		}
		remotePath, _ := ctx.Value(remotePathKey{}).(string)
		if remotePath != "" {
			os.RemoveAll(remotePath)
		}
		home, _ := ctx.Value(homeKey{}).(string)
		if home != "" {
			os.RemoveAll(home)
		}
		return ctx, nil
	})
}

// iRun executes the `When I run "..."` step with no interactive input. It also
// backs the `... a second time` phrasing: the scenario's local and remote
// tempdirs persist in the context across steps, so a repeat invocation runs
// against the already-synced state — which is what idempotence checks assert on.
// With no stdin, a second run that does surface phantom changes reads EOF at the
// prompt and selects nothing rather than blocking.
func iRun(ctx context.Context, command string) (context.Context, error) {
	return runCsync(ctx, command, nil, "")
}

// iRunFromTheProjectDirectory backs `When I run "..." from the project
// directory`: it runs csync with its working directory set to the scenario's
// local tempdir, so cwd-only .csync.toml discovery finds the dotfile the
// config-write step placed there. The verb forms (push/pull) take no operands,
// so no stdin is fed.
func iRunFromTheProjectDirectory(ctx context.Context, command string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	return runCsync(ctx, command, nil, local)
}

// iRunFromTheProjectDirectoryAndRespond backs `When I run "..." from the project
// directory and respond with "..."`: like iRunFromTheProjectDirectory but feeds
// the prompt response on stdin, so a push/pull resolved from .csync.toml can be
// driven through the selection prompt. The `<empty>` sentinel stands for a bare
// Enter, as in iRunAndRespond.
func iRunFromTheProjectDirectoryAndRespond(ctx context.Context, command, response string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	if response == "<empty>" {
		response = ""
	}
	return runCsync(ctx, command, strings.NewReader(response+"\n"), local)
}

// iRunAndRespond executes the `When I run "..." and respond with "..."` step,
// feeding the response to csync on stdin as if typed at the prompt. The
// `<empty>` sentinel stands for an empty response (a bare Enter); a trailing
// newline is appended so the response reads as a completed line.
func iRunAndRespond(ctx context.Context, command, response string) (context.Context, error) {
	if response == "<empty>" {
		response = ""
	}
	return runCsync(ctx, command, strings.NewReader(response+"\n"), "")
}

// resolvedRemote returns the real remote operand that the Gherkin placeholder
// "user@host:/project" stands for in this scenario: a `fakehost:` path under
// @remote (so rsync runs in sender/receiver mode via RSYNC_RSH), a bare local
// path otherwise, or "" when no remote directory was set up. Both runCsync (for
// command-line operands) and the .csync.toml write step resolve through this, so
// a remote read from config points at the same place as one passed on argv.
func resolvedRemote(ctx context.Context) string {
	remotePath, _ := ctx.Value(remotePathKey{}).(string)
	if remotePath == "" {
		return ""
	}
	remoteMode, _ := ctx.Value(remoteModeKey{}).(bool)
	if remoteMode {
		return "fakehost:" + remotePath
	}
	return remotePath
}

// stateHome returns the directory the csync child sees as XDG_STATE_HOME, or ""
// when no scenario home was set up. It is deliberately NOT the home directory's
// ~/.local/state: csync falls back to that path when the variable is unset, so
// pointing both at the same place would let a csync that ignored XDG_STATE_HOME
// pass the location scenario by accident. Kept distinct, the two are
// distinguishable, and a run that lands in the wrong one is visible.
func stateHome(ctx context.Context) string {
	home, _ := ctx.Value(homeKey{}).(string)
	if home == "" {
		return ""
	}
	return filepath.Join(home, "xdg-state")
}

// csyncEnv builds the environment for the csync child: the ambient environment
// with HOME and XDG_STATE_HOME redirected into the scenario's throwaway home, and
// RSYNC_RSH added under @remote.
//
// The two variables are filtered out of the ambient environment before being set,
// rather than appended to override by last-duplicate-wins. Filtering is what makes
// the fallback testable: under `XDG_STATE_HOME is not set` the child must see no
// XDG_STATE_HOME at all, which an append can never achieve if the developer's own
// shell exported one. It also makes the child's view of both variables the same
// whoever runs the suite.
func csyncEnv(ctx context.Context) []string {
	env := os.Environ()
	home, _ := ctx.Value(homeKey{}).(string)
	if home != "" {
		env = withoutVars(env, "HOME", "XDG_STATE_HOME")
		env = append(env, "HOME="+home)
		noXdg, _ := ctx.Value(noXdgKey{}).(bool)
		if !noXdg {
			env = append(env, "XDG_STATE_HOME="+stateHome(ctx))
		}
	}
	remoteMode, _ := ctx.Value(remoteModeKey{}).(bool)
	if remoteMode {
		// rsync reads RSYNC_RSH as its remote shell; fakeRsh execs locally so the
		// `fakehost:` operand transfers on this machine over the real remote code
		// path. csync's own rsync child inherits this environment.
		rsh := fakeRsh
		stallMode, _ := ctx.Value(stallModeKey{}).(bool)
		if stallMode {
			// Swap in the shell that stops answering after the comparison, and shorten
			// csync's own bound so the scenario doesn't wait out the shipped default.
			rsh = stallRsh
			env = append(env, "CSYNC_TEST_RSH_STATE="+filepath.Join(home, "rsh-invocations"))
			env = append(env, "CSYNC_STALL_TIMEOUT="+stallTestTimeout)
		}
		env = append(env, "RSYNC_RSH="+rsh)
	}
	return env
}

// withoutVars returns env with every `NAME=...` entry for the named variables
// removed, so a caller can set them cleanly without an ambient value surviving.
func withoutVars(env []string, names ...string) []string {
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	kept := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if drop[name] {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// runCsync splits the command, substitutes the Gherkin placeholders
// (`./project`, `user@host:/project`, `<empty>`) with the scenario's real
// tempdir paths, runs the csync binary with the given stdin, and stashes the
// captured streams and exit code in the context.
func runCsync(ctx context.Context, command string, stdin io.Reader, dir string) (context.Context, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ctx, fmt.Errorf("empty command")
	}
	if parts[0] != "csync" {
		return ctx, fmt.Errorf("expected command to start with %q, got %q", "csync", parts[0])
	}

	args := parts[1:]
	// <empty> is a sentinel for an empty-string argument: the step regex and
	// strings.Fields can't carry a literal "" through the Gherkin command, so
	// scenarios write <empty> and we substitute it here.
	subs := map[string]string{"<empty>": ""}
	localPath, _ := ctx.Value(localPathKey{}).(string)
	if localPath != "" {
		subs["./project"] = localPath
	}
	remote := resolvedRemote(ctx)
	if remote != "" {
		subs["user@host:/project"] = remote
	}
	for i, a := range args {
		replacement, ok := subs[a]
		if ok {
			args[i] = replacement
		}
	}

	runCtx, cancelRun := context.WithTimeout(ctx, runWait)
	defer cancelRun()
	cmd := exec.CommandContext(runCtx, csyncBinary, args...)
	cmd.Stdin = stdin
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = csyncEnv(ctx)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()

	// A csync that never exits is a failure of the thing under test, not a slow
	// machine: report it as one rather than letting the killed child's signal be
	// read as an ordinary non-zero exit.
	if runCtx.Err() != nil {
		return ctx, fmt.Errorf("csync did not exit within %s; stderr so far:\n%s", runWait, stderrBuf.String())
	}

	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return ctx, fmt.Errorf("exec failed: %w", err)
		}
		exitCode = exitErr.ExitCode()
	}

	result := runResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}
	ctx = context.WithValue(ctx, invokedArgsKey{}, args)
	return context.WithValue(ctx, outputKey{}, result), nil
}

// theReportedSourceShouldBe asserts the parsed "Source:" line equals want.
func theReportedSourceShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Source

	if got != want {
		return fmt.Errorf("Source: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedDestinationShouldBe asserts the parsed "Destination:" line equals want.
func theReportedDestinationShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Destination

	if got != want {
		return fmt.Errorf("Destination: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// csyncShouldReturnExitCode asserts the captured process exit code equals want.
func csyncShouldReturnExitCode(ctx context.Context, want int) error {
	got := captured(ctx).ExitCode

	if got != want {
		return fmt.Errorf("exit code: got %d, want %d (stderr: %q)", got, want, captured(ctx).Stderr)
	}
	return nil
}

// csyncShouldReturnANonZeroExitCode asserts the captured exit code is non-zero
// (the error path, without pinning a specific code).
func csyncShouldReturnANonZeroExitCode(ctx context.Context) error {
	r := captured(ctx)

	if r.ExitCode == 0 {
		return fmt.Errorf("exit code: got 0, want non-zero (stdout: %q, stderr: %q)", r.Stdout, r.Stderr)
	}
	return nil
}

// theHelpTextShouldContain asserts a substring appears in what csync printed to
// stdout for `--help` — used to check the help summary carries its version
// header, sections, commands, and flags without pinning the full block (the
// view package's own tests guard the exact layout).
func theHelpTextShouldContain(ctx context.Context, want string) error {
	r := captured(ctx)
	if !strings.Contains(r.Stdout, want) {
		return fmt.Errorf("help output missing %q in stdout:\n%s", want, r.Stdout)
	}
	return nil
}

// aRemoteThatGoesSilentOnceTheComparisonIsDone points the scenario at the remote
// shell that answers the comparison and then stops answering, so the stall lands
// on the transfer rather than on the comparison. The comparison must be allowed
// to succeed: it is what produces the change list the scenario then chooses from,
// and csync never reaches a transfer without it.
func aRemoteThatGoesSilentOnceTheComparisonIsDone(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, stallModeKey{}, true), nil
}

// theReportedErrorShouldMention asserts csync's stderr contains want — the error
// it printed before exiting non-zero. The config-rejection scenarios use it to
// pin that the failure names the offending file (or its nature) rather than
// failing opaquely or, worse, letting an empty operand reach rsync.
func theReportedErrorShouldMention(ctx context.Context, want string) error {
	r := captured(ctx)
	if !strings.Contains(r.Stderr, want) {
		return fmt.Errorf("expected error to mention %q, stderr was:\n%s", want, r.Stderr)
	}
	return nil
}

// csyncShouldReportThatItRewrote asserts csync disclosed on stdout that it
// rewrote an operand from the given original path portion — the inline
// "(rewritten from …)" note beside the header value that keeps a "~"
// normalization from being silent.
func csyncShouldReportThatItRewrote(ctx context.Context, from string) error {
	r := captured(ctx)
	want := "(rewritten from " + from + ")"
	if !strings.Contains(r.Stdout, want) {
		return fmt.Errorf("expected stdout to disclose %q, stdout was:\n%s", want, r.Stdout)
	}
	return nil
}

// theReportedMessageShouldBeginWith asserts the free-text status message parsed
// from stdout starts with want.
func theReportedMessageShouldBeginWith(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Message

	if !strings.HasPrefix(got, want) {
		return fmt.Errorf("Message: got %q, want prefix %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedVersionShouldBe asserts the version line csync prints for
// `--version` (parsed from stdout) equals want.
func theReportedVersionShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Version

	if got != want {
		return fmt.Errorf("Version: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedLicenseShouldContain asserts a substring appears in what csync
// printed to stdout — used to check `csync --license` emits the MIT notices
// without pinning the full text (which the license package's own test guards).
func theReportedLicenseShouldContain(ctx context.Context, want string) error {
	r := captured(ctx)
	if !strings.Contains(r.Stdout, want) {
		return fmt.Errorf("license output missing %q in stdout:\n%s", want, r.Stdout)
	}
	return nil
}

// csyncShouldReportWhereItLoggedTheRun asserts csync disclosed the path of the
// run log it wrote. A record nobody can find is not a record, so the disclosure is a
// behavior in its own right — which is why the steps that read a log's contents find
// it by scanning the scenario's state home instead: were they to read through the
// disclosure, breaking it would redden every one of them alongside this.
func csyncShouldReportWhereItLoggedTheRun(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if !out.HasLogPath {
		return fmt.Errorf("csync reported no log path; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	if out.LogPath == "" {
		return fmt.Errorf("csync printed an empty log path; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	return nil
}

// aRunLogShouldExistAtTheReportedPath asserts the path csync disclosed names a
// regular file that is really there. Pairing the two steps is what keeps csync
// honest: reporting a path it never wrote to fails here, and writing a log it
// never mentions fails the step above.
func aRunLogShouldExistAtTheReportedPath(ctx context.Context) error {
	r := captured(ctx)
	path := parseOutput(r.Stdout, r.Stderr).LogPath
	if path == "" {
		return fmt.Errorf("csync reported no log path, so there is none to look for; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("run log reported at %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("run log reported at %q is not a regular file (mode %s)", path, info.Mode())
	}
	return nil
}

// thatAFileHasBeenChangedLocally arranges the one condition the run-log scenarios
// need from a comparison: something to sync, so csync stops at the prompt. Which
// file, and how many, is not what those scenarios are about — they name neither —
// so this composes the existing steps and picks a file from the Background itself.
func thatAFileHasBeenChangedLocally(ctx context.Context) (context.Context, error) {
	ctx, err := allFilesIdenticalBetweenLocalAndRemote(ctx)
	if err != nil {
		return ctx, err
	}
	const changed = "README.md"
	ctx, err = theFileHasBeenChangedLocally(ctx, changed)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, changedFileKey{}, changed), nil
}

// theChangedFileIsDeletedBeforeIAnswer removes the local file the scenario changed,
// while csync sits at the prompt with the comparison already done. The transfer is
// then told to send a file that is no longer there and stops with a partial-transfer
// error, which is a failure on the far side of the prompt rather than before it.
//
// Deleting rather than chmod-ing, for the reason thatCsyncCannotWriteItsLog gives:
// root ignores permission bits, so a setup built on them stops testing anything the
// day this suite runs as root. Nothing gets to read a file that is gone.
func theChangedFileIsDeletedBeforeIAnswer(ctx context.Context) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return fmt.Errorf("local path not set; missing Background step?")
	}
	changed, _ := ctx.Value(changedFileKey{}).(string)
	if changed == "" {
		return fmt.Errorf("no file was changed locally, so there is none to delete")
	}
	err := os.Remove(filepath.Join(local, changed))
	if err != nil {
		return fmt.Errorf("deleting the changed file: %w", err)
	}
	return nil
}

// thatCsyncCannotWriteItsLog puts a regular file where csync expects to create its
// state directory, so every attempt to make one fails with ENOTDIR.
//
// A file rather than a chmod-ed directory: root ignores permission bits, so a
// read-only directory would quietly stop testing anything the day this suite runs
// as root (in a container, say). Nothing gets to mkdir inside a regular file.
func thatCsyncCannotWriteItsLog(ctx context.Context) error {
	root := stateHome(ctx)
	if root == "" {
		return fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	err := os.WriteFile(root, []byte("not a directory\n"), 0o600)
	if err != nil {
		return fmt.Errorf("blocking the state directory at %s: %w", root, err)
	}
	return nil
}

// theChangedFileShouldBeIdenticalBetweenLocalAndRemote asserts the sync moved the
// file the parameterless setup step changed — the whole point of a run that logged
// nothing still being a run that worked.
func theChangedFileShouldBeIdenticalBetweenLocalAndRemote(ctx context.Context) error {
	changed, _ := ctx.Value(changedFileKey{}).(string)
	if changed == "" {
		return fmt.Errorf("no file was changed; missing a step that changes one?")
	}
	return theFileShouldBeIdenticalBetweenLocalAndRemote(ctx, changed)
}

// csyncShouldWarnThatItCouldNotWriteARunLog asserts csync said out loud that this
// run went unlogged. Declining silently would be the worst of the three outcomes:
// the user goes looking for the record of a destructive run, finds nothing, and
// cannot tell whether csync failed to write it or they misremembered where it goes.
func csyncShouldWarnThatItCouldNotWriteARunLog(ctx context.Context) error {
	r := captured(ctx)
	warning := parseOutput(r.Stdout, r.Stderr).Warning
	if warning == "" {
		return fmt.Errorf("csync issued no warning; stderr:\n%s", r.Stderr)
	}
	if !strings.Contains(warning, "log") {
		return fmt.Errorf("csync warned about something other than the run log: %q", warning)
	}
	return nil
}

// csyncShouldNotReportWhereItLoggedTheRun asserts csync named no log path. It is
// the counterpart of csyncShouldReportWhereItLoggedTheRun: a path is disclosed when
// there is a file at the end of it, and withheld when there is not. Sending the
// user to a log that was never created is a worse answer than admitting there is
// none.
// It reads the presence of the disclosure line, not the truth of its value: csync
// printing a bare "Log written to" with nothing after it is a disclosure, and a useless one.
func csyncShouldNotReportWhereItLoggedTheRun(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if out.HasLogPath {
		return fmt.Errorf("csync reported a log path %q, but it wrote no log", out.LogPath)
	}
	return nil
}

// iHaveStartedCsyncButNotYetAnsweredThePrompt starts csync and returns once it is
// blocked on stdin at the selection prompt, leaving it there for the steps that
// follow. The operands are not in the Gherkin: the scenarios that use this step
// are about the run log, not about how csync is invoked, so the command line is an
// implementation detail of the step rather than a fact of the scenario.
//
// Waiting for the prompt to appear on stderr is what makes the observation that
// follows deterministic. csync writes it after the comparison and before any
// transfer, then blocks — so the process is suspended, not merely slow, and a step
// that reads the log now cannot be racing a csync that is still writing it.
func iHaveStartedCsyncButNotYetAnsweredThePrompt(ctx context.Context) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	remote := resolvedRemote(ctx)
	if remote == "" {
		return ctx, fmt.Errorf("remote path not set; missing a step that creates one?")
	}

	cmd := exec.Command(csyncBinary, local, remote)
	cmd.Env = csyncEnv(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ctx, fmt.Errorf("stdin pipe: %w", err)
	}
	run := &runningCsync{cmd: cmd, stdin: stdin, stdout: &syncBuffer{}, stderr: &syncBuffer{}, done: make(chan error, 1)}
	cmd.Stdout = run.stdout
	cmd.Stderr = run.stderr
	err = cmd.Start()
	if err != nil {
		return ctx, fmt.Errorf("start csync: %w", err)
	}
	go func() { run.done <- cmd.Wait() }()
	// Stash before waiting: if the prompt never comes, the After hook still has the
	// child to kill.
	ctx = context.WithValue(ctx, startedKey{}, run)

	deadline := time.Now().Add(promptWait)
	for !strings.Contains(run.stderr.String(), selectionPrompt) {
		select {
		case waitErr := <-run.done:
			// csync exited rather than asking. Report that, not the timeout it would
			// otherwise become: a scenario whose csync died has a different problem
			// from one whose csync hung, and the two should not read alike.
			run.reaped = true
			return ctx, fmt.Errorf("csync exited (%v) before reaching the selection prompt\nstdout:\n%s\nstderr:\n%s",
				waitErr, run.stdout.String(), run.stderr.String())
		default:
		}
		if time.Now().After(deadline) {
			return ctx, fmt.Errorf("csync never reached the selection prompt within %s\nstdout:\n%s\nstderr:\n%s",
				promptWait, run.stdout.String(), run.stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ctx, nil
}

// iLocateTheLogFile finds the run log under the scenario's XDG_STATE_HOME and
// stashes its path. It backs both `When I look for the log file` and `Given I have
// taken note of where the log file is`.
//
// It searches rather than being told, because the scenarios must not name the
// path: csync has not yet disclosed it (that happens as csync exits) and where it
// belongs is pinned by the location scenario alone. Insisting on exactly one match
// is what makes the search an honest stand-in for the disclosure — with two logs
// under the state directory, "the log file" would not mean anything.
func iLocateTheLogFile(ctx context.Context) (context.Context, error) {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, foundLogKey{}, log), nil
}

// theLogFileShouldAlreadyHaveContent asserts the log is not empty at the moment
// csync is blocked at the prompt. Content, not existence: csync opens the file as
// it starts, so an empty one proves only that it was created. Bytes on disk here
// prove the records were written as the run proceeded — a log assembled in memory
// and flushed on the way out would be empty at this moment, and would be lost
// entirely on the abnormal exits the log exists to survive.
func theLogFileShouldAlreadyHaveContent(ctx context.Context) error {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		return fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("run log at %q: %w", path, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("run log at %q is empty while csync waits at the prompt; nothing has been written to disk yet", path)
	}
	return nil
}

// parseLogAt reads the run log at path and returns it parsed, along with the raw
// contents for error messages. It is the one place the log steps turn a path into a
// ParsedLog, so each reads structured fields rather than matching substrings.
func parseLogAt(path string) (ParsedLog, string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- path is the log this suite created under its own tempdir
	if err != nil {
		return ParsedLog{}, "", fmt.Errorf("run log at %q: %w", path, err)
	}
	return parseLog(string(content)), string(content), nil
}

// locatedLog returns the parsed log the scenario found under XDG_STATE_HOME (via
// `I look for the log file`), for the steps that read it while csync is still
// blocked at the prompt — before csync has disclosed the path itself.
func locatedLog(ctx context.Context) (ParsedLog, string, error) {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		return ParsedLog{}, "", fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	return parseLogAt(path)
}

// theLogShouldRecordThatTheVersionWas asserts the located log names the version
// csync ran as. It reads the file while csync is still blocked at the prompt, so a
// pass proves the version was recorded up front rather than at exit. The check ties
// the record to the known version the harness injected (see report-version): a log
// that named some other build, or named none, fails here.
func theLogShouldRecordThatTheVersionWas(ctx context.Context, want string) error {
	log, content, err := locatedLog(ctx)
	if err != nil {
		return err
	}
	if log.Version != want {
		return fmt.Errorf("run log records version %q, want %q; contents:\n%s", log.Version, want, content)
	}
	return nil
}

// theLogShouldRecordRunningForTheComparison asserts the located log names the
// external command csync ran to compare the two sides. Read while csync is blocked
// at the prompt, the comparison is the only command that has run, so an "exec
// <name>" record for it proves csync logs what it actually invoked — the fact a
// destructive run cannot be re-run to recover. It keys on the "exec <name>"
// pairing, not the name alone, so the name appearing elsewhere cannot satisfy it.
func theLogShouldRecordRunningForTheComparison(ctx context.Context, name string) error {
	log, content, err := locatedLog(ctx)
	if err != nil {
		return err
	}
	if _, ok := log.command(name); !ok {
		return fmt.Errorf("run log records no command %q; contents:\n%s", name, content)
	}
	return nil
}

// theLogShouldRecordTheTransferThatRan asserts a completed run logged the transfer
// pass, not only the comparison. Both are rsync, so the transfer surfaces as a second
// rsync record beyond the dry-run comparison: after the run finishes the log holds
// two, where at the prompt it held one. Keying on the count of rsync records rather
// than any flag keeps the check to the fidelity fact that matters — the transfer was
// recorded at all — and off the argv composition a refactor might change.
func theLogShouldRecordTheTransferThatRan(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	rsyncs := log.commands("rsync")
	if len(rsyncs) < 2 {
		return fmt.Errorf("run log records %d rsync command(s), want the comparison and the transfer; contents:\n%s", len(rsyncs), content)
	}
	return nil
}

// exitStatusRE pulls the numeric code out of the error csync surfaces when a command
// fails — an *exec.ExitError renders as "exit status N". The reconciliation reads it so
// the exit-code scenario need not hardcode rsync's failure code, which differs between
// rsync flavors.
var exitStatusRE = regexp.MustCompile(`exit status (\d+)`)

// theLogShouldRecordTheComparisonsFailingExitCode asserts the run log recorded the
// real, non-zero exit code of a comparison that failed at rsync — not a zero, not a
// placeholder. It reconciles against the code csync surfaced in its own error ("rsync:
// exit status N"), so the check needs no hardcoded rsync code: whatever csync saw, the
// log must show. A runner that logged a fixed exit=0, or dropped the process's real
// code, reddens here — which is what keeps the log honest about the runs worth reading.
func theLogShouldRecordTheComparisonsFailingExitCode(ctx context.Context) error {
	r := captured(ctx)
	m := exitStatusRE.FindStringSubmatch(r.Stderr)
	if m == nil {
		return fmt.Errorf("csync surfaced no rsync exit status to reconcile against; stderr:\n%s", r.Stderr)
	}
	want, _ := strconv.Atoi(m[1])
	if want == 0 {
		return fmt.Errorf("csync surfaced exit status 0, which is not a failure to test")
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	if cmd.ExitCode != want {
		return fmt.Errorf("run log records rsync exit=%d, want %d (what csync reported); contents:\n%s", cmd.ExitCode, want, content)
	}
	return nil
}

// wholeMillisRE matches a duration expressed as a whole number of milliseconds with no
// fractional part — "44ms", not "43.7ms". The capture is the millisecond count, so the
// duration scenario can also check it is greater than zero.
var wholeMillisRE = regexp.MustCompile(`^(\d+)ms$`)

// resolvedLog returns the parsed log a scenario is asking about, from wherever it is
// available: the path a mid-run "look for the log file" step stashed, or else the one
// log under the scenario's state home. It locates the log without reading what csync
// disclosed, so that a scenario asking what a log contains fails only on the content it
// names; whether csync says where it logged is a separate behavior with its own
// scenarios, and coupling the two made one break redden both.
func resolvedLog(ctx context.Context) (ParsedLog, string, error) {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		found, err := theOneRunLog(ctx)
		if err != nil {
			return ParsedLog{}, "", err
		}
		path = found
	}
	return parseLogAt(path)
}

// theLogShouldRecordNClassifiedChanges asserts the log recorded the classification, and
// that its stated count and its list agree on how many changes csync found. Checking the
// count and the list length together catches a record whose header and body disagree.
func theLogShouldRecordNClassifiedChanges(ctx context.Context, n int) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasClassified {
		return fmt.Errorf("run log records no classified-changes line; contents:\n%s", content)
	}
	if log.ClassifiedCount != n || len(log.Classified) != n {
		return fmt.Errorf("run log records classified count=%d over a list of %d, want %d of each; contents:\n%s", log.ClassifiedCount, len(log.Classified), n, content)
	}
	return nil
}

// theLogShouldRecordNSelectedChanges asserts the log recorded the selection, count and
// list agreeing. HasSelected distinguishes "recorded that none were selected" (a real
// 0) from "never recorded a selection at all", so "record 0 selected changes" still
// demands the record be present.
func theLogShouldRecordNSelectedChanges(ctx context.Context, n int) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasSelected {
		return fmt.Errorf("run log records no selected-changes line; contents:\n%s", content)
	}
	if log.SelectedCount != n || len(log.Selected) != n {
		return fmt.Errorf("run log records selected count=%d over a list of %d, want %d of each; contents:\n%s", log.SelectedCount, len(log.Selected), n, content)
	}
	return nil
}

// theClassifiedChangesShouldInclude asserts a specific verb/path pair is among the
// changes csync recorded classifying — that the record names the actual changes, not
// just a count.
func theClassifiedChangesShouldInclude(ctx context.Context, verb, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.has(log.Classified, verb, path) {
		return fmt.Errorf("run log's classified changes do not include %s %q; got %+v; contents:\n%s", verb, path, log.Classified, content)
	}
	return nil
}

// theSelectedChangesShouldInclude asserts a specific verb/path pair is among the changes
// the user selected — the record that a removal, say, was actually taken and applied.
func theSelectedChangesShouldInclude(ctx context.Context, verb, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.has(log.Selected, verb, path) {
		return fmt.Errorf("run log's selected changes do not include %s %q; got %+v; contents:\n%s", verb, path, log.Selected, content)
	}
	return nil
}

// theLogShouldRecordAmongTheExcludedPaths asserts a specific gitignored path is named
// in the exclusion record — the point of #82's exclusion logging over a bare count: the
// log can answer whether a given file was held out of the comparison, not just how many
// were. It reads through the facade, which keeps the excluded names, so the assertion is
// on the recorded name rather than a substring of the raw line.
func theLogShouldRecordAmongTheExcludedPaths(ctx context.Context, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasExcluded {
		return fmt.Errorf("run log records no excluded line; contents:\n%s", content)
	}
	if slices.Contains(log.ExcludedGitignored, path) {
		return nil
	}
	return fmt.Errorf("run log's excluded paths do not include %q; got %+v; contents:\n%s", path, log.ExcludedGitignored, content)
}

// theLogShouldRecordThatTheGitDirectoryWasExcluded asserts the exclusion record notes
// the .git directory was withheld — the singleton exclusion csync always applies in a
// work tree, named for free (there is only ever one) alongside the gitignored paths.
func theLogShouldRecordThatTheGitDirectoryWasExcluded(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasExcluded {
		return fmt.Errorf("run log records no excluded line; contents:\n%s", content)
	}
	if !log.ExcludedGitDir {
		return fmt.Errorf("run log's excluded line does not note the .git directory; contents:\n%s", content)
	}
	return nil
}

// theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds asserts a recorded
// command's duration is present, decimal-free, and greater than zero — the shape a
// rounded-up whole-millisecond value takes. It reads the comparison's duration through
// the facade, which keeps the raw duration text, so this pins the rendered format: an
// unrounded "43.764397ms" fails the whole-millisecond match, and a "0ms" fails the
// greater-than-zero check that rounding up exists to uphold.
func theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	m := wholeMillisRE.FindStringSubmatch(cmd.Duration)
	if m == nil {
		return fmt.Errorf("run log records duration %q, want a whole number of milliseconds like %q; contents:\n%s", cmd.Duration, "44ms", content)
	}
	ms, _ := strconv.Atoi(m[1])
	if ms <= 0 {
		return fmt.Errorf("run log records duration %q, want greater than zero; contents:\n%s", cmd.Duration, content)
	}
	return nil
}

// theLogShouldRecordThatSourcePathAsOneArgument asserts the source operand — a path
// carrying a character the log format has to handle specially (a space, or the quote
// delimiter itself) — came back out of the log's argument vector as a single element,
// whole. It reconciles against the source csync reported (source of truth, so the check
// needs no knowledge of the tempdir) and reads the comparison's argv through the facade,
// whose parseLogArgs unquotes each token. The trailing slash rsync's operands carry is
// trimmed before the compare. Two ways the operand could fail to survive: joined with
// spaces it fractures (the space scenario's teeth), and wrapped without escaping the
// embedded quote closes the token early (the double-quote scenario's) — either leaves no
// element carrying the operand whole, which is the failure this reconciliation catches.
func theLogShouldRecordThatSourcePathAsOneArgument(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if !strings.ContainsAny(out.Source, " \"") {
		return fmt.Errorf("csync reported source %q, which has no space or quote to test fidelity with", out.Source)
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	for _, a := range cmd.Args {
		if strings.TrimSuffix(a, "/") == out.Source {
			return nil
		}
	}
	return fmt.Errorf("run log's rsync argv does not carry source %q as one argument; args=%q; contents:\n%s", out.Source, cmd.Args, content)
}

// theLogShouldRecordRunningForTheIgnoreRules asserts a run in a git work tree logged
// the git query csync made to learn what the repository ignores. Read after the run
// exits (identical sides, so it never prompts), the git command appears alongside the
// comparison — where a non-repo run logs no git at all, since the work-tree probe that
// gates it is deliberately left unlogged. It keys on the "exec <name>" pairing, like
// the comparison step, so the name appearing elsewhere cannot satisfy it.
func theLogShouldRecordRunningForTheIgnoreRules(ctx context.Context, name string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if _, ok := log.command(name); !ok {
		return fmt.Errorf("run log records no command %q; contents:\n%s", name, content)
	}
	return nil
}

// theLogShouldRecordTheRemovalThatRan asserts a deletion-only run logged the removal
// pass. Like the transfer, the removal is rsync, so it surfaces as a second rsync
// record beyond the dry-run comparison — but here the setup deletes rather than
// changes a file, so the second pass is the --delete removal, not a transfer (there
// is nothing to transfer). Counting the rsync records, rather than reading a flag,
// pins the presence-fidelity fact — the removal was recorded — and isolates it from
// the transfer scenario by what the run did.
func theLogShouldRecordTheRemovalThatRan(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	rsyncs := log.commands("rsync")
	if len(rsyncs) < 2 {
		return fmt.Errorf("run log records %d rsync command(s), want the comparison and the removal; contents:\n%s", len(rsyncs), content)
	}
	return nil
}

// theLogShouldRecordTheCommandLineThatWasRun asserts the log's invocation line is
// the literal command csync was run with: "csync" followed by the argument vector
// runCsync actually passed (placeholders already substituted). Reconciling against
// the stashed argv keeps the check honest without the scenario reconstructing the
// tempdir substitution, and pins that the line is the raw invocation — not the
// resolved operands, which get their own lines.
func theLogShouldRecordTheCommandLineThatWasRun(ctx context.Context) error {
	args, _ := ctx.Value(invokedArgsKey{}).([]string)
	want := strings.Join(append([]string{"csync"}, args...), " ")
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if log.Invocation != want {
		return fmt.Errorf("run log records invocation %q, want %q; contents:\n%s", log.Invocation, want, content)
	}
	return nil
}

// theLogShouldNameTheSourceAndDestinationReported asserts the log records both
// operands, and that they are the same source and destination csync printed in its
// header. Reconciling the two is what makes the check honest: csync is the source of
// truth for what the operands resolved to, so a log that named some other path, or
// named none, fails here — without the scenario having to know the tempdir layout.
func theLogShouldNameTheSourceAndDestinationReported(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if out.Source == "" || out.Destination == "" {
		return fmt.Errorf("csync reported an empty operand (source %q, destination %q); nothing to reconcile", out.Source, out.Destination)
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if log.Source != out.Source {
		return fmt.Errorf("run log names source %q, want %q (what csync reported); contents:\n%s", log.Source, out.Source, content)
	}
	if log.Destination != out.Destination {
		return fmt.Errorf("run log names destination %q, want %q (what csync reported); contents:\n%s", log.Destination, out.Destination, content)
	}
	return nil
}

// iAnswerThePrompt sends a bare Enter to the waiting csync — accept every change —
// and waits for it to finish, stashing its streams and exit code where the ordinary
// Then steps read them.
func iAnswerThePrompt(ctx context.Context) (context.Context, error) {
	run, _ := ctx.Value(startedKey{}).(*runningCsync)
	if run == nil {
		return ctx, fmt.Errorf("csync was not started; missing a step that starts it?")
	}
	_, err := io.WriteString(run.stdin, "\n")
	if err != nil {
		return ctx, fmt.Errorf("answering the prompt: %w", err)
	}
	err = run.stdin.Close()
	if err != nil {
		return ctx, fmt.Errorf("closing stdin: %w", err)
	}

	exitCode := 0
	err = <-run.done
	run.reaped = true
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return ctx, fmt.Errorf("waiting for csync: %w", err)
		}
		exitCode = exitErr.ExitCode()
	}
	result := runResult{Stdout: run.stdout.String(), Stderr: run.stderr.String(), ExitCode: exitCode}
	return context.WithValue(ctx, outputKey{}, result), nil
}

// csyncShouldExitNormally asserts csync ran to completion rather than falling over
// partway. It is `exit code 0` said in the vocabulary of a scenario that cares only
// that the run finished, since a csync that died has nothing to report about a log.
func csyncShouldExitNormally(ctx context.Context) error {
	return csyncShouldReturnExitCode(ctx, 0)
}

// theReportedLogPathShouldBeTheOneIFoundEarlier ties the log csync discloses on the
// way out to the file it was seen filling in mid-run. Without it the two halves are
// each honest and jointly useless: csync could write one file and name another.
func theReportedLogPathShouldBeTheOneIFoundEarlier(ctx context.Context) error {
	want, _ := ctx.Value(foundLogKey{}).(string)
	if want == "" {
		return fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).LogPath
	if got != want {
		return fmt.Errorf("csync reported log path %q, but the log it was writing is %q", got, want)
	}
	return nil
}

// csyncShouldSayLastOfAllThatTheRunWasNotLogged asserts the final thing csync
// prints is that it kept no record, and why.
//
// Last of all, not merely somewhere: the warning csync gives before the prompt can
// scroll away behind a long change list, and the whole point of repeating it is
// that the notice survives to where the user is still looking when the run ends. A
// step that only checked the notice was present would pass against the warning
// alone and prove nothing.
func csyncShouldSayLastOfAllThatTheRunWasNotLogged(ctx context.Context) error {
	r := captured(ctx)
	reason := parseOutput(r.Stdout, r.Stderr).NotLogged
	if reason == "" {
		return fmt.Errorf("csync gave no reason for keeping no record; stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "Not logged:") {
		return fmt.Errorf("csync's last word was %q, not that the run went unlogged", last)
	}
	return nil
}

// noRunLogShouldHaveBeenWritten asserts nothing was recorded under the scenario's
// state home: no log file anywhere beneath it, the state directory not existing at
// all counting as the same thing. It is the counterpart of iLocateTheLogFile —
// where that insists on exactly one log, this insists on none — and it is how a run
// that never reached rsync proves it left the state directory untouched.
func noRunLogShouldHaveBeenWritten(ctx context.Context) error {
	root := stateHome(ctx)
	if root == "" {
		return fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	logs, err := logsUnder(root)
	if err != nil {
		return fmt.Errorf("searching %s for run logs: %w", root, err)
	}
	if len(logs) != 0 {
		return fmt.Errorf("expected no run log under %s, but found %d: %v", root, len(logs), logs)
	}
	return nil
}

// xdgStateHomeIsSet documents the precondition that holds by default: the Before
// hook points the csync child's XDG_STATE_HOME into the scenario's throwaway home
// (see csyncEnv), so this step changes nothing. It is here so the location scenario
// names the condition it depends on, symmetric with its `... is not set` sibling.
func xdgStateHomeIsSet(ctx context.Context) error {
	return nil
}

// xdgStateHomeIsNotSet arranges for the csync child to run with no XDG_STATE_HOME,
// so its log falls back to ~/.local/state under the throwaway home. csyncEnv reads
// the flag and omits the variable (and filters any ambient one), which is the only
// way the fallback path is reached.
func xdgStateHomeIsNotSet(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, noXdgKey{}, true), nil
}

// theRunLogShouldBeUnderIn asserts the run log sits in the named subdirectory of a
// state-directory base — the one place in the suite that pins the layout. base is
// the symbol the scenario used, resolved to the throwaway home so the test names no
// real path: "$XDG_STATE_HOME" is where the harness points the variable, and
// "~/.local/state" is the spec's fallback under the child's HOME.
func theRunLogShouldBeUnderIn(ctx context.Context, sub, base string) error {
	var root string
	switch base {
	case "$XDG_STATE_HOME":
		root = stateHome(ctx)
	case "~/.local/state":
		home, _ := ctx.Value(homeKey{}).(string)
		if home == "" {
			return fmt.Errorf("no scenario home; the Before hook did not run?")
		}
		root = filepath.Join(home, ".local", "state")
	default:
		return fmt.Errorf("unknown state-directory base %q in scenario", base)
	}
	log, err := singleLogUnder(root)
	if err != nil {
		return err
	}
	wantDir := filepath.Join(root, sub)
	gotDir := filepath.Dir(log)
	if gotDir != wantDir {
		return fmt.Errorf("run log is in %s, want it in %s", gotDir, wantDir)
	}
	return nil
}

// theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner asserts the run log's
// directory is 0700 — the shape of a work tree is nobody else's business.
func theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner(ctx context.Context) error {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return err
	}
	return assertPerm(filepath.Dir(log), 0o700)
}

// theRunLogFileShouldBeAccessibleOnlyByItsOwner asserts the run log file itself is
// 0600, for the same reason its directory is 0700.
func theRunLogFileShouldBeAccessibleOnlyByItsOwner(ctx context.Context) error {
	log, err := theOneRunLog(ctx)
	if err != nil {
		return err
	}
	return assertPerm(log, 0o600)
}

// aLocalDirectoryWhosePathContainsASpace creates a local tempdir whose path holds a
// space and populates it like the plain local-directory step, re-stashing it under
// localPathKey (overriding the Background's). The space rides into the source operand
// csync hands rsync, so the run log must quote it to keep the operand one argument —
// which is what the space-fidelity scenario reads back out.
func aLocalDirectoryWhosePathContainsASpace(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, "src/main.go\nREADME.md")
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalDirectoryWhosePathContainsADoubleQuote creates a local tempdir whose path
// holds a double-quote character, populated like the plain local-directory step and
// re-stashed under localPathKey. The quote is the log's own delimiter: recorded naively
// it would forge a false argument boundary, so this is the counterpart to the space
// step — the space proves real boundaries are kept, the quote proves fake ones can't be
// minted.
func aLocalDirectoryWhosePathContainsADoubleQuote(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", `csync-q"-*`)
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, "src/main.go\nREADME.md")
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalSourcePathThatDoesNotExist stashes, as the local operand, a well-formed
// tempdir path that has been removed — so it is absent when rsync runs. It sets up a
// comparison that fails at rsync (a missing source errors out), the case that proves
// the run log records the real, non-zero exit code of a command that failed.
func aLocalSourcePathThatDoesNotExist(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-gone-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = os.RemoveAll(dir)
	if err != nil {
		return ctx, fmt.Errorf("removing %s: %w", dir, err)
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalDirectoryContainingTheseFiles creates a local tempdir populated with
// the (empty) files named in the DocString and stashes its path under
// localPathKey.
func aLocalDirectoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalGitRepositoryContainingTheseFiles creates a local tempdir, initializes a
// git work tree in it, and populates it with the (empty) files named in the
// DocString — the local-side setup the .gitignore scenarios need so csync can ask
// git what to ignore. The path is stashed under localPathKey, like its non-git
// twin, so the remote-setup and file-mutation steps work against it unchanged.
func aLocalGitRepositoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		return ctx, fmt.Errorf("git init: %w", err)
	}
	err = writeFiles(dir, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// theLocalFileContains writes the DocString to the named file in the local
// directory (e.g. ".gitignore"), establishing the ignore rules a scenario
// exercises. A trailing newline is appended so each line stands on its own. It
// backs both the "repository's" and the "directory's" phrasings: the same write
// serves a git work tree and a plain directory (the non-repo no-op scenario uses
// the latter to prove the gate is work-tree membership, not .gitignore presence).
func theLocalFileContains(ctx context.Context, name string, ds *godog.DocString) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, name)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte(strings.TrimSpace(ds.Content)+"\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	return ctx, nil
}

// aCsyncTomlInTheProjectDirectoryContaining writes the DocString to
// ./.csync.toml in the scenario's local directory — the saved-target file that
// `csync push`/`pull` read. The "user@host:/project" placeholder is rewritten to
// the scenario's real remote (the same substitution runCsync applies to a
// command-line operand) so a push/pull actually transfers against it; it is left
// verbatim when no remote was set up, which the resolution-display scenario
// relies on to assert the literal placeholder.
func aCsyncTomlInTheProjectDirectoryContaining(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	content := ds.Content
	remote := resolvedRemote(ctx)
	if remote != "" {
		content = strings.ReplaceAll(content, "user@host:/project", remote)
	}
	err := os.WriteFile(filepath.Join(local, ".csync.toml"), []byte(content), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write .csync.toml: %w", err)
	}
	return ctx, nil
}

// writeFiles creates each newline-separated relative path in content as an empty
// file under dir, making parent directories as needed; blank lines are skipped.
// Shared by the plain-directory and git-repository setup steps.
func writeFiles(dir, content string) error {
	for line := range strings.SplitSeq(strings.TrimSpace(content), "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" {
			continue
		}
		full := filepath.Join(dir, rel)
		err := os.MkdirAll(filepath.Dir(full), 0o755)
		if err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		err = os.WriteFile(full, []byte(""), 0o644)
		if err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
	}
	return nil
}

// allFilesIdenticalBetweenLocalAndRemote copies the local tree into a fresh
// remote tempdir so the two sides start identical, stashing the remote path
// under remotePathKey.
func allFilesIdenticalBetweenLocalAndRemote(ctx context.Context) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	remote, err := os.MkdirTemp("", "csync-remote-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = copyTree(local, remote)
	if err != nil {
		return ctx, fmt.Errorf("copy: %w", err)
	}
	return context.WithValue(ctx, remotePathKey{}, remote), nil
}

// anEmptyRemoteDirectory creates an empty remote tempdir and stashes its path
// under remotePathKey.
func anEmptyRemoteDirectory(ctx context.Context) (context.Context, error) {
	remote, err := os.MkdirTemp("", "csync-remote-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	return context.WithValue(ctx, remotePathKey{}, remote), nil
}

// theFileHasBeenChangedLocally overwrites the named file in the local tree so a
// later comparison reports it as modified.
func theFileHasBeenChangedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.WriteFile(full, []byte("modified\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	err = os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasADifferentMtimeButIdenticalContent stamps the named local file with a
// past modification time WITHOUT rewriting its bytes, so it differs from the remote
// copy in mtime alone. rsync's size+mtime quick-check then flags it for transfer
// even though the content is identical — the "phantom change" csync must not report.
// The harness otherwise creates and copies files within the same
// wall-clock second, so their mtimes compare equal at rsync's 1-second granularity;
// dating this one to the past forces the mtime-only delta the scenario needs.
func theFileHasADifferentMtimeButIdenticalContent(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasBeenAddedLocally writes a new file (creating parent dirs) into the
// local tree so a later comparison reports it as added.
func theFileHasBeenAddedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte("new file\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	err = os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasBeenDeletedLocally removes the named file from the local tree, which
// (the two sides having started identical) leaves it present on the remote but
// gone from the local side — a deletion on a push. With --delete on the compare,
// a later comparison reports it as a removal.
func theFileHasBeenDeletedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	err := os.Remove(filepath.Join(local, relPath))
	if err != nil {
		return ctx, fmt.Errorf("remove %s: %w", relPath, err)
	}
	return ctx, nil
}

// theFileHasBeenAddedOnTheRemote writes a new file (creating parent dirs) into
// the remote tree so a later comparison reports it as remote-only.
func theFileHasBeenAddedOnTheRemote(ctx context.Context, relPath string) (context.Context, error) {
	remote, _ := ctx.Value(remotePathKey{}).(string)
	if remote == "" {
		return ctx, fmt.Errorf("remote path not set; missing 'identical between local and remote' step?")
	}
	full := filepath.Join(remote, relPath)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte("remote only\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	return ctx, nil
}

// noActionsShouldBeReported asserts csync's output lists zero actions.
func noActionsShouldBeReported(ctx context.Context) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	if len(got) != 0 {
		return fmt.Errorf("Actions: got %d (%+v), want 0", len(got), got)
	}
	return nil
}

// theReportedActionsShouldBe asserts the reported actions match the table,
// order-insensitively — both sides are sorted before comparison. For an
// order-sensitive check see theReportedActionsShouldBeInOrder.
func theReportedActionsShouldBe(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	gotSorted := sortActions(verbPath(got))
	wantSorted := sortActions(verbPath(want))
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		return fmt.Errorf("Actions: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedActionsShouldBeInOrder asserts the reported actions match the
// table exactly, including sequence — unlike theReportedActionsShouldBe, which
// is order-insensitive. Used by scenarios that pin the display ordering.
func theReportedActionsShouldBeInOrder(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(verbPath(got), verbPath(want)) {
		return fmt.Errorf("Actions (in order): got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedChangesShouldBeNumberedInOrder asserts the reported changes match
// the table exactly — sequence and the visible 1-based selection number — so a
// user typing "1" at the prompt picks the first row. Unlike
// theReportedActionsShouldBeInOrder, this compares the rendered Index too.
func theReportedChangesShouldBeNumberedInOrder(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("Numbered changes: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// actionsFromTable reads a Gherkin table with "action" and "path" columns into
// a slice of Actions, preserving row order. An optional "number" column sets
// each Action's selection Index; absent it, Index is left 0 (the verb/path-only
// tables read by the order-agnostic steps).
func actionsFromTable(table *godog.Table) ([]Action, error) {
	headers := map[string]int{}
	for i, cell := range table.Rows[0].Cells {
		headers[cell.Value] = i
	}
	actionCol, ok1 := headers["action"]
	pathCol, ok2 := headers["path"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("expected 'action' and 'path' columns; got headers: %+v", headers)
	}
	numberCol, hasNumber := headers["number"]

	var actions []Action
	for _, row := range table.Rows[1:] {
		act := Action{
			Verb: row.Cells[actionCol].Value,
			Path: row.Cells[pathCol].Value,
		}
		if hasNumber {
			n, err := strconv.Atoi(strings.TrimSpace(row.Cells[numberCol].Value))
			if err != nil {
				return nil, fmt.Errorf("number column: %w", err)
			}
			act.Index = n
		}
		actions = append(actions, act)
	}
	return actions, nil
}

// verbPath returns a copy of a with each Action's selection Index cleared, for
// the steps that assert verb and path only and are indifferent to the displayed
// numbering. theReportedChangesShouldBeNumberedInOrder compares Index directly.
func verbPath(a []Action) []Action {
	out := make([]Action, len(a))
	for i, x := range a {
		out[i] = Action{Verb: x.Verb, Path: x.Path}
	}
	return out
}

// sortActions returns a copy of a sorted by verb then path, so two action
// slices can be compared regardless of their original order.
func sortActions(a []Action) []Action {
	cpy := append([]Action(nil), a...)
	sort.Slice(cpy, func(i, j int) bool {
		if cpy[i].Verb != cpy[j].Verb {
			return cpy[i].Verb < cpy[j].Verb
		}
		return cpy[i].Path < cpy[j].Path
	})
	return cpy
}

// theReportedChangeCountShouldBe asserts csync printed a "Changes:" line and
// that its count equals want.
func theReportedChangeCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasChangeCount {
		return fmt.Errorf("no Changes line in output:\n%s", r.Stdout)
	}
	if parsed.ChangeCount != want {
		return fmt.Errorf("Changes: got %d, want %d in output:\n%s", parsed.ChangeCount, want, r.Stdout)
	}
	return nil
}

// theReportedExcludedCountShouldBe asserts csync printed an exclusion disclosure
// and that its count equals want. The "(excluding …)" line is the user's only
// signal that ignored paths were hidden, so its absence (HasExcludedCount false)
// is itself a failure.
func theReportedExcludedCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasExcludedCount {
		return fmt.Errorf("no exclusion disclosure in output:\n%s", r.Stdout)
	}
	if parsed.ExcludedCount != want {
		return fmt.Errorf("excluded count: got %d, want %d in output:\n%s", parsed.ExcludedCount, want, r.Stdout)
	}
	return nil
}

// noGitignoredPathsShouldBeReportedAsExcluded asserts csync printed no exclusion
// disclosure at all — the "(excluding …)" aside is omitted entirely when nothing
// was hidden, so a non-repo (or empty-ignore) sync stays free of empty-exclusion
// noise.
func noGitignoredPathsShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if parsed.HasExcludedCount {
		return fmt.Errorf("exclusion disclosure present (count %d) but none expected in output:\n%s", parsed.ExcludedCount, r.Stdout)
	}
	return nil
}

// theGitDirectoryShouldBeReportedAsExcluded asserts csync's exclusion disclosure
// announces the .git directory. git never lists .git/ as ignored, so this
// disclosure is the user's only signal that the VCS metadata dir was held back.
func theGitDirectoryShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.ExcludedGitDir {
		return fmt.Errorf("expected the .git directory to be reported as excluded, but it was not, in output:\n%s", r.Stdout)
	}
	return nil
}

// theCsyncTomlShouldBeReportedAsExcluded asserts csync's exclusion disclosure
// announces that its own .csync.toml was held back. The config file is withheld from every
// sync with no opt-out, so — like the .git disclosure — this line is the user's
// only signal it was excluded rather than offered for transfer.
func theCsyncTomlShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.ExcludedCsyncToml {
		return fmt.Errorf("expected the .csync.toml file to be reported as excluded, but it was not, in output:\n%s", r.Stdout)
	}
	return nil
}

// theGitDirectoryShouldNotBeReportedAsExcluded asserts csync did NOT announce a
// .git exclusion — the case where the local side is not a git work tree, so there
// is no .git/ to exclude and nothing to disclose.
func theGitDirectoryShouldNotBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if parsed.ExcludedGitDir {
		return fmt.Errorf("the .git directory was reported as excluded but should not be, in output:\n%s", r.Stdout)
	}
	return nil
}

// theReportedSyncCountShouldBe asserts csync printed a "Synced:" line and that
// its count equals want.
func theReportedSyncCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasSyncCount {
		return fmt.Errorf("no Synced line in output:\n%s", r.Stdout)
	}
	if parsed.SyncCount != want {
		return fmt.Errorf("Synced: got %d, want %d in output:\n%s", parsed.SyncCount, want, r.Stdout)
	}
	return nil
}

// theReportedRemovedCountShouldBe asserts the post-sync summary called out
// removals distinctly ("… M of which were removed") and that M equals want. It
// reads the removal count specifically, not the total files count that
// theReportedSyncCountShouldBe checks.
func theReportedRemovedCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasRemovedCount {
		return fmt.Errorf("no removal clause in summary:\n%s", r.Stdout)
	}
	if parsed.RemovedCount != want {
		return fmt.Errorf("removed: got %d, want %d in output:\n%s", parsed.RemovedCount, want, r.Stdout)
	}
	return nil
}

// theFileShouldBeIdenticalBetweenLocalAndRemote asserts the named file has the
// same bytes on both sides — i.e. the transfer actually moved it.
func theFileShouldBeIdenticalBetweenLocalAndRemote(ctx context.Context, relPath string) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)

	localBytes, err := os.ReadFile(filepath.Join(local, relPath))
	if err != nil {
		return fmt.Errorf("read local %s: %w", relPath, err)
	}
	remoteBytes, err := os.ReadFile(filepath.Join(remote, relPath))
	if err != nil {
		return fmt.Errorf("read remote %s: %w", relPath, err)
	}
	if !bytes.Equal(localBytes, remoteBytes) {
		return fmt.Errorf("file %q differs: local %q, remote %q", relPath, localBytes, remoteBytes)
	}
	return nil
}

// theFileShouldStillDifferBetweenLocalAndRemote asserts the named file's bytes
// differ across the two sides — i.e. a change that wasn't selected was left
// untransferred (the file exists on both sides, but the remote is still stale).
func theFileShouldStillDifferBetweenLocalAndRemote(ctx context.Context, relPath string) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)

	localBytes, err := os.ReadFile(filepath.Join(local, relPath))
	if err != nil {
		return fmt.Errorf("read local %s: %w", relPath, err)
	}
	remoteBytes, err := os.ReadFile(filepath.Join(remote, relPath))
	if err != nil {
		return fmt.Errorf("read remote %s: %w", relPath, err)
	}
	if bytes.Equal(localBytes, remoteBytes) {
		return fmt.Errorf("file %q is identical on both sides but should still differ", relPath)
	}
	return nil
}

// theFileShouldNotExistOnTheRemote asserts the named file is absent on the
// remote side — i.e. a change that wasn't selected was not transferred.
func theFileShouldNotExistOnTheRemote(ctx context.Context, relPath string) error {
	remote, _ := ctx.Value(remotePathKey{}).(string)

	_, err := os.Stat(filepath.Join(remote, relPath))
	if err == nil {
		return fmt.Errorf("file %q exists on the remote but should not", relPath)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat remote %s: %w", relPath, err)
	}
	return nil
}

// theFileShouldStillExistOnTheRemote asserts the named file is present on the
// remote side — i.e. a reported deletion was not applied (it was left unselected,
// or the run only reported changes without transferring). The mirror of
// theFileShouldNotExistOnTheRemote.
func theFileShouldStillExistOnTheRemote(ctx context.Context, relPath string) error {
	remote, _ := ctx.Value(remotePathKey{}).(string)

	_, err := os.Stat(filepath.Join(remote, relPath))
	if os.IsNotExist(err) {
		return fmt.Errorf("file %q is absent on the remote but should still exist", relPath)
	}
	if err != nil {
		return fmt.Errorf("stat remote %s: %w", relPath, err)
	}
	return nil
}

// captured returns the runResult stashed by iRun, or a zero value if the run
// step hasn't executed.
func captured(ctx context.Context) runResult {
	r, _ := ctx.Value(outputKey{}).(runResult)
	return r
}

// logsUnder returns every regular file beneath root, in walk order. A root that
// does not exist yields no files rather than an error: a run that wrote no log
// leaves the state directory uncreated, which is the same observation as an empty
// one. It underlies the log-location assertions — none, exactly one, and under a
// named directory.
func logsUnder(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// singleLogUnder returns the one run log beneath root, or an error naming what it
// found instead. Insisting on exactly one is what lets a caller speak of "the run
// log" — with two under the state directory the phrase would mean nothing.
func singleLogUnder(root string) (string, error) {
	logs, err := logsUnder(root)
	if err != nil {
		return "", fmt.Errorf("searching %s for a run log: %w", root, err)
	}
	if len(logs) != 1 {
		return "", fmt.Errorf("want exactly one run log under %s, found %d: %v", root, len(logs), logs)
	}
	return logs[0], nil
}

// theOneRunLog returns the single run log under the scenario's XDG_STATE_HOME, the
// path the assertions that inspect a log's location or permissions read through.
func theOneRunLog(ctx context.Context) (string, error) {
	root := stateHome(ctx)
	if root == "" {
		return "", fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	return singleLogUnder(root)
}

// runLogNameRE matches the names csync gives its run logs. The pruning steps count
// and compare through it rather than through logsUnder, which returns every regular
// file: a scenario that plants a foreign file in the log directory would otherwise
// see it counted as a log, and the scenario asserting that foreign files survive
// would pass for the wrong reason.
var runLogNameRE = regexp.MustCompile(`^run-\d{8}T\d{6}Z-\d+\.log$`)

// runLogsUnder returns the names of the run logs directly beneath the log directory
// in root, sorted oldest first. Sorting by name is sorting by start time: the
// timestamp is fixed-width and leads the name, so lexical order is chronological.
func runLogsUnder(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "cherry-sync"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading the log directory under %s: %w", root, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && runLogNameRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// scenarioRunLogs returns the run logs presently under the scenario's state home,
// oldest first.
func scenarioRunLogs(ctx context.Context) ([]string, error) {
	root := stateHome(ctx)
	if root == "" {
		return nil, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	return runLogsUnder(root)
}

// nRunLogsAlreadyExist plants n run logs in the log directory, named as csync names
// its own so that pruning treats them as candidates. Their timestamps run from a
// fixed date well in the past, one minute apart and ascending, so the set has an
// unambiguous oldest-to-newest order and every one of them is older than the log the
// run about to happen will write.
func nRunLogsAlreadyExist(ctx context.Context, n int) (context.Context, error) {
	root := stateHome(ctx)
	if root == "" {
		return ctx, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	dir := filepath.Join(root, "cherry-sync")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return ctx, fmt.Errorf("creating the log directory %s: %w", dir, err)
	}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	var planted []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("run-%s-%d.log", base.Add(time.Duration(i)*time.Minute).Format("20060102T150405Z"), 1000+i)
		err = os.WriteFile(filepath.Join(dir, name), []byte("seeded by the test harness\n"), 0o600)
		if err != nil {
			return ctx, fmt.Errorf("planting run log %s: %w", name, err)
		}
		planted = append(planted, name)
	}
	sort.Strings(planted)
	return context.WithValue(ctx, seededLogsKey{}, planted), nil
}

// aFileThatIsNotARunLogInTheLogDirectory plants a file csync did not write beside
// the run logs. The name deliberately sits close to the real thing — it begins
// "run-", ends ".log", and lacks only the process-id field — so a prune matching on
// either affix rather than the whole name treats it as a candidate.
//
// Its timestamp predates every planted log, which is what gives the scenario teeth:
// a loosely-matching prune sorts this file oldest and deletes it first, where a name
// sorting among the newest would survive such a prune by luck and leave the scenario
// passing for the wrong reason.
func aFileThatIsNotARunLogInTheLogDirectory(ctx context.Context) (context.Context, error) {
	root := stateHome(ctx)
	if root == "" {
		return ctx, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	dir := filepath.Join(root, "cherry-sync")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return ctx, fmt.Errorf("creating the log directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, "run-19990101T000000Z.log")
	err = os.WriteFile(path, []byte("not a run log\n"), 0o600)
	if err != nil {
		return ctx, fmt.Errorf("planting %s: %w", path, err)
	}
	return context.WithValue(ctx, plantedFileKey{}, path), nil
}

// theLogDirectoryShouldHoldNRunLogs asserts the directory holds exactly n run logs
// once the run is over — the ceiling, counted rather than reasoned about.
func theLogDirectoryShouldHoldNRunLogs(ctx context.Context, n int) error {
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	if len(names) != n {
		return fmt.Errorf("log directory holds %d run log(s), want %d: %v", len(names), n, names)
	}
	return nil
}

// theSurvivingRunLogsShouldBeTheNewestOnes asserts pruning kept the right logs, not
// merely the right number of them. Every planted log that survived must be newer
// than every planted log that did not, which is the property "keep the newest"
// means and which a prune in filesystem order would break while still counting out.
func theSurvivingRunLogsShouldBeTheNewestOnes(ctx context.Context) error {
	planted, _ := ctx.Value(seededLogsKey{}).([]string)
	if len(planted) == 0 {
		return fmt.Errorf("no run logs were planted, so there is no ordering to check")
	}
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	survived := make(map[string]bool, len(names))
	for _, n := range names {
		survived[n] = true
	}
	var kept, gone []string
	for _, n := range planted {
		if survived[n] {
			kept = append(kept, n)
		} else {
			gone = append(gone, n)
		}
	}
	if len(gone) == 0 {
		return fmt.Errorf("no planted run log was pruned, so nothing distinguishes newest from oldest; %d planted, %d present", len(planted), len(names))
	}
	if len(kept) == 0 {
		return fmt.Errorf("every planted run log was pruned; %d planted", len(planted))
	}
	// planted is sorted oldest first, so the survivors must be exactly its tail.
	if kept[0] < gone[len(gone)-1] {
		return fmt.Errorf("pruning kept %q but discarded the newer %q; kept %v, discarded %v", kept[0], gone[len(gone)-1], kept, gone)
	}
	return nil
}

// thatFileShouldStillBeThere asserts the planted not-a-run-log survived the prune.
func thatFileShouldStillBeThere(ctx context.Context) error {
	path, _ := ctx.Value(plantedFileKey{}).(string)
	if path == "" {
		return fmt.Errorf("no file was planted, so there is nothing to look for")
	}
	_, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("the planted file did not survive pruning: %w", err)
	}
	return nil
}

// thisRunsLog returns the log the run under test wrote: the one run log present that
// the scenario did not plant. Identifying it by difference rather than by age keeps
// the assertion off the ordering the pruning scenarios exist to test.
func thisRunsLog(ctx context.Context) (ParsedLog, string, error) {
	planted, _ := ctx.Value(seededLogsKey{}).([]string)
	seeded := make(map[string]bool, len(planted))
	for _, n := range planted {
		seeded[n] = true
	}
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return ParsedLog{}, "", err
	}
	var fresh []string
	for _, n := range names {
		if !seeded[n] {
			fresh = append(fresh, n)
		}
	}
	if len(fresh) != 1 {
		return ParsedLog{}, "", fmt.Errorf("want exactly one run log this run wrote, found %d: %v", len(fresh), fresh)
	}
	return parseLogAt(filepath.Join(stateHome(ctx), "cherry-sync", fresh[0]))
}

// theLogShouldNameTheRunLogsItPruned asserts the run accounted for the logs it
// deleted by name, and that the names it claims are really gone. Reconciling the
// two is what makes the record trustworthy: a run that listed a file it left behind
// would read as an explanation for an absence that never happened.
func theLogShouldNameTheRunLogsItPruned(ctx context.Context) error {
	log, content, err := thisRunsLog(ctx)
	if err != nil {
		return err
	}
	if len(log.Pruned) == 0 {
		return fmt.Errorf("run log names no pruned logs; contents:\n%s", content)
	}
	present, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	still := make(map[string]bool, len(present))
	for _, n := range present {
		still[n] = true
	}
	for _, n := range log.Pruned {
		if still[n] {
			return fmt.Errorf("run log claims to have pruned %q, but it is still there; contents:\n%s", n, content)
		}
	}
	return nil
}

// theLogShouldRecordThatNothingWasPruned asserts the record is present and says so,
// rather than being absent. An absent line cannot distinguish a run that pruned
// nothing from one whose pruning never ran.
func theLogShouldRecordThatNothingWasPruned(ctx context.Context) error {
	log, content, err := thisRunsLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasPruned {
		return fmt.Errorf("run log records no pruned line at all; contents:\n%s", content)
	}
	if len(log.Pruned) != 0 {
		return fmt.Errorf("run log names %d pruned log(s), want none; contents:\n%s", len(log.Pruned), content)
	}
	return nil
}

// assertPerm checks that path carries exactly the permission bits want, so a
// location or file whose bits drifted (a 0644 log, a 0755 directory) is caught.
func assertPerm(path string, want fs.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	got := info.Mode().Perm()
	if got != want {
		return fmt.Errorf("%s has permissions %#o, want %#o", path, got, want)
	}
	return nil
}

// copyTree recursively copies the file tree rooted at src into dst, recreating
// directories and file contents (permissions are normalized, not preserved).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		err = os.MkdirAll(filepath.Dir(target), 0o755)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
