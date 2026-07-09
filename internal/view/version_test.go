package view

import (
	"strings"
	"testing"
)

// TestVersionLine covers how a raw build version renders as the shared
// "cherry-sync <version>" line: a real injected version gets a "v" prefix, while
// the un-injected "dev" default renders as "(dev build)" rather than a
// nonsensical "vdev".
func TestVersionLine(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"injected semver", "1.2.3", "cherry-sync v1.2.3"},
		{"test injection", "0.0.0-test", "cherry-sync v0.0.0-test"},
		{"dev default", "dev", "cherry-sync (dev build)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := versionLine(c.raw)

			if got != c.want {
				t.Errorf("versionLine(%q): got %q, want %q", c.raw, got, c.want)
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
		name string
		raw  string
		want string
	}{
		{"injected semver", "1.2.3", "cherry-sync v1.2.3\n\n"},
		{"dev default", "dev", "cherry-sync (dev build)\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Banner(c.raw)

			if got != c.want {
				t.Errorf("Banner(%q): got %q, want %q", c.raw, got, c.want)
			}
			if strings.Contains(got, versionDescription) || strings.Contains(got, versionURL) {
				t.Errorf("Banner(%q) should not carry the description or URL: got %q", c.raw, got)
			}
		})
	}
}

// TestVersionReport covers the full --version output: the version line, the
// one-line description, the project URL, and the license pointer, each on its
// own line and in that order.
func TestVersionReport(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"injected semver", "1.2.3", "cherry-sync v1.2.3\n" + versionDescription + "\n" + versionURL + "\n" + versionLicense},
		{"dev default", "dev", "cherry-sync (dev build)\n" + versionDescription + "\n" + versionURL + "\n" + versionLicense},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := VersionReport(c.raw)

			if got != c.want {
				t.Errorf("VersionReport(%q):\ngot  %q\nwant %q", c.raw, got, c.want)
			}
		})
	}
}
