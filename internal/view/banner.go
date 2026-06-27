// banner.go renders csync's title banner, shown at the top of an interactive run.

package view

// Banner returns csync's title banner — the tool name followed by a blank line,
// ready to print above the header. It carries no version yet: version injection is
// deferred until a --version flag exists, so today the banner is just the name.
// main shows it only on an interactive run, where decoration belongs; piped output
// skips it.
func Banner() string {
	return "cherry-sync\n\n"
}
