package cli_test

import (
	"strings"
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

// Behavior: --help selects Help mode. Mirrors the Gherkin scenario "The --help
// flag prints usage to stdout and exits successfully" in
// features/show-help.feature; main.go turns Help mode into the printed usage.
func TestParse_Help_SelectsHelpMode(t *testing.T) {
	got, err := cli.Parse([]string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.Help}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: the "-h" short flag selects Help mode, the same as "--help". Mirrors
// the Gherkin scenario "The -h alias behaves like --help".
func TestParse_HelpShortFlag_SelectsHelpMode(t *testing.T) {
	got, err := cli.Parse([]string{"-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.Help}
	if got != want {
		t.Errorf("parsed args mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

// Behavior: --help short-circuits — like --version it wins over trailing
// operands rather than being rejected as the wrong argument count. Mirrors the
// Gherkin scenario "--help short-circuits any operands".
func TestParse_Help_ShortCircuitsOperands(t *testing.T) {
	got, err := cli.Parse([]string{"--help", "./project", "user@host:/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := cli.Args{Mode: cli.Help}
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

// Behavior: a lone argument that can't be a path is reported as a mistyped
// command, so csync points at push/pull rather than complaining about operand
// count. This pins the classification branch, not the exact copy (the acceptance
// scenario "A mistyped command is reported as such" gates the wording).
func TestParse_LoneWord_ReportedAsMistypedCommand(t *testing.T) {
	_, err := cli.Parse([]string{"pill"})

	if err == nil {
		t.Fatal("expected error for a lone non-path argument, got nil")
	}
	if !strings.Contains(err.Error(), "is not a command") {
		t.Errorf("error should classify %q as a mistyped command, got: %v", "pill", err)
	}
}

// Behavior: a lone argument shaped like a path is reported as a missing-operand
// error (both paths required), NOT a mistyped command — the two messages split
// on whether the argument could be a path. Guards the heuristic against
// over-triggering; mirrors the scenario "One path only — report that both are
// required".
func TestParse_LonePath_ReportedAsMissingOperand(t *testing.T) {
	_, err := cli.Parse([]string{"./project"})

	if err == nil {
		t.Fatal("expected error for a single path argument, got nil")
	}
	if strings.Contains(err.Error(), "is not a command") {
		t.Errorf("a path-shaped argument should not be called a command, got: %v", err)
	}
}
