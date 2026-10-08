// Steps asserting what csync wrote to the terminal and the status it exited
// with — reported operands, exit codes, help, version, license, error text.

package acceptance_test

import (
	"context"
	"fmt"
	"strings"
)

// asPlaceholder maps the scenario's real operands back to the "./project" and
// "user@host:/project" placeholders they stand in for. The harness substitutes
// throwaway directories for both, which no scenario can name.
func asPlaceholder(ctx context.Context, operand string) string {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local != "" && operand == local {
		return "./project"
	}
	remote := resolvedRemote(ctx)
	if remote != "" && operand == remote {
		return "user@host:/project"
	}
	return operand
}

// theReportedSourceShouldBe asserts the parsed "Source:" line equals want.
func theReportedSourceShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := asPlaceholder(ctx, parseOutput(r.Stdout, r.Stderr).Source)

	if got != want {
		return fmt.Errorf("Source: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedDestinationShouldBe asserts the parsed "Destination:" line equals want.
func theReportedDestinationShouldBe(ctx context.Context, want string) error {
	r := captured(ctx)
	got := asPlaceholder(ctx, parseOutput(r.Stdout, r.Stderr).Destination)

	if got != want {
		return fmt.Errorf("Destination: got %q, want %q in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// markExitAsserted records that the scenario has spoken for csync's exit status,
// so a non-zero exit is the scenario's own expectation rather than a failure.
func markExitAsserted(ctx context.Context) {
	check, _ := ctx.Value(exitCheckKey{}).(*exitCheck)
	if check != nil {
		check.asserted = true
	}
}

// unassertedFailure reports a csync run that exited non-zero in a scenario that
// never asserted on the exit status. Success is the default expectation, so a
// scenario only names the exit status when failing is part of what it describes.
func unassertedFailure(ctx context.Context) error {
	check, _ := ctx.Value(exitCheckKey{}).(*exitCheck)
	if check != nil && check.asserted {
		return nil
	}
	r, ok := ctx.Value(outputKey{}).(runResult)
	if !ok || r.ExitCode == 0 {
		return nil
	}
	return fmt.Errorf("csync exited %d, but the scenario expects success (stderr: %q)", r.ExitCode, r.Stderr)
}

// csyncShouldReturnExitCode asserts the captured process exit code equals want.
func csyncShouldReturnExitCode(ctx context.Context, want int) error {
	markExitAsserted(ctx)
	got := captured(ctx).ExitCode

	if got != want {
		return fmt.Errorf("exit code: got %d, want %d (stderr: %q)", got, want, captured(ctx).Stderr)
	}
	return nil
}

// csyncShouldReturnANonZeroExitCode asserts the captured exit code is non-zero
// (the error path, without pinning a specific code).
func csyncShouldReturnANonZeroExitCode(ctx context.Context) error {
	markExitAsserted(ctx)
	r := captured(ctx)

	if r.ExitCode == 0 {
		return fmt.Errorf("exit code: got 0, want non-zero (stdout: %q, stderr: %q)", r.Stdout, r.Stderr)
	}
	return nil
}

// theHelpTextShouldContain asserts a substring appears in what csync printed to
// stdout for `--help` — used to check the help summary carries its version
// header, sections, commands, and flags without pinning the full block (the
// view package's own tests guard the exact layout).
func theHelpTextShouldContain(ctx context.Context, want string) error {
	r := captured(ctx)
	if !strings.Contains(r.Stdout, want) {
		return fmt.Errorf("help output missing %q in stdout:\n%s", want, r.Stdout)
	}
	return nil
}

// aRemoteThatGoesSilentOnceTheComparisonIsDone points the scenario at the remote
// shell that answers the comparison and then stops answering, so the stall lands
// on the transfer rather than on the comparison. The comparison must be allowed
// to succeed: it is what produces the change list the scenario then chooses from,
// and csync never reaches a transfer without it.
func aRemoteThatGoesSilentOnceTheComparisonIsDone(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, stallModeKey{}, true), nil
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
