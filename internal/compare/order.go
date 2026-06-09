// order.go computes the stable display order of reported actions — the sequence
// the user sees and selects against — so the list is predictable rather than
// rsync's directory-grouped emit order. The end-to-end contract lives in
// features/order-reported-actions.feature; the per-rule unit tests in
// order_test.go.

package compare

import (
	"slices"
	"strings"
)

// sortActions orders actions the way a file tree presents them, so the
// displayed list — and the numbering the user selects against — is stable and
// predictable rather than rsync's directory-grouped emit order. See the
// contract documented in features/order-reported-actions.feature.
func sortActions(actions []Action) {
	slices.SortFunc(actions, func(a, b Action) int {
		return comparePaths(a.Path, b.Path)
	})
}

// comparePaths compares two relative paths segment by segment, applying these
// keys in order at each level: (1) dot entries before non-dot, (2) files before
// subdirectories, (3) the segment ordering in compareSegment.
func comparePaths(a, b string) int {
	as := strings.Split(a, "/")
	bs := strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		aSeg, bSeg := as[i], bs[i]
		aIsFile := i == len(as)-1
		bIsFile := i == len(bs)-1

		ad, bd := isDotSegment(aSeg), isDotSegment(bSeg)
		if ad != bd {
			return firstIf(ad)
		}
		if aIsFile != bIsFile {
			return firstIf(aIsFile)
		}
		if aSeg != bSeg {
			return compareSegment(aSeg, bSeg)
		}
		// identical segment: descend into the shared subdirectory
	}
	// One path is a prefix of the other (a file sharing a directory's name);
	// the shallower one sorts first.
	return len(as) - len(bs)
}

// compareSegment orders two distinct same-kind, same-dotness segments:
// number-leading names before letter-leading, numbers by value, everything
// else alphabetically (case-insensitive) with byte order breaking ties.
func compareSegment(a, b string) int {
	aNum, bNum := startsWithDigit(a), startsWithDigit(b)
	if aNum != bNum {
		return firstIf(aNum)
	}
	if aNum {
		numCmp := compareNumericRun(a, b)
		if numCmp != 0 {
			return numCmp
		}
		return strings.Compare(a, b)
	}
	nameCmp := strings.Compare(strings.ToLower(a), strings.ToLower(b))
	if nameCmp != 0 {
		return nameCmp
	}
	return strings.Compare(a, b)
}

// compareNumericRun compares the leading digit runs of a and b by numeric
// value, tolerant of leading zeros and arbitrarily long numbers (it compares
// zero-trimmed runs by length then lexically, so it never overflows).
func compareNumericRun(a, b string) int {
	na := strings.TrimLeft(leadingDigits(a), "0")
	nb := strings.TrimLeft(leadingDigits(b), "0")
	if len(na) != len(nb) {
		return len(na) - len(nb)
	}
	return strings.Compare(na, nb)
}

// leadingDigits returns the longest prefix of s made up of ASCII digits.
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// startsWithDigit reports whether s begins with an ASCII digit.
func startsWithDigit(s string) bool {
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// isDotSegment reports whether a path segment begins with a dot.
func isDotSegment(s string) bool {
	return strings.HasPrefix(s, ".")
}

// firstIf maps "should this one sort first?" to the int a comparator returns.
func firstIf(first bool) int {
	if first {
		return -1
	}
	return 1
}
