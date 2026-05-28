package cli_test

import (
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/cli"
)

// Behavior: given two positional arguments, Parse returns them as the
// Source and Destination of an Args value. Mirrors the Gherkin scenario in
// features/invoke-command.feature ("Push direction").
func TestParse_ExtractsSourceAndDestination(t *testing.T) {
	got, err := cli.Parse([]string{"./project", "user@host:/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{
		Source:      "./project",
		Destination: "user@host:/project",
	}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: with no arguments, Parse returns an error. Mirrors the Gherkin
// scenario "No arguments — show usage and exit non-zero" in
// features/invoke-command.feature; main.go turns the error into the
// user-facing usage message.
func TestParse_NoArguments_ReturnsError(t *testing.T) {
	_, err := cli.Parse([]string{})

	if err == nil {
		t.Fatal("expected error for empty argv, got nil")
	}
}
