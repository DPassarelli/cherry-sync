package view

import (
	"strings"
	"testing"
)

// TestVersionLine covers how a build renders as the shared
// "cherry-sync <version>" line: a real injected version gets a "v" prefix and
// ignores the commit, while the un-injected "dev" default renders as
// "(dev build)" rather than a nonsensical "vdev", naming the commit when the
// build recorded one so two dev binaries can be told apart.
func TestVersionLine(t *testing.T) {
	cases := []struct {
		name  string
		build Build
		want  string
	}{
		{"injected semver", Build{Version: "1.2.3"}, "cherry-sync v1.2.3"},
		{"test injection", Build{Version: "0.0.0-test"}, "cherry-sync v0.0.0-test"},
		{"release ignores commit", Build{Version: "1.2.3", Commit: "e608fd7"}, "cherry-sync v1.2.3"},
		{"dev with commit", Build{Version: "dev", Commit: "e608fd7"}, "cherry-sync (dev build: e608fd7)"},
		{"dev without commit", Build{Version: "dev"}, "cherry-sync (dev build)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := versionLine(c.build)

			if got != c.want {
				t.Errorf("versionLine(%+v): got %q, want %q", c.build, got, c.want)
			}
		})
	}
}

// TestBanner_ShowsProjectNameAndVersion pins that the interactive banner shows
// the project name and version on one line — so a terminal run shows which build
// is running at the top of the header — and carries neither the description nor
// the URL, which would clutter a normal run.
func TestBanner_ShowsProjectNameAndVersion(t *testing.T) {
	cases := []struct {
		name  string
		build Build
		want  string
	}{
		{"injected semver", Build{Version: "1.2.3"}, "cherry-sync v1.2.3\n\n"},
		{"dev default", Build{Version: "dev"}, "cherry-sync (dev build)\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Banner(c.build)

			if got != c.want {
				t.Errorf("Banner(%+v): got %q, want %q", c.build, got, c.want)
			}
			if strings.Contains(got, versionDescription) || strings.Contains(got, versionURL) {
				t.Errorf("Banner(%+v) should not carry the description or URL: got %q", c.build, got)
			}
		})
	}
}

// TestVersionReport covers the full --version output: the version line, the
// one-line description, the project URL, and the license pointer, each on its
// own line and in that order.
func TestVersionReport(t *testing.T) {
	cases := []struct {
		name  string
		build Build
		want  string
	}{
		{"injected semver", Build{Version: "1.2.3"}, "cherry-sync v1.2.3\n" + versionDescription + "\n" + versionURL + "\n" + versionLicense},
		{"dev default", Build{Version: "dev"}, "cherry-sync (dev build)\n" + versionDescription + "\n" + versionURL + "\n" + versionLicense},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := VersionReport(c.build)

			if got != c.want {
				t.Errorf("VersionReport(%+v):\ngot  %q\nwant %q", c.build, got, c.want)
			}
		})
	}
}
