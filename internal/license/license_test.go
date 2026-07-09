package license

import (
	"os"
	"strings"
	"testing"
)

// TestText_ReturnsLicenseNotice pins that Text returns the MIT notices --license
// must print: the title, the copyright line, and the warranty paragraph (the
// "permission notice" MIT requires travel with every copy).
func TestText_ReturnsLicenseNotice(t *testing.T) {
	got := Text()
	for _, want := range []string{"MIT License", "Copyright (c)", "THE SOFTWARE IS PROVIDED"} {
		if !strings.Contains(got, want) {
			t.Errorf("Text() is missing %q; got:\n%s", want, got)
		}
	}
}

// TestText_MatchesRootLicense guards against drift between the canonical
// repository-root LICENSE and the copy this package embeds. go:embed cannot
// reach a parent directory, so the copy is embedded instead; this test fails if
// editing one (e.g. bumping the copyright year in the root file) isn't mirrored
// in the other.
func TestText_MatchesRootLicense(t *testing.T) {
	root, err := os.ReadFile("../../LICENSE")
	if err != nil {
		t.Fatalf("reading root LICENSE: %v", err)
	}
	if Text() != string(root) {
		t.Error("embedded license differs from the root LICENSE; copy the root file to internal/license/LICENSE")
	}
}
