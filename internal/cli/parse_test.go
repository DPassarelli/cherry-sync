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

// Behavior: --version selects Version mode. Mirrors the Gherkin scenario "The
// --version flag prints the version and exits successfully" in
// features/report-version.feature; main.go turns Version mode into the printed
// version line.
func TestParse_Version_SelectsVersionMode(t *testing.T) {
	got, err := cli.Parse([]string{"--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.Version}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: --version short-circuits — it wins over trailing operands rather
// than being rejected as the wrong argument count. Mirrors the Gherkin scenario
// "--version short-circuits any operands". Without the short-circuit these three
// arguments would fail the two-operand check.
func TestParse_Version_ShortCircuitsOperands(t *testing.T) {
	got, err := cli.Parse([]string{"--version", "./project", "user@host:/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.Version}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: --license selects License mode. Mirrors the Gherkin scenario "The
// --license flag prints the license and exits successfully" in
// features/report-license.feature; main.go turns License mode into the printed
// license text.
func TestParse_License_SelectsLicenseMode(t *testing.T) {
	got, err := cli.Parse([]string{"--license"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.License}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: --license short-circuits — like --version it wins over trailing
// operands rather than being rejected as the wrong argument count. Mirrors the
// Gherkin scenario "--license short-circuits any operands".
func TestParse_License_ShortCircuitsOperands(t *testing.T) {
	got, err := cli.Parse([]string{"--license", "./project", "user@host:/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.License}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: an empty-string path is rejected. Left unchecked, "" + "/" = "/"
// would point rsync at the filesystem root. Mirrors the Gherkin scenario
// "Empty path argument — show usage and exit non-zero" in
// features/invoke-command.feature. Either position counts.
func TestParse_EmptyPath_ReturnsError(t *testing.T) {
	cases := map[string][]string{
		"empty source":      {"", "user@host:/project"},
		"empty destination": {"./project", ""},
	}
	for name, argv := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := cli.Parse(argv)

			if err == nil {
				t.Fatalf("expected error for %v, got nil", argv)
			}
		})
	}
}
