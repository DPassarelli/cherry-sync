// Steps asserting the comparison csync reported: which actions, in what order,
// what was counted or excluded, and the resulting state of each file.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// noActionsShouldBeReported asserts csync's output lists zero actions.
func noActionsShouldBeReported(ctx context.Context) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	if len(got) != 0 {
		return fmt.Errorf("Actions: got %d (%+v), want 0", len(got), got)
	}
	return nil
}

// theReportedActionsShouldBe asserts the reported actions match the table,
// order-insensitively — both sides are sorted before comparison. For an
// order-sensitive check see theReportedActionsShouldBeInOrder.
func theReportedActionsShouldBe(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	gotSorted := sortActions(verbPath(got))
	wantSorted := sortActions(verbPath(want))
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		return fmt.Errorf("Actions: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theWithheldChangesShouldBe asserts the "Withheld (gitignored):" block lists
// exactly the table's rows — the changes csync found but declined to offer because
// the path is gitignored. Order-insensitive, like its counterpart for the action
// list.
func theWithheldChangesShouldBe(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Withheld

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	gotSorted := sortActions(verbPath(got))
	wantSorted := sortActions(verbPath(want))
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		return fmt.Errorf("Withheld: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// noWithheldChangesShouldBeReported asserts csync named no withheld change at all:
// an empty block is as much a failure as a populated one, since a heading over
// nothing still tells the user something was held back.
func noWithheldChangesShouldBeReported(ctx context.Context) error {
	r := captured(ctx)
	out := parseOutput(r.Stdout, r.Stderr)
	if out.HasWithheldBlock || len(out.Withheld) > 0 {
		return fmt.Errorf("expected no withheld changes, got %+v in output:\n%s", out.Withheld, r.Stdout)
	}
	return nil
}

// theReportedActionsShouldBeInOrder asserts the reported actions match the
// table exactly, including sequence — unlike theReportedActionsShouldBe, which
// is order-insensitive. Used by scenarios that pin the display ordering.
func theReportedActionsShouldBeInOrder(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(verbPath(got), verbPath(want)) {
		return fmt.Errorf("Actions (in order): got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// theReportedChangesShouldBeNumberedInOrder asserts the reported changes match
// the table exactly — sequence and the visible 1-based selection number — so a
// user typing "1" at the prompt picks the first row. Unlike
// theReportedActionsShouldBeInOrder, this compares the rendered Index too.
func theReportedChangesShouldBeNumberedInOrder(ctx context.Context, table *godog.Table) error {
	r := captured(ctx)
	got := parseOutput(r.Stdout, r.Stderr).Actions

	want, err := actionsFromTable(table)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("Numbered changes: got %+v, want %+v in output:\n%s", got, want, r.Stdout)
	}
	return nil
}

// actionsFromTable reads a Gherkin table with "action" and "path" columns into
// a slice of Actions, preserving row order. An optional "number" column sets
// each Action's selection Index; absent it, Index is left 0 (the verb/path-only
// tables read by the order-agnostic steps).
func actionsFromTable(table *godog.Table) ([]Action, error) {
	headers := map[string]int{}
	for i, cell := range table.Rows[0].Cells {
		headers[cell.Value] = i
	}
	actionCol, ok1 := headers["action"]
	pathCol, ok2 := headers["path"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("expected 'action' and 'path' columns; got headers: %+v", headers)
	}
	numberCol, hasNumber := headers["number"]

	var actions []Action
	for _, row := range table.Rows[1:] {
		act := Action{
			Verb: row.Cells[actionCol].Value,
			Path: row.Cells[pathCol].Value,
		}
		if hasNumber {
			n, err := strconv.Atoi(strings.TrimSpace(row.Cells[numberCol].Value))
			if err != nil {
				return nil, fmt.Errorf("number column: %w", err)
			}
			act.Index = n
		}
		actions = append(actions, act)
	}
	return actions, nil
}

// verbPath returns a copy of a with each Action's selection Index cleared, for
// the steps that assert verb and path only and are indifferent to the displayed
// numbering. theReportedChangesShouldBeNumberedInOrder compares Index directly.
func verbPath(a []Action) []Action {
	out := make([]Action, len(a))
	for i, x := range a {
		out[i] = Action{Verb: x.Verb, Path: x.Path}
	}
	return out
}

// sortActions returns a copy of a sorted by verb then path, so two action
// slices can be compared regardless of their original order.
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

// theReportedChangeCountShouldBe asserts csync printed a "Changes:" line and
// that its count equals want.
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

// theReportedExcludedCountShouldBe asserts csync printed an exclusion disclosure
// and that its count equals want. The "(excluding …)" line is the user's only
// signal that ignored paths were hidden, so its absence (HasExcludedCount false)
// is itself a failure.
func theReportedExcludedCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasExcludedCount {
		return fmt.Errorf("no exclusion disclosure in output:\n%s", r.Stdout)
	}
	if parsed.ExcludedCount != want {
		return fmt.Errorf("excluded count: got %d, want %d in output:\n%s", parsed.ExcludedCount, want, r.Stdout)
	}
	return nil
}

// noGitignoredPathsShouldBeReportedAsExcluded asserts csync printed no exclusion
// disclosure at all — the "(excluding …)" aside is omitted entirely when nothing
// was hidden, so a non-repo (or empty-ignore) sync stays free of empty-exclusion
// noise.
func noGitignoredPathsShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if parsed.HasExcludedCount {
		return fmt.Errorf("exclusion disclosure present (count %d) but none expected in output:\n%s", parsed.ExcludedCount, r.Stdout)
	}
	return nil
}

// theGitDirectoryShouldBeReportedAsExcluded asserts csync's exclusion disclosure
// announces the .git directory. git never lists .git/ as ignored, so this
// disclosure is the user's only signal that the VCS metadata dir was held back.
func theGitDirectoryShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.ExcludedGitDir {
		return fmt.Errorf("expected the .git directory to be reported as excluded, but it was not, in output:\n%s", r.Stdout)
	}
	return nil
}

// theCsyncTomlShouldBeReportedAsExcluded asserts csync's exclusion disclosure
// announces that its own .csync.toml was held back. The config file is withheld from every
// sync with no opt-out, so — like the .git disclosure — this line is the user's
// only signal it was excluded rather than offered for transfer.
func theCsyncTomlShouldBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.ExcludedCsyncToml {
		return fmt.Errorf("expected the .csync.toml file to be reported as excluded, but it was not, in output:\n%s", r.Stdout)
	}
	return nil
}

// theGitDirectoryShouldNotBeReportedAsExcluded asserts csync did NOT announce a
// .git exclusion — the case where the local side is not a git work tree, so there
// is no .git/ to exclude and nothing to disclose.
func theGitDirectoryShouldNotBeReportedAsExcluded(ctx context.Context) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if parsed.ExcludedGitDir {
		return fmt.Errorf("the .git directory was reported as excluded but should not be, in output:\n%s", r.Stdout)
	}
	return nil
}

// theReportedSyncCountShouldBe asserts csync printed a "Synced:" line and that
// its count equals want.
func theReportedSyncCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasSyncCount {
		return fmt.Errorf("no Synced line in output:\n%s", r.Stdout)
	}
	if parsed.SyncCount != want {
		return fmt.Errorf("Synced: got %d, want %d in output:\n%s", parsed.SyncCount, want, r.Stdout)
	}
	return nil
}

