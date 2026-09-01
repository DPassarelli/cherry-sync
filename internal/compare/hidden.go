// hidden.go parses the paths rsync names as held back by an --exclude, so csync
// can disclose a withheld .git it has no other way to see: the one on the far end
// of a pull, which no check of the local side could find.

package compare

import "strings"

// hiddenMarkers lists the phrases rsync uses to introduce a path it held back,
// across both dialects csync runs against — GNU rsync's "hiding"/"protecting" and
// openrsync's "hiding"/"skip excluded". Each is followed immediately by the path.
// GNU prefixes the line with [sender] or [generator] and openrsync with
// rsync(PID):, so a marker is searched for anywhere in the line rather than
// anchored to its start. GNU names a directory as such while openrsync calls
// everything a file, which is why the noun is matched rather than trusted.
var hiddenMarkers = []string{
	"hiding file ",
	"hiding directory ",
	"protecting file ",
	"protecting directory ",
	"skip excluded file ",
	"skip excluded directory ",
}

// patternClause is the trailing clause that follows the path on a "hiding" or
// "protecting" line. GNU rsync names the matching pattern after it; openrsync
// emits the same words and stops. Cutting at the LAST occurrence rather than the
// first is what keeps a path that itself contains the phrase intact under both.
const patternClause = " because of pattern"

// hiddenPath returns the path one line of rsync's -vv output reports as held back
// by an --exclude, and whether the line is such a report at all. The path is
// relative to the transfer root, so a nested one (a submodule's .git) keeps its
// directory prefix.
func hiddenPath(line string) (string, bool) {
	for _, marker := range hiddenMarkers {
		start := strings.Index(line, marker)
		if start < 0 {
			continue
		}
		path := line[start+len(marker):]
		clause := strings.LastIndex(path, patternClause)
		if clause >= 0 {
			path = path[:clause]
		}
		if path == "" {
			return "", false
		}
		return path, true
	}
	return "", false
}

// gitDirHidden reports whether rsync's -vv output names a .git it held back, at
// the transfer root or nested. This is the evidence behind the disclosure: the
// .git exclude is passed on every run, so only rsync can say whether it actually
// matched anything, and which side it matched on. Asking rsync rather than
// checking the local side is what lets a pull from a remote repository into a
// plain directory report the .git it withheld.
func gitDirHidden(rsyncOut string) bool {
	for line := range strings.SplitSeq(rsyncOut, "\n") {
		path, ok := hiddenPath(line)
		if !ok {
			continue
		}
		if path == ".git" || strings.HasSuffix(path, "/.git") {
			return true
		}
	}
	return false
}
