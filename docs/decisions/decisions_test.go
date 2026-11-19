package decisions_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// templateName is the one Markdown file in this folder that is not a record.
const templateName = "TEMPLATE.md"

// dateLayout is the date format that prefixes every record's file name.
const dateLayout = "2006-01-02"

// recordNameRE matches a record's file name, capturing its date prefix.
var recordNameRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-[a-z0-9]+(?:-[a-z0-9]+)*\.md$`)

// supersededRE matches a Status line naming the record that replaced this one,
// capturing that record's file name.
var supersededRE = regexp.MustCompile(`^Status: Superseded by \[[^\]]+\]\(([^)/]+\.md)\)$`)

// earliest and latest bound a record's date, both inclusive: nothing was decided
// before the repository existed, and the far bound rejects mistyped years.
var (
	earliest = time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	latest   = time.Date(2036, 12, 31, 0, 0, 0, 0, time.UTC)
)

// TestDecisionRecords_FollowTheConvention keeps every record findable by its date
// and shaped like the template, so a reader can rely on the structure and the
// supersede chain without checking each file by hand.
func TestDecisionRecords_FollowTheConvention(t *testing.T) {
	want := sections(t, templateName)
	if len(want) == 0 {
		t.Fatalf("%s has no ## sections to require of records", templateName)
	}

	names, err := filepath.Glob("*.md")
	if err != nil {
		t.Fatalf("listing records: %v", err)
	}
	records := map[string]time.Time{}
	for _, name := range names {
		if name == templateName {
			continue
		}
		date, ok := recordDate(name)
		if !ok {
			t.Errorf("%s: name must be YYYY-MM-DD-kebab-title.md with a real date", name)
			continue
		}
		if date.Before(earliest) || date.After(latest) {
			t.Errorf("%s: date must fall between %s and %s inclusive", name, earliest.Format(dateLayout), latest.Format(dateLayout))
		}
		records[name] = date
	}

	for name, date := range records {
		got := sections(t, name)
		if !inOrder(want, got) {
			t.Errorf("%s: sections %q must include %q in that order", name, got, want)
		}
		checkStatus(t, name, date, records)
	}
}

// recordDate parses the date prefix of a record's file name, reporting false
// when the name does not follow the convention or the date does not exist.
func recordDate(name string) (time.Time, bool) {
	m := recordNameRE.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	date, err := time.Parse(dateLayout, m[1])
	if err != nil {
		return time.Time{}, false
	}
	return date, true
}

// checkStatus requires exactly one Status line, either Accepted or superseded by
// an existing record dated no earlier than this one.
func checkStatus(t *testing.T, name string, date time.Time, records map[string]time.Time) {
	t.Helper()
	var statuses []string
	for _, line := range lines(t, name) {
		if strings.HasPrefix(line, "Status:") {
			statuses = append(statuses, line)
		}
	}
	if len(statuses) != 1 {
		t.Errorf("%s: want exactly one Status line, found %d", name, len(statuses))
		return
	}
	status := statuses[0]
	if status == "Status: Accepted" {
		return
	}
	m := supersededRE.FindStringSubmatch(status)
	if m == nil {
		t.Errorf("%s: Status must be \"Accepted\" or \"Superseded by [title](record.md)\", got %q", name, status)
		return
	}
	successor, ok := records[m[1]]
	if !ok {
		t.Errorf("%s: superseded by %s, which is not a record here", name, m[1])
		return
	}
	if successor.Before(date) {
		t.Errorf("%s: superseded by %s, which is dated earlier", name, m[1])
	}
}

// sections returns the file's level-two headings in document order.
func sections(t *testing.T, name string) []string {
	t.Helper()
	var found []string
	for _, line := range lines(t, name) {
		heading, ok := strings.CutPrefix(line, "## ")
		if ok {
			found = append(found, strings.TrimSpace(heading))
		}
	}
	return found
}

// lines reads a file in this folder line by line.
func lines(t *testing.T, name string) []string {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	defer f.Close()
	var all []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	err = scanner.Err()
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return all
}

// inOrder reports whether every element of want appears in got, in the same
// relative order; got may carry extra sections between them.
func inOrder(want, got []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}
