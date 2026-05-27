package csync_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	ctx.Step(`^the reported direction should be "([^"]*)"$`, theReportedDirectionShouldBe)
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
	return assertReportedField(ctx, "Source", expected)
}

func theReportedDestinationShouldBe(ctx context.Context, expected string) error {
	return assertReportedField(ctx, "Destination", expected)
}

func theReportedDirectionShouldBe(ctx context.Context, expected string) error {
	return assertReportedField(ctx, "Direction", expected)
}

func assertReportedField(ctx context.Context, label, expected string) error {
	out, _ := ctx.Value(outputKey{}).(string)
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(label) + `:\s+(.+)\s*$`)
	match := re.FindStringSubmatch(out)
	if match == nil {
		return fmt.Errorf("no %q line in output:\n%s", label+":", out)
	}
	actual := strings.TrimSpace(match[1])
	if actual != expected {
		return fmt.Errorf("expected %s %q, got %q", label, expected, actual)
	}
	return nil
}
