package csync_test

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
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// csyncBinary is the path to the compiled csync binary, set by TestMain
// before any scenario runs.
var csyncBinary string

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

// runResult holds everything the test world cares about after a csync
// invocation: the two output streams kept separate so step funcs can assert
// against the right one, plus the process exit code.
type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
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
	build := exec.Command("go", "build", "-o", csyncBinary, "./cmd/csync")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	err = build.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(2)
	}

	os.Exit(m.Run())
}

// TestFeatures runs the godog feature suite under `go test`, excluding
// @wip-tagged features. A non-zero suite result fails the test.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"features"},
			Strict: true,
			// Exclude features tagged @wip — scenarios drafted ahead of their
			// step definitions and production code. Drop the tag on a feature
			// to bring it into the run.
			Tags:     "~@wip",
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
	ctx.Step(`^I run "([^"]*)" and respond with "([^"]*)"$`, iRunAndRespond)
	ctx.Step(`^the reported source should be "([^"]*)"$`, theReportedSourceShouldBe)
	ctx.Step(`^the reported destination should be "([^"]*)"$`, theReportedDestinationShouldBe)
	ctx.Step(`^csync should return exit code (\d+)$`, csyncShouldReturnExitCode)
	ctx.Step(`^csync should return a non-zero exit code$`, csyncShouldReturnANonZeroExitCode)
	ctx.Step(`^the reported usage should begin with "([^"]*)"$`, theReportedUsageShouldBeginWith)
	ctx.Step(`^the reported message should begin with "([^"]*)"$`, theReportedMessageShouldBeginWith)
	ctx.Step(`^a local directory containing these files:$`, aLocalDirectoryContainingTheseFiles)
	ctx.Step(`^that all of the files are identical between local and remote$`, allFilesIdenticalBetweenLocalAndRemote)
	ctx.Step(`^an empty remote directory$`, anEmptyRemoteDirectory)
	ctx.Step(`^that the file "([^"]*)" has been changed locally$`, theFileHasBeenChangedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added locally$`, theFileHasBeenAddedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added on the remote$`, theFileHasBeenAddedOnTheRemote)
	ctx.Step(`^no actions should be reported$`, noActionsShouldBeReported)
	ctx.Step(`^the reported actions should be:$`, theReportedActionsShouldBe)
	ctx.Step(`^the reported actions should be, in order:$`, theReportedActionsShouldBeInOrder)
	ctx.Step(`^the reported change count should be (\d+)$`, theReportedChangeCountShouldBe)
	ctx.Step(`^the reported sync count should be (\d+)$`, theReportedSyncCountShouldBe)
	ctx.Step(`^the file "([^"]*)" should be identical between local and remote$`, theFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^the file "([^"]*)" should not exist on the remote$`, theFileShouldNotExistOnTheRemote)
	ctx.Step(`^the file "([^"]*)" should still differ between local and remote$`, theFileShouldStillDifferBetweenLocalAndRemote)

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		localPath, _ := ctx.Value(localPathKey{}).(string)
		if localPath != "" {
			os.RemoveAll(localPath)
		}
		remotePath, _ := ctx.Value(remotePathKey{}).(string)
		if remotePath != "" {
			os.RemoveAll(remotePath)
		}
		return ctx, nil
	})
}

// iRun executes the `When I run "..."` step with no interactive input.
func iRun(ctx context.Context, command string) (context.Context, error) {
	return runCsync(ctx, command, nil)
}

// iRunAndRespond executes the `When I run "..." and respond with "..."` step,
// feeding the response to csync on stdin as if typed at the prompt. The
// `<empty>` sentinel stands for an empty response (a bare Enter); a trailing
// newline is appended so the response reads as a completed line.
func iRunAndRespond(ctx context.Context, command, response string) (context.Context, error) {
	if response == "<empty>" {
		response = ""
	}
	return runCsync(ctx, command, strings.NewReader(response+"\n"))
}

// runCsync splits the command, substitutes the Gherkin placeholders
// (`./project`, `user@host:/project`, `<empty>`) with the scenario's real
// tempdir paths, runs the csync binary with the given stdin, and stashes the
// captured streams and exit code in the context.
func runCsync(ctx context.Context, command string, stdin io.Reader) (context.Context, error) {
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
	remotePath, _ := ctx.Value(remotePathKey{}).(string)
	if remotePath != "" {
		subs["user@host:/project"] = remotePath
	}
	for i, a := range args {
		replacement, ok := subs[a]
		if ok {
			args[i] = replacement
		}
	}

	cmd := exec.Command(csyncBinary, args...)
	cmd.Stdin = stdin
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

// aLocalDirectoryContainingTheseFiles creates a local tempdir populated with
// the (empty) files named in the DocString and stashes its path under
// localPathKey.
func aLocalDirectoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(ds.Content), "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" {
			continue
		}
		full := filepath.Join(dir, rel)
		err = os.MkdirAll(filepath.Dir(full), 0o755)
		if err != nil {
			return ctx, fmt.Errorf("mkdir: %w", err)
		}
		err = os.WriteFile(full, []byte(""), 0o644)
		if err != nil {
			return ctx, fmt.Errorf("write %s: %w", full, err)
		}
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
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

	gotSorted := sortActions(got)
	wantSorted := sortActions(want)
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

	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("Actions (in order): got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// actionsFromTable reads a Gherkin table with "action" and "path" columns into
// a slice of Actions, preserving row order.
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

	var actions []Action
	for _, row := range table.Rows[1:] {
		actions = append(actions, Action{
			Verb: row.Cells[actionCol].Value,
			Path: row.Cells[pathCol].Value,
		})
	}
	return actions, nil
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
