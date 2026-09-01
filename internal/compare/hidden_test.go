package compare

import "testing"

// The tests below pin gitDirHidden against the exact lines the two rsync
// implementations emit under -vv, captured by experiment: GNU rsync 3.4.1 on
// Linux and openrsync (protocol 29) on macOS. Each end of a transfer reports in
// its own words, and only one of the four shapes appears on any given run, so a
// parser that handles just the dialect the developer happens to run would leave
// the other platform silently withholding a .git. The scenarios exercise GNU only
// (the suite's rsync), which is why openrsync's wording is pinned here.

// assertGitDirHidden checks gitDirHidden's verdict on one line of rsync output.
func assertGitDirHidden(t *testing.T, want bool, line string) {
	t.Helper()
	got := gitDirHidden(line)
	if got != want {
		t.Errorf("gitDirHidden(%q) = %v, want %v", line, got, want)
	}
}

// GNU rsync, sending side: the case a push from a repository produces, and the
// one a pull relays from the remote sender.
func TestGitDirHidden_GNUSender(t *testing.T) {
	assertGitDirHidden(t, true, "[sender] hiding directory .git because of pattern .git")
}

// GNU rsync, receiving side: the exclude also protects an existing .git on the
// destination from --delete, and the generator says so in different words.
func TestGitDirHidden_GNUGenerator(t *testing.T) {
	assertGitDirHidden(t, true, "[generator] protecting directory .git because of pattern .git")
}

// openrsync, sending side. It calls every excluded path a "file" regardless of
// type and names no pattern after the clause, so neither the noun nor a trailing
// pattern can be relied on.
func TestGitDirHidden_OpenrsyncSender(t *testing.T) {
	assertGitDirHidden(t, true, "rsync(88839): : hiding file .git because of pattern")
}

// openrsync, receiving side. Its wording shares no phrase with the other three
// beyond the path itself.
func TestGitDirHidden_OpenrsyncReceiver(t *testing.T) {
	assertGitDirHidden(t, true, "rsync(88841): : skip excluded file .git")
}

// A submodule's .git sits below the transfer root and is reported with its
// directory prefix; it is git metadata just the same, and the floating exclude
// that catches it must be disclosed like any other.
func TestGitDirHidden_NestedGitDir(t *testing.T) {
	assertGitDirHidden(t, true, "[sender] hiding file vendor/lib/.git because of pattern .git")
}

// Teeth: the gitignored excludes are passed on the same run and produce the same
// shape of line. Treating any withheld path as the .git directory would report it
// on every run against a repository with a .gitignore.
func TestGitDirHidden_GitignoredPathIsNotTheGitDir(t *testing.T) {
	assertGitDirHidden(t, false, "[sender] hiding directory build because of pattern /build")
}

// Teeth: .gitignore starts with the same eight bytes as the directory csync
// withholds. A prefix or substring test would call this a .git and disclose an
// exclusion that never happened.
func TestGitDirHidden_GitignoreFileIsNotTheGitDir(t *testing.T) {
	assertGitDirHidden(t, false, "[sender] hiding file .gitignore because of pattern .gitignore")
}

// Teeth: a run that withheld nothing must say nothing. rsync's ordinary itemize
// output carries no report line, and reading one out of it would put the
// disclosure on every sync.
func TestGitDirHidden_ItemizeOutputAlone(t *testing.T) {
	assertGitDirHidden(t, false, ">f+++++++++ .gitignore\ncd+++++++++ src/\n>f+++++++++ src/main.go")
}