// theReportedRemovedCountShouldBe asserts the post-sync summary called out
// removals distinctly ("… M of which were removed") and that M equals want. It
// reads the removal count specifically, not the total files count that
// theReportedSyncCountShouldBe checks.
func theReportedRemovedCountShouldBe(ctx context.Context, want int) error {
	r := captured(ctx)
	parsed := parseOutput(r.Stdout, r.Stderr)

	if !parsed.HasRemovedCount {
		return fmt.Errorf("no removal clause in summary:\n%s", r.Stdout)
	}
	if parsed.RemovedCount != want {
		return fmt.Errorf("removed: got %d, want %d in output:\n%s", parsed.RemovedCount, want, r.Stdout)
	}
	return nil
}

// theFileShouldBeIdenticalBetweenLocalAndRemote asserts the named file has the
// same bytes on both sides — i.e. the transfer actually moved it.
func theFileShouldBeIdenticalBetweenLocalAndRemote(ctx context.Context, relPath string) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)

	localBytes, err := os.ReadFile(filepath.Join(local, relPath))
	if err != nil {
		return fmt.Errorf("read local %s: %w", relPath, err)
	}
	remoteBytes, err := os.ReadFile(filepath.Join(remote, relPath))
	if err != nil {
		return fmt.Errorf("read remote %s: %w", relPath, err)
	}
	if !bytes.Equal(localBytes, remoteBytes) {
		return fmt.Errorf("file %q differs: local %q, remote %q", relPath, localBytes, remoteBytes)
	}
	return nil
}

// theFileShouldStillDifferBetweenLocalAndRemote asserts the named file's bytes
// differ across the two sides — i.e. a change that wasn't selected was left
// untransferred (the file exists on both sides, but the remote is still stale).
func theFileShouldStillDifferBetweenLocalAndRemote(ctx context.Context, relPath string) error {
	local, _ := ctx.Value(localPathKey{}).(string)
	remote, _ := ctx.Value(remotePathKey{}).(string)

	localBytes, err := os.ReadFile(filepath.Join(local, relPath))
	if err != nil {
		return fmt.Errorf("read local %s: %w", relPath, err)
	}
	remoteBytes, err := os.ReadFile(filepath.Join(remote, relPath))
	if err != nil {
		return fmt.Errorf("read remote %s: %w", relPath, err)
	}
	if bytes.Equal(localBytes, remoteBytes) {
		return fmt.Errorf("file %q is identical on both sides but should still differ", relPath)
	}
	return nil
}

// theFileShouldNotExistOnTheRemote asserts the named file is absent on the
// remote side — i.e. a change that wasn't selected was not transferred.
func theFileShouldNotExistOnTheRemote(ctx context.Context, relPath string) error {
	remote, _ := ctx.Value(remotePathKey{}).(string)

	_, err := os.Stat(filepath.Join(remote, relPath))
	if err == nil {
		return fmt.Errorf("file %q exists on the remote but should not", relPath)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat remote %s: %w", relPath, err)
	}
	return nil
}

// theFileShouldStillExistOnTheRemote asserts the named file is present on the
// remote side — i.e. a reported deletion was not applied (it was left unselected,
// or the run only reported changes without transferring). The mirror of
// theFileShouldNotExistOnTheRemote.
func theFileShouldStillExistOnTheRemote(ctx context.Context, relPath string) error {
	remote, _ := ctx.Value(remotePathKey{}).(string)

	_, err := os.Stat(filepath.Join(remote, relPath))
	if os.IsNotExist(err) {
		return fmt.Errorf("file %q is absent on the remote but should still exist", relPath)
	}
	if err != nil {
		return fmt.Errorf("stat remote %s: %w", relPath, err)
	}
	return nil
}
