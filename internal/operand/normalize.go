// Package operand normalizes the source and destination strings csync hands to
// rsync — whether they came from the command line or a saved .csync.toml — so
// their meaning is a property of csync rather than an accident of what rsync
// happens to tolerate. It resolves a home shortcut ("~") on either side — rsync
// has none of its own and would take the character literally — and collapses
// trailing slashes.
package operand

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result is the outcome of normalizing one rsync operand.
type Result struct {
	// Path is the normalized operand to hand to rsync and to display.
	Path string
	// Rewrote reports whether a leading "~" home shortcut was resolved — the
	// surprising, semantic change worth disclosing. A trailing-slash collapse
	// alone does not set it: that cleanup is already visible in the header echo.
	Rewrote bool
	// From is the original path portion when Rewrote is true (e.g. "~/working"),
	// so the caller can note "(rewritten from …)" beside the operand; "" when
	// nothing was rewritten.
	From string
}

// Normalize resolves a leading remote home shortcut and collapses trailing
// slashes on op. A remote path beginning with "~/" (or a bare "~") is rewritten
// to the equivalent relative path — rsync interprets a relative remote path
// against the login home, whereas modern rsync's protected-args default passes a
// literal "~" through and the transfer fails (#50). A "~user" form has no
// relative equivalent, so Normalize rejects it with an error rather than let
// rsync fail confusingly later. A local operand's "~" is expanded against this
// user's own home instead, since a local path is resolved here rather than by a
// login shell on the far side.
func Normalize(op string) (Result, error) {
	var res Result

	prefix, path := splitRemote(op)
	// A remote operand carries a non-empty "[user@]host:" prefix; a local one has
	// none. Only a remote path gets tilde resolution — a local "~" is a separate
	// concern (it would expand against our own home) and is left untouched.
	if prefix != "" {
		switch {
		case path == "~" || path == "~/":
			// The home directory itself → the directory the remote already sits in.
			res.From = path
			res.Rewrote = true
			path = "."
		case strings.HasPrefix(path, "~/"):
			res.From = path
			res.Rewrote = true
			path = path[len("~/"):]
		case strings.HasPrefix(path, "~"):
			// "~user" names another user's home, which no relative path can reach.
			return Result{}, fmt.Errorf("remote path %q uses a ~user home shortcut, which has no relative form; use an absolute path", op)
		}
	} else if strings.HasPrefix(path, "~") {
		expanded, err := expandHome(path)
		if err != nil {
			return Result{}, err
		}
		res.From = path
		res.Rewrote = true
		path = expanded
	}

	path = collapseTrailingSlash(path)
	// Never emit an empty path: "" plus rsync's appended slash would become the
	// filesystem root. A path that reduced to nothing (a bare "~", "host:", or a
	// run of slashes) means the directory itself — "." — not root.
	if path == "" {
		path = "."
	}

	res.Path = prefix + path
	return res, nil
}

// expandHome resolves a local path's leading "~" against this user's home
// directory. rsync has no home shortcut of its own, so an unexpanded one is taken
// as an ordinary directory name: "~/x" in the source position misses and fails,
// and a "~" destination quietly writes into a directory literally named "~" (#71).
// A "~user" form names another account's home, which csync does not resolve; it is
// rejected here as it is on the remote side.
func expandHome(path string) (string, error) {
	rest := path[len("~"):]
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", fmt.Errorf("local path %q uses a ~user home shortcut, which csync does not resolve; use an absolute path", path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand the ~ in local path %q: %w", path, err)
	}
	// Join rather than concatenation: it absorbs a home that already ends in a
	// slash, and keeps a home of "/" from producing a doubled separator.
	return filepath.Join(home, rest), nil
}

// splitRemote separates op into an rsync remote prefix ("[user@]host:") and the
// path that follows; the prefix is empty for a local operand. It uses rsync's own
// rule: an operand is remote when a ":" appears before the first "/", so a local
// path like "./a:b" (slash first) is not mistaken for a remote.
func splitRemote(op string) (prefix, path string) {
	colon := strings.IndexByte(op, ':')
	slash := strings.IndexByte(op, '/')
	if colon >= 0 && (slash < 0 || colon < slash) {
		return op[:colon+1], op[colon+1:]
	}
	return "", op
}

// collapseTrailingSlash strips trailing slashes from p so csync appends exactly
// one itself, instead of relying on rsync collapsing a doubled slash. A path that
// is only slashes represents the filesystem root and is preserved as "/"; an
// empty p stays empty for the caller to map to ".".
func collapseTrailingSlash(p string) string {
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" && len(p) > 0 {
		return "/"
	}
	return trimmed
}
