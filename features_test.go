package csync_test

import (
	"bytes"
	"context"
	"fmt"
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

// localPathKey and remotePathKey stash the per-scenario tempdir paths set
// up by `Given a local directory ...` and `Given that all of the files are
// identical between local and remote`. iRun reads them to substitute the
// Gherkin placeholders `./project` and `user@host:/project` with the real
// paths before invoking csync.
type localPathKey struct{}
type remotePathKey struct{}

// runResult holds everything the test world cares about after a csync
// invocation: the two output streams kept separate so step funcs can assert
// against the right one, plus the process exit code.
type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

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

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Strict:   true,
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^I run "([^"]*)"$`, iRun)
	ctx.Step(`^the reported source should be "([^"]*)"$`, theReportedSourceShouldBe)
	ctx.Step(`^the reported destination should be "([^"]*)"$`, theReportedDestinationShouldBe)
	ctx.Step(`^csync should return exit code (\d+)$`, csyncShouldReturnExitCode)
	ctx.Step(`^the reported usage should begin with "([^"]*)"$`, theReportedUsageShouldBeginWith)
	ctx.Step(`^a local directory containing these files:$`, aLocalDirectoryContainingTheseFiles)
	ctx.Step(`^that all of the files are identical between local and remote$`, allFilesIdenticalBetweenLocalAndRemote)
	ctx.Step(`^that the file "([^"]*)" has been changed locally$`, theFileHasBeenChangedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added locally$`, theFileHasBeenAddedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added on the remote$`, theFileHasBeenAddedOnTheRemote)
	ctx.Step(`^no actions should be reported$`, noActionsShouldBeReported)
	ctx.Step(`^the reported actions should be:$`, theReportedActionsShouldBe)
	ctx.Step(`^the reported change count should be (\d+)$`, theReportedChangeCountShouldBe)

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

func iRun(ctx context.Context, command string) (context.Context, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ctx, fmt.Errorf("empty command")
	}
	if parts[0] != "csync" {
		return ctx, fmt.Errorf("expected command to start with %q, got %q", "csync", parts[0])
	}

	args := parts[1:]
	subs := map[string]string{}
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

func theReportedSourceShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Source

	if got != want {
		return fmt.Errorf("Source: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

func theReportedDestinationShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Destination

	if got != want {
		return fmt.Errorf("Destination: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

func csyncShouldReturnExitCode(ctx context.Context, want int) error {
	got := captured(ctx).ExitCode

	if got != want {
		return fmt.Errorf("exit code: got %d, want %d (stderr: %q)", got, want, captured(ctx).Stderr)
	}
	return nil
}

func theReportedUsageShouldBeginWith(ctx context.Context, want string) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Usage

	if !strings.HasPrefix(got, want) {
		return fmt.Errorf("Usage: got %q, want prefix %q", got, want)
	}
	return nil
}

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

func noActionsShouldBeReported(ctx context.Context) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	if len(got) != 0 {
		return fmt.Errorf("Actions: got %d (%+v), want 0", len(got), got)
	}
	return nil
}

func theReportedActionsShouldBe(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	headers := map[string]int{}
	for i, cell := range table.Rows[0].Cells {
		headers[cell.Value] = i
	}
	actionCol, ok1 := headers["action"]
	pathCol, ok2 := headers["path"]
	if !ok1 || !ok2 {
		return fmt.Errorf("expected 'action' and 'path' columns; got headers: %+v", headers)
	}

	var want []Action
	for _, row := range table.Rows[1:] {
		want = append(want, Action{
			Verb: row.Cells[actionCol].Value,
			Path: row.Cells[pathCol].Value,
		})
	}

	gotSorted := sortActions(got)
	wantSorted := sortActions(want)
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		return fmt.Errorf("Actions: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

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

func captured(ctx context.Context) runResult {
	r, _ := ctx.Value(outputKey{}).(runResult)
	return r
}

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
