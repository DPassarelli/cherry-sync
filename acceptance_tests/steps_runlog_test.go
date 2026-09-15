// Steps asserting what a run log records: the commands csync invoked, the
// changes it classified and selected, and the diagnostics of a failed pass.

package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// parseLogAt reads the run log at path and returns it parsed, along with the raw
// contents for error messages. It is the one place the log steps turn a path into a
// ParsedLog, so each reads structured fields rather than matching substrings.
func parseLogAt(path string) (ParsedLog, string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- path is the log this suite created under its own tempdir
	if err != nil {
		return ParsedLog{}, "", fmt.Errorf("run log at %q: %w", path, err)
	}
	return parseLog(string(content)), string(content), nil
}

// locatedLog returns the parsed log the scenario found under XDG_STATE_HOME (via
// `I look for the log file`), for the steps that read it while csync is still
// blocked at the prompt — before csync has disclosed the path itself.
func locatedLog(ctx context.Context) (ParsedLog, string, error) {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		return ParsedLog{}, "", fmt.Errorf("no run log was located; missing a step that looks for it?")
	}
	return parseLogAt(path)
}

// theLogShouldRecordThatTheVersionWas asserts the located log names the version
// csync ran as. It reads the file while csync is still blocked at the prompt, so a
// pass proves the version was recorded up front rather than at exit. The check ties
// the record to the known version the harness injected (see report-version): a log
// that named some other build, or named none, fails here.
func theLogShouldRecordThatTheVersionWas(ctx context.Context, want string) error {
	log, content, err := locatedLog(ctx)
	if err != nil {
		return err
	}
	if log.Version != want {
		return fmt.Errorf("run log records version %q, want %q; contents:\n%s", log.Version, want, content)
	}
	return nil
}

// theLogShouldRecordRunningForTheComparison asserts the located log names the
// external command csync ran to compare the two sides. Read while csync is blocked
// at the prompt, the comparison is the only command that has run, so an "exec
// <name>" record for it proves csync logs what it actually invoked — the fact a
// destructive run cannot be re-run to recover. It keys on the "exec <name>"
// pairing, not the name alone, so the name appearing elsewhere cannot satisfy it.
func theLogShouldRecordRunningForTheComparison(ctx context.Context, name string) error {
	log, content, err := locatedLog(ctx)
	if err != nil {
		return err
	}
	_, ok := log.command(name)
	if !ok {
		return fmt.Errorf("run log records no command %q; contents:\n%s", name, content)
	}
	return nil
}

// theLogShouldRecordTheTransferThatRan asserts a completed run logged the transfer
// pass, not only the comparison. Both are rsync, so the transfer surfaces as a second
// rsync record beyond the dry-run comparison: after the run finishes the log holds
// two, where at the prompt it held one. Keying on the count of rsync records rather
// than any flag keeps the check to the fidelity fact that matters — the transfer was
// recorded at all — and off the argv composition a refactor might change.
func theLogShouldRecordTheTransferThatRan(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	rsyncs := log.commands("rsync")
	if len(rsyncs) < 2 {
		return fmt.Errorf("run log records %d rsync command(s), want the comparison and the transfer; contents:\n%s", len(rsyncs), content)
	}
	return nil
}

// exitStatusRE pulls the numeric code out of the error csync surfaces when a command
// fails — an *exec.ExitError renders as "exit status N". The reconciliation reads it so
// the exit-code scenario need not hardcode rsync's failure code, which differs between
// rsync flavors.
var exitStatusRE = regexp.MustCompile(`exit status (\d+)`)

