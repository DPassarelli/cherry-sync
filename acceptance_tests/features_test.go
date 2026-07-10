package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
type runningCsync struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *syncBuffer
	stderr *syncBuffer
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
	ctx.Step(`^the reported usage should begin with "([^"]*)"$`, theReportedUsageShouldBeginWith)
	ctx.Step(`^the reported message should begin with "([^"]*)"$`, theReportedMessageShouldBeginWith)
	ctx.Step(`^the reported version should be "([^"]*)"$`, theReportedVersionShouldBe)
	ctx.Step(`^the reported license should contain "([^"]*)"$`, theReportedLicenseShouldContain)
	ctx.Step(`^csync should report where it logged the run$`, csyncShouldReportWhereItLoggedTheRun)
	ctx.Step(`^a run log should exist at the reported path$`, aRunLogShouldExistAtTheReportedPath)
	ctx.Step(`^that a file has been changed locally$`, thatAFileHasBeenChangedLocally)
	ctx.Step(`^I have started csync but not yet answered the prompt$`, iHaveStartedCsyncButNotYetAnsweredThePrompt)
	// Two phrasings of the same act, kept apart because Gherkin reads better when a
	// Given narrates in the past and a When in the present. Both locate the log.
	ctx.Step(`^I look for the log file$`, iLocateTheLogFile)
	ctx.Step(`^I have taken note of where the log file is$`, iLocateTheLogFile)
	ctx.Step(`^the log file should already have content$`, theLogFileShouldAlreadyHaveContent)
	ctx.Step(`^I answer the prompt$`, iAnswerThePrompt)
	// A restatement of `csync should return exit code 0` in the vocabulary of a
	// scenario that has no interest in the number, only in csync having finished
	// what it was doing rather than falling over partway.
	ctx.Step(`^csync should exit normally$`, csyncShouldExitNormally)
	ctx.Step(`^the reported log path should be the one I found earlier$`, theReportedLogPathShouldBeTheOneIFoundEarlier)
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
		if run != nil && run.cmd.ProcessState == nil {
			run.stdin.Close()
			run.cmd.Process.Kill()
			run.cmd.Wait()
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
// RSYNC_RSH added under @remote. Later entries win over earlier duplicates (see
// exec.Cmd.Env), so the redirections override whatever the developer's shell had.
func csyncEnv(ctx context.Context) []string {
	env := os.Environ()
	home, _ := ctx.Value(homeKey{}).(string)
	if home != "" {
		env = append(env, "HOME="+home, "XDG_STATE_HOME="+stateHome(ctx))
	}
	remoteMode, _ := ctx.Value(remoteModeKey{}).(bool)
	if remoteMode {
		// rsync reads RSYNC_RSH as its remote shell; fakeRsh execs locally so the
		// `fakehost:` operand transfers on this machine over the real remote code
		// path. csync's own rsync child inherits this environment.
		env = append(env, "RSYNC_RSH="+fakeRsh)
	}
	return env
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

	cmd := exec.Command(csyncBinary, args...)
	cmd.Stdin = stdin
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = csyncEnv(ctx)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()

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

// theReportedUsageShouldBeginWith asserts the usage text parsed from stderr
// starts with want.
func theReportedUsageShouldBeginWith(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Usage

	if !strings.HasPrefix(got, want) {
		return fmt.Errorf("Usage: got %q, want prefix %q", got, want)
	}
	return nil
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
// run log it wrote. Nothing else in the suite may name that path: the scenarios
// learn it from csync, and one location scenario holds csync to putting it in the
// right place. A record nobody can find is not a record, so the disclosure is a
// behavior in its own right, not merely the seam this test reads through.
func csyncShouldReportWhereItLoggedTheRun(ctx context.Context) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).LogPath
	if got == "" {
		return fmt.Errorf("csync reported no log path in stdout:\n%s", r.Stdout)
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
		return fmt.Errorf("csync reported no log path, so there is none to look for; stdout:\n%s", r.Stdout)
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
	return theFileHasBeenChangedLocally(ctx, "README.md")
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
	run := &runningCsync{cmd: cmd, stdin: stdin, stdout: &syncBuffer{}, stderr: &syncBuffer{}}
	cmd.Stdout = run.stdout
	cmd.Stderr = run.stderr
	err = cmd.Start()
	if err != nil {
		return ctx, fmt.Errorf("start csync: %w", err)
	}
	// Stash before waiting: if the prompt never comes, the After hook still has the
	// child to kill.
	ctx = context.WithValue(ctx, startedKey{}, run)

	deadline := time.Now().Add(promptWait)
	for !strings.Contains(run.stderr.String(), selectionPrompt) {
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
	root := stateHome(ctx)
	if root == "" {
		return ctx, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return ctx, fmt.Errorf("searching %s for a run log: %w", root, err)
	}
	if len(found) != 1 {
		return ctx, fmt.Errorf("want exactly one run log under %s, found %d: %v", root, len(found), found)
	}
	return context.WithValue(ctx, foundLogKey{}, found[0]), nil
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
	err = run.cmd.Wait()
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
