package csync_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// csyncBinary is the path to the compiled csync binary, set by TestMain
// before any scenario runs.
var csyncBinary string

// outputKey is the context key used to stash the captured stdout+stderr
// from `When I run "..."` so the following Then steps can assert on it.
type outputKey struct{}

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
	if err := build.Run(); err != nil {
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
			Paths:    []string{"features/invoke-command.feature"},
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
}

func iRun(ctx context.Context, command string) (context.Context, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ctx, fmt.Errorf("empty command")
	}
	if parts[0] != "csync" {
		return ctx, fmt.Errorf("expected command to start with %q, got %q", "csync", parts[0])
	}

	out, _ := exec.Command(csyncBinary, parts[1:]...).CombinedOutput()
	return context.WithValue(ctx, outputKey{}, string(out)), nil
}

func theReportedSourceShouldBe(ctx context.Context, expected string) error {
	raw := captured(ctx)
	actual := parseStdout(raw).Source

	if actual != expected {
		return fmt.Errorf("expected Source %q, got %q in output:\n%s", expected, actual, raw)
	}
	return nil
}

func theReportedDestinationShouldBe(ctx context.Context, expected string) error {
	raw := captured(ctx)
	actual := parseStdout(raw).Destination

	if actual != expected {
		return fmt.Errorf("expected Destination %q, got %q in output:\n%s", expected, actual, raw)
	}
	return nil
}

func captured(ctx context.Context) string {
	s, _ := ctx.Value(outputKey{}).(string)
	return s
}
