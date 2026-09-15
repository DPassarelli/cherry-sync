// Steps for run-log pruning: seeding a log directory, and asserting which logs
// survived, which were removed, and what the current run recorded about it.

package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// logsUnder returns every regular file beneath root, in walk order. A root that
// does not exist yields no files rather than an error: a run that wrote no log
// leaves the state directory uncreated, which is the same observation as an empty
// one. It underlies the log-location assertions — none, exactly one, and under a
// named directory.
func logsUnder(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// singleLogUnder returns the one run log beneath root, or an error naming what it
// found instead. Insisting on exactly one is what lets a caller speak of "the run
// log" — with two under the state directory the phrase would mean nothing.
func singleLogUnder(root string) (string, error) {
	logs, err := logsUnder(root)
	if err != nil {
		return "", fmt.Errorf("searching %s for a run log: %w", root, err)
	}
	if len(logs) != 1 {
		return "", fmt.Errorf("want exactly one run log under %s, found %d: %v", root, len(logs), logs)
	}
	return logs[0], nil
}

// theOneRunLog returns the single run log under the scenario's XDG_STATE_HOME, the
// path the assertions that inspect a log's location or permissions read through.
func theOneRunLog(ctx context.Context) (string, error) {
	root := stateHome(ctx)
	if root == "" {
		return "", fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	return singleLogUnder(root)
}

// runLogNameRE matches the names csync gives its run logs. The pruning steps count
// and compare through it rather than through logsUnder, which returns every regular
// file: a scenario that plants a foreign file in the log directory would otherwise
// see it counted as a log, and the scenario asserting that foreign files survive
// would pass for the wrong reason.
var runLogNameRE = regexp.MustCompile(`^run-\d{8}T\d{6}Z-\d+\.log$`)

// runLogsUnder returns the names of the run logs directly beneath the log directory
// in root, sorted oldest first. Sorting by name is sorting by start time: the
// timestamp is fixed-width and leads the name, so lexical order is chronological.
func runLogsUnder(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "cherry-sync"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading the log directory under %s: %w", root, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && runLogNameRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// scenarioRunLogs returns the run logs presently under the scenario's state home,
// oldest first.
func scenarioRunLogs(ctx context.Context) ([]string, error) {
	root := stateHome(ctx)
	if root == "" {
		return nil, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	return runLogsUnder(root)
}

// nRunLogsAlreadyExist plants n run logs in the log directory, named as csync names
// its own so that pruning treats them as candidates. Their timestamps run from a
// fixed date well in the past, one minute apart and ascending, so the set has an
// unambiguous oldest-to-newest order and every one of them is older than the log the
// run about to happen will write.
func nRunLogsAlreadyExist(ctx context.Context, n int) (context.Context, error) {
	root := stateHome(ctx)
	if root == "" {
		return ctx, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	dir := filepath.Join(root, "cherry-sync")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return ctx, fmt.Errorf("creating the log directory %s: %w", dir, err)
	}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	var planted []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("run-%s-%d.log", base.Add(time.Duration(i)*time.Minute).Format("20060102T150405Z"), 1000+i)
		err = os.WriteFile(filepath.Join(dir, name), []byte("seeded by the test harness\n"), 0o600)
		if err != nil {
			return ctx, fmt.Errorf("planting run log %s: %w", name, err)
		}
		planted = append(planted, name)
	}
	sort.Strings(planted)
	return context.WithValue(ctx, seededLogsKey{}, planted), nil
}

// aFileThatIsNotARunLogInTheLogDirectory plants a file csync did not write beside
// the run logs. The name deliberately sits close to the real thing — it begins
// "run-", ends ".log", and lacks only the process-id field — so a prune matching on
// either affix rather than the whole name treats it as a candidate.
//
// Its timestamp predates every planted log, which is what gives the scenario teeth:
// a loosely-matching prune sorts this file oldest and deletes it first, where a name
// sorting among the newest would survive such a prune by luck and leave the scenario
// passing for the wrong reason.
func aFileThatIsNotARunLogInTheLogDirectory(ctx context.Context) (context.Context, error) {
	root := stateHome(ctx)
	if root == "" {
		return ctx, fmt.Errorf("no scenario state home; the Before hook did not run?")
	}
	dir := filepath.Join(root, "cherry-sync")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return ctx, fmt.Errorf("creating the log directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, "run-19990101T000000Z.log")
	err = os.WriteFile(path, []byte("not a run log\n"), 0o600)
	if err != nil {
		return ctx, fmt.Errorf("planting %s: %w", path, err)
	}
	return context.WithValue(ctx, plantedFileKey{}, path), nil
}

// theLogDirectoryShouldHoldNRunLogs asserts the directory holds exactly n run logs
// once the run is over — the ceiling, counted rather than reasoned about.
func theLogDirectoryShouldHoldNRunLogs(ctx context.Context, n int) error {
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	if len(names) != n {
		return fmt.Errorf("log directory holds %d run log(s), want %d: %v", len(names), n, names)
	}
	return nil
}

// theSurvivingRunLogsShouldBeTheNewestOnes asserts pruning kept the right logs, not
// merely the right number of them. Every planted log that survived must be newer
// than every planted log that did not, which is the property "keep the newest"
// means and which a prune in filesystem order would break while still counting out.
func theSurvivingRunLogsShouldBeTheNewestOnes(ctx context.Context) error {
	planted, _ := ctx.Value(seededLogsKey{}).([]string)
	if len(planted) == 0 {
		return fmt.Errorf("no run logs were planted, so there is no ordering to check")
	}
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	survived := make(map[string]bool, len(names))
	for _, n := range names {
		survived[n] = true
	}
	var kept, gone []string
	for _, n := range planted {
		if survived[n] {
			kept = append(kept, n)
		} else {
			gone = append(gone, n)
		}
	}
	if len(gone) == 0 {
		return fmt.Errorf("no planted run log was pruned, so nothing distinguishes newest from oldest; %d planted, %d present", len(planted), len(names))
	}
	if len(kept) == 0 {
		return fmt.Errorf("every planted run log was pruned; %d planted", len(planted))
	}
	// planted is sorted oldest first, so the survivors must be exactly its tail.
	if kept[0] < gone[len(gone)-1] {
		return fmt.Errorf("pruning kept %q but discarded the newer %q; kept %v, discarded %v", kept[0], gone[len(gone)-1], kept, gone)
	}
	return nil
}

// thatFileShouldStillBeThere asserts the planted not-a-run-log survived the prune.
func thatFileShouldStillBeThere(ctx context.Context) error {
	path, _ := ctx.Value(plantedFileKey{}).(string)
	if path == "" {
		return fmt.Errorf("no file was planted, so there is nothing to look for")
	}
	_, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("the planted file did not survive pruning: %w", err)
	}
	return nil
}

// thisRunsLog returns the log the run under test wrote: the one run log present that
// the scenario did not plant. Identifying it by difference rather than by age keeps
// the assertion off the ordering the pruning scenarios exist to test.
func thisRunsLog(ctx context.Context) (ParsedLog, string, error) {
	planted, _ := ctx.Value(seededLogsKey{}).([]string)
	seeded := make(map[string]bool, len(planted))
	for _, n := range planted {
		seeded[n] = true
	}
	names, err := scenarioRunLogs(ctx)
	if err != nil {
		return ParsedLog{}, "", err
	}
	var fresh []string
	for _, n := range names {
		if !seeded[n] {
			fresh = append(fresh, n)
		}
	}
	if len(fresh) != 1 {
		return ParsedLog{}, "", fmt.Errorf("want exactly one run log this run wrote, found %d: %v", len(fresh), fresh)
	}
	return parseLogAt(filepath.Join(stateHome(ctx), "cherry-sync", fresh[0]))
}

// theLogShouldNameTheRunLogsItPruned asserts the run accounted for the logs it
// deleted by name, and that the names it claims are really gone. Reconciling the
// two is what makes the record trustworthy: a run that listed a file it left behind
// would read as an explanation for an absence that never happened.
func theLogShouldNameTheRunLogsItPruned(ctx context.Context) error {
	log, content, err := thisRunsLog(ctx)
	if err != nil {
		return err
	}
	if len(log.Pruned) == 0 {
		return fmt.Errorf("run log names no pruned logs; contents:\n%s", content)
	}
	present, err := scenarioRunLogs(ctx)
	if err != nil {
		return err
	}
	still := make(map[string]bool, len(present))
	for _, n := range present {
		still[n] = true
	}
	for _, n := range log.Pruned {
		if still[n] {
			return fmt.Errorf("run log claims to have pruned %q, but it is still there; contents:\n%s", n, content)
		}
	}
	return nil
}

// theLogShouldRecordThatNothingWasPruned asserts the record is present and says so,
// rather than being absent. An absent line cannot distinguish a run that pruned
// nothing from one whose pruning never ran.
func theLogShouldRecordThatNothingWasPruned(ctx context.Context) error {
	log, content, err := thisRunsLog(ctx)
	if err != nil {
		return err
	}
	if !log.HasPruned {
		return fmt.Errorf("run log records no pruned line at all; contents:\n%s", content)
	}
	if len(log.Pruned) != 0 {
		return fmt.Errorf("run log names %d pruned log(s), want none; contents:\n%s", len(log.Pruned), content)
	}
	return nil
}
