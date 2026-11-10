// banner.go renders csync's title banner (shown at the top of an interactive
// run) and the --version report. Both lead with the same "cherry-sync <version>"
// line so they can't drift; the report adds a one-line description and the
// project URL, which the banner omits to keep a normal run uncluttered.

package view

const (
	// projectName leads the banner and the --version report. It is the project's
	// name, deliberately not the "csync" binary name.
	projectName = "cherry-sync"
	// versionDescription is the one-line summary --version prints. --help reuses
	// the same VersionReport header, so both flags state what csync does the same
	// way.
	versionDescription = "An interactive rsync wrapper for selectively moving files over SSH."
	// versionURL is the project home --version points users to.
	versionURL = "https://github.com/dpassarelli/cherry-sync"
	// versionLicense is the closing --version line pointing at the --license flag,
	// which prints the full terms the binary carries embedded.
	versionLicense = "MIT licensed. Run 'csync --license' to view the terms."
)

// Build identifies the binary that is running. Version is main.version, the
// release version injected at build time or "dev" when none was. Commit is the
// abbreviated revision Go recorded from the source checkout, empty when it
// recorded none; only a dev build shows it, since a release is already named by
// its version.
type Build struct {
	Version string
	Commit  string
}

// Banner returns csync's title banner — the project name and version on one line
// (e.g. "cherry-sync v1.2.3"), then a blank line — ready to print above the
// header. main shows it only on an interactive run, where decoration belongs;
// piped output skips it.
func Banner(b Build) string {
	return versionLine(b) + "\n\n"
}

// VersionReport returns the full text the --version flag prints: the version
// line, a one-line description of what csync does, the project URL, and a
// pointer to --license. The interactive banner shows only the version line, so a
// normal run stays terse.
func VersionReport(b Build) string {
	return versionLine(b) + "\n" + versionDescription + "\n" + versionURL + "\n" + versionLicense
}

// versionLine renders the project name and version, e.g. "cherry-sync v1.2.3" or
// "cherry-sync (dev build: e608fd7)". It is the one line the banner and the
// --version report share.
func versionLine(b Build) string {
	return projectName + " " + versionSuffix(b)
}

// versionSuffix renders a build as the suffix that follows the project name: a
// real injected version (e.g. "1.2.3") becomes "v1.2.3", while the un-injected
// "dev" default becomes "(dev build)" rather than a nonsensical "vdev", naming
// the commit when one was recorded.
func versionSuffix(b Build) string {
	if b.Version != "dev" {
		return "v" + b.Version
	}
	if b.Commit == "" {
		return "(dev build)"
	}
	return "(dev build: " + b.Commit + ")"
}
