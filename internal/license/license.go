// Package license exposes csync's MIT license text, embedded into the binary so
// `csync --license` can print it. The canonical file is the repository-root
// LICENSE; go:embed cannot reach a parent directory, so this package embeds a
// byte-identical copy that lives beside this file. license_test.go fails if the
// two ever drift, keeping the root file the single source of truth.
package license

import _ "embed"

// text is the embedded MIT license — the byte-for-byte copy of the repository
// root LICENSE that sits next to this file. See the package doc for why a copy
// exists rather than embedding the root file directly.
//
//go:embed LICENSE
var text string

// Text returns the full MIT license text: the copyright notice and the
// permission notice. MIT requires both accompany every copy of the software,
// which for a distributed bare binary means being able to print them on demand.
func Text() string {
	return text
}
