package cli_test

import (
	"testing"

	"github.com/dpassarelli/cherry-sync/internal/cli"
)

// Behavior: given two positional arguments, Parse returns them as the
// Source and Destination of an Args value. Mirrors the Gherkin scenario in
// features/invoke-command.feature ("Push direction").
func TestParse_ExtractsSourceAndDestination(t *testing.T) {
	got := cli.Parse([]string{"./project", "user@host:/project"})

	want := cli.Args{
		Source:      "./project",
		Destination: "user@host:/project",
	}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}
