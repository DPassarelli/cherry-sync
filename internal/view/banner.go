// banner.go renders csync's title banner (shown at the top of an interactive
// run) and the --version report. Both lead with the same "cherry-sync <version>"
// line so they can't drift; the report adds a one-line description and the
// project URL, which the banner omits to keep a normal run uncluttered.

package view

const (
	// projectName leads the banner and the --version report. It is the project's
	// name, deliberately not the "csync" binary name.
	projectName = "cherry-sync"
	// versionDescription is the one-line summary --version prints. csync has no
	// --help yet, so this is the only in-tool statement of what it does.
	versionDescription = "An interactive rsync wrapper for selectively moving files over SSH."
	// versionURL is the project home --version points users to.
	versionURL = "https://github.com/dpassarelli/cherry-sync"
)

// Banner returns csync's title banner — the project name and version on one line
// (e.g. "cherry-sync v1.2.3"), then a blank line — ready to print above the
// header. main shows it only on an interactive run, where decoration belongs;
// piped output skips it. raw is the build version (main.version).
func Banner(raw string) string {
	return versionLine(raw) + "\n\n"
}

// VersionReport returns the full text the --version flag prints: the version
// line, a one-line description of what csync does, and the project URL. The
// interactive banner shows only the version line, so a normal run stays terse.
func VersionReport(raw string) string {
	return versionLine(raw) + "\n" + versionDescription + "\n" + versionURL
}

// versionLine renders the project name and version, e.g. "cherry-sync v1.2.3" or
// "cherry-sync (dev build)". It is the one line the banner and the --version
// report share.
func versionLine(raw string) string {
	return projectName + " " + versionSuffix(raw)
}

// versionSuffix renders a raw build version as the suffix that follows the
// project name: a real injected version (e.g. "1.2.3") becomes "v1.2.3", while
// the un-injected "dev" default becomes "(dev build)" rather than a nonsensical
// "vdev".
func versionSuffix(raw string) string {
	if raw == "dev" {
		return "(dev build)"
	}
	return "v" + raw
}