// theLogShouldRecordTheComparisonsFailingExitCode asserts the run log recorded the
// real, non-zero exit code of a comparison that failed at rsync — not a zero, not a
// placeholder. It reconciles against the code csync surfaced in its own error ("rsync:
// exit status N"), so the check needs no hardcoded rsync code: whatever csync saw, the
// log must show. A runner that logged a fixed exit=0, or dropped the process's real
// code, reddens here — which is what keeps the log honest about the runs worth reading.
func theLogShouldRecordTheComparisonsFailingExitCode(ctx context.Context) error {
	r := captured(ctx)
	m := exitStatusRE.FindStringSubmatch(r.Stderr)
	if m == nil {
		return fmt.Errorf("csync surfaced no rsync exit status to reconcile against; stderr:\n%s", r.Stderr)
	}
	want, _ := strconv.Atoi(m[1])
	if want == 0 {
		return fmt.Errorf("csync surfaced exit status 0, which is not a failure to test")
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	if cmd.ExitCode != want {
		return fmt.Errorf("run log records rsync exit=%d, want %d (what csync reported); contents:\n%s", cmd.ExitCode, want, content)
	}
	return nil
}

// diagnosticRE splits the error csync surfaces for a failed command into the exit
// status and whatever the command itself said about the failure. The remainder is
// matched rather than any particular wording because the two rsync implementations
// csync runs against word the same failure differently; what the scenarios pin is
// that rsync's account reaches the user at all, not what rsync chose to say.
var diagnosticRE = regexp.MustCompile(`exit status \d+: (.+)`)

// csyncShouldReportTheDiagnosticRsyncWrote asserts a failed comparison told the user
// what rsync said, not merely the code it exited with. A csync that reported the code
// alone leaves every ssh-layer failure looking identical — 255 is ssh's code, worn by a
// refused key, a changed host key and an unreachable host alike — so the sentence rsync
// wrote is the only part of the report anyone can act on.
func csyncShouldReportTheDiagnosticRsyncWrote(ctx context.Context) error {
	r := captured(ctx)
	m := diagnosticRE.FindStringSubmatch(r.Stderr)
	if m == nil || strings.TrimSpace(m[1]) == "" {
		return fmt.Errorf("csync reported an exit status with no account of the failure; stderr:\n%s", r.Stderr)
	}
	return nil
}

// theLogShouldRecordWhatRsyncSaidAboutTheFailure asserts the run log kept rsync's own
// account of a failed comparison, so the failure can still be diagnosed once the run is
// over and the terminal is gone. It reconciles against what csync printed rather than
// against any wording of its own: whatever the log kept must be text the user was also
// shown, which holds whichever rsync flavor produced it and however much of a long
// diagnostic the log chose to keep.
func theLogShouldRecordWhatRsyncSaidAboutTheFailure(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	if cmd.Stderr == "" {
		return fmt.Errorf("run log records no stderr for the rsync that failed; contents:\n%s", content)
	}
	r := captured(ctx)
	if !strings.Contains(r.Stderr, cmd.Stderr) {
		return fmt.Errorf("run log recorded stderr csync never reported:\nlogged: %q\nreported:\n%s", cmd.Stderr, r.Stderr)
	}
	return nil
}

// wholeMillisRE matches a duration expressed as a whole number of milliseconds with no
// fractional part — "44ms", not "43.7ms". The capture is the millisecond count, so the
// duration scenario can also check it is greater than zero.
var wholeMillisRE = regexp.MustCompile(`^(\d+)ms$`)

// resolvedLog returns the parsed log a scenario is asking about, from wherever it is
// available: the path a mid-run "look for the log file" step stashed, or else the one
// log under the scenario's state home. It locates the log without reading what csync
// disclosed, so that a scenario asking what a log contains fails only on the content it
// names; whether csync says where it logged is a separate behavior with its own
// scenarios, and coupling the two made one break redden both.
func resolvedLog(ctx context.Context) (ParsedLog, string, error) {
	path, _ := ctx.Value(foundLogKey{}).(string)
	if path == "" {
		found, err := theOneRunLog(ctx)
		if err != nil {
			return ParsedLog{}, "", err
		}
		path = found
	}
	return parseLogAt(path)
}

// theLogShouldRecordNClassifiedChanges asserts the log recorded the classification, and
// that its stated count and its list agree on how many changes csync found. Checking the
// count and the list length together catches a record whose header and body disagree.
func theLogShouldRecordNClassifiedChanges(ctx context.Context, n int) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasClassified {
		return fmt.Errorf("run log records no classified-changes line; contents:\n%s", content)
	}
	if log.ClassifiedCount != n || len(log.Classified) != n {
		return fmt.Errorf("run log records classified count=%d over a list of %d, want %d of each; contents:\n%s", log.ClassifiedCount, len(log.Classified), n, content)
	}
	return nil
}

