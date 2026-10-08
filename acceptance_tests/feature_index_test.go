package acceptance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// featureLinkRE matches a Markdown link to a feature file in the same directory,
// capturing the file name.
var featureLinkRE = regexp.MustCompile(`\]\(([a-z0-9-]+\.feature)\)`)

// TestFeatureIndex_LinksEveryFeature keeps features/README.md, the map of user
// journeys, from drifting away from the specs it describes: a feature added
// without a place on the map, or a link left pointing at a renamed file, fails
// here rather than leaving readers with a stale picture.
func TestFeatureIndex_LinksEveryFeature(t *testing.T) {
	index, err := os.ReadFile(filepath.Join("features", "README.md"))
	if err != nil {
		t.Fatalf("reading the feature index: %v", err)
	}
	linked := map[string]bool{}
	for _, m := range featureLinkRE.FindAllStringSubmatch(string(index), -1) {
		linked[m[1]] = true
	}

	files, err := filepath.Glob(filepath.Join("features", "*.feature"))
	if err != nil {
		t.Fatalf("listing features: %v", err)
	}
	present := map[string]bool{}
	for _, f := range files {
		name := filepath.Base(f)
		present[name] = true
		if !linked[name] {
			t.Errorf("features/README.md does not link %s", name)
		}
	}
	for name := range linked {
		if !present[name] {
			t.Errorf("features/README.md links %s, which does not exist", name)
		}
	}
}
