package csync_test

import (
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures is the single entry point that `go test` invokes to run the
// Gherkin scenarios. godog discovers .feature files at the paths listed below
// and matches each step against the regexes registered in InitializeScenario.
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

func iRun(command string) error {
	return godog.ErrPending
}

func theReportedSourceShouldBe(expected string) error {
	return godog.ErrPending
}

func theReportedDestinationShouldBe(expected string) error {
	return godog.ErrPending
}

func theReportedDirectionShouldBe(expected string) error {
	return godog.ErrPending
}