// theLogShouldRecordNSelectedChanges asserts the log recorded the selection, count and
// list agreeing. HasSelected distinguishes "recorded that none were selected" (a real
// 0) from "never recorded a selection at all", so "record 0 selected changes" still
// demands the record be present.
func theLogShouldRecordNSelectedChanges(ctx context.Context, n int) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasSelected {
		return fmt.Errorf("run log records no selected-changes line; contents:\n%s", content)
	}
	if log.SelectedCount != n || len(log.Selected) != n {
		return fmt.Errorf("run log records selected count=%d over a list of %d, want %d of each; contents:\n%s", log.SelectedCount, len(log.Selected), n, content)
	}
	return nil
}

// theClassifiedChangesShouldInclude asserts a specific verb/path pair is among the
// changes csync recorded classifying — that the record names the actual changes, not
// just a count.
func theClassifiedChangesShouldInclude(ctx context.Context, verb, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.has(log.Classified, verb, path) {
		return fmt.Errorf("run log's classified changes do not include %s %q; got %+v; contents:\n%s", verb, path, log.Classified, content)
	}
	return nil
}

// theSelectedChangesShouldInclude asserts a specific verb/path pair is among the changes
// the user selected — the record that a removal, say, was actually taken and applied.
func theSelectedChangesShouldInclude(ctx context.Context, verb, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.has(log.Selected, verb, path) {
		return fmt.Errorf("run log's selected changes do not include %s %q; got %+v; contents:\n%s", verb, path, log.Selected, content)
	}
	return nil
}

// theLogShouldRecordAmongTheExcludedPaths asserts a specific gitignored path is named
// in the exclusion record — the point of #82's exclusion logging over a bare count: the
// log can answer whether a given file was held out of the comparison, not just how many
// were. It reads through the facade, which keeps the excluded names, so the assertion is
// on the recorded name rather than a substring of the raw line.
func theLogShouldRecordAmongTheExcludedPaths(ctx context.Context, path string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasExcluded {
		return fmt.Errorf("run log records no excluded line; contents:\n%s", content)
	}
	if slices.Contains(log.ExcludedGitignored, path) {
		return nil
	}
	return fmt.Errorf("run log's excluded paths do not include %q; got %+v; contents:\n%s", path, log.ExcludedGitignored, content)
}

// theLogShouldRecordThatTheGitDirectoryWasExcluded asserts the exclusion record notes
// the .git directory was withheld — the singleton exclusion csync always applies in a
// work tree, named for free (there is only ever one) alongside the gitignored paths.
func theLogShouldRecordThatTheGitDirectoryWasExcluded(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasExcluded {
		return fmt.Errorf("run log records no excluded line; contents:\n%s", content)
	}
	if !log.ExcludedGitDir {
		return fmt.Errorf("run log's excluded line does not note the .git directory; contents:\n%s", content)
	}
	return nil
}

// theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds asserts a recorded
// command's duration is present, decimal-free, and greater than zero — the shape a
// rounded-up whole-millisecond value takes. It reads the comparison's duration through
// the facade, which keeps the raw duration text, so this pins the rendered format: an
// unrounded "43.764397ms" fails the whole-millisecond match, and a "0ms" fails the
// greater-than-zero check that rounding up exists to uphold.
func theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	m := wholeMillisRE.FindStringSubmatch(cmd.Duration)
	if m == nil {
		return fmt.Errorf("run log records duration %q, want a whole number of milliseconds like %q; contents:\n%s", cmd.Duration, "44ms", content)
	}
	ms, _ := strconv.Atoi(m[1])
	if ms <= 0 {
		return fmt.Errorf("run log records duration %q, want greater than zero; contents:\n%s", cmd.Duration, content)
	}
	return nil
}

// theLogShouldRecordThatSourcePathAsOneArgument asserts the source operand — a path
// carrying a character the log format has to handle specially (a space, or the quote
// delimiter itself) — came back out of the log's argument vector as a single element,
// whole. It reconciles against the source csync reported (source of truth, so the check
// needs no knowledge of the tempdir) and reads the comparison's argv through the facade,
// whose parseLogArgs unquotes each token. The trailing slash rsync's operands carry is
// trimmed before the compare. Two ways the operand could fail to survive: joined with
// spaces it fractures (the space scenario's teeth), and wrapped without escaping the
// embedded quote closes the token early (the double-quote scenario's) — either leaves no
// element carrying the operand whole, which is the failure this reconciliation catches.
func theLogShouldRecordThatSourcePathAsOneArgument(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if !strings.ContainsAny(out.Source, " \"") {
		return fmt.Errorf("csync reported source %q, which has no space or quote to test fidelity with", out.Source)
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	cmd, ok := log.command("rsync")
	if !ok {
		return fmt.Errorf("run log records no rsync command; contents:\n%s", content)
	}
	for _, a := range cmd.Args {
		if strings.TrimSuffix(a, "/") == out.Source {
			return nil
		}
	}
	return fmt.Errorf("run log's rsync argv does not carry source %q as one argument; args=%q; contents:\n%s", out.Source, cmd.Args, content)
}

// theLogShouldRecordRunningForTheIgnoreRules asserts a run in a git work tree logged
// the git query csync made to learn what the repository ignores. Read after the run
// exits (identical sides, so it never prompts), the git command appears alongside the
// comparison — where a non-repo run logs no git at all, since the work-tree probe that
// gates it is deliberately left unlogged. It keys on the "exec <name>" pairing, like
// the comparison step, so the name appearing elsewhere cannot satisfy it.
func theLogShouldRecordRunningForTheIgnoreRules(ctx context.Context, name string) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	_, ok := log.command(name)
	if !ok {
		return fmt.Errorf("run log records no command %q; contents:\n%s", name, content)
	}
	return nil
}

// theLogShouldRecordTheRemovalThatRan asserts a deletion-only run logged the removal
// pass. Like the transfer, the removal is rsync, so it surfaces as a second rsync
// record beyond the dry-run comparison — but here the setup deletes rather than
// changes a file, so the second pass is the --delete removal, not a transfer (there
// is nothing to transfer). Counting the rsync records, rather than reading a flag,
// pins the presence-fidelity fact — the removal was recorded — and isolates it from
// the transfer scenario by what the run did.
func theLogShouldRecordTheRemovalThatRan(ctx context.Context) error {
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	rsyncs := log.commands("rsync")
	if len(rsyncs) < 2 {
		return fmt.Errorf("run log records %d rsync command(s), want the comparison and the removal; contents:\n%s", len(rsyncs), content)
	}
	return nil
}

// theLogShouldRecordTheCommandLineThatWasRun asserts the log's invocation line is
// the literal command csync was run with: "csync" followed by the argument vector
// runCsync actually passed (placeholders already substituted). Reconciling against
// the stashed argv keeps the check honest without the scenario reconstructing the
// tempdir substitution, and pins that the line is the raw invocation — not the
// resolved operands, which get their own lines.
func theLogShouldRecordTheCommandLineThatWasRun(ctx context.Context) error {
	args, _ := ctx.Value(invokedArgsKey{}).([]string)
	want := strings.Join(append([]string{"csync"}, args...), " ")
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if log.Invocation != want {
		return fmt.Errorf("run log records invocation %q, want %q; contents:\n%s", log.Invocation, want, content)
	}
	return nil
}

// theLogShouldNameTheSourceAndDestinationReported asserts the log records both
// operands, and that they are the same source and destination csync printed in its
// header. Reconciling the two is what makes the check honest: csync is the source of
// truth for what the operands resolved to, so a log that named some other path, or
// named none, fails here — without the scenario having to know the tempdir layout.
func theLogShouldNameTheSourceAndDestinationReported(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if out.Source == "" || out.Destination == "" {
		return fmt.Errorf("csync reported an empty operand (source %q, destination %q); nothing to reconcile", out.Source, out.Destination)
	}
	log, content, err := resolvedLog(ctx)
	if err != nil {
		return err
	}
	if log.Source != out.Source {
		return fmt.Errorf("run log names source %q, want %q (what csync reported); contents:\n%s", log.Source, out.Source, content)
	}
	if log.Destination != out.Destination {
		return fmt.Errorf("run log names destination %q, want %q (what csync reported); contents:\n%s", log.Destination, out.Destination, content)
	}
	return nil
}
