// Package filediff shows what differs inside one file between a source and a
// destination. rsync fetches both copies, so a remote side needs nothing beyond what
// a sync already requires.
package filediff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
	"github.com/dpassarelli/cherry-sync/internal/operand"
	"github.com/pmezard/go-difflib/difflib"
)

// SizeLimit is the largest copy a diff will read, in bytes. It keeps a slip of the
// "d" key from pulling a huge file over SSH just to scroll past it.
const SizeLimit = 200 * 1024

// Copy names one of a file's two copies.
type Copy int

// The copies a finding can name, or none.
const (
	NoCopy Copy = iota
	SourceCopy
	DestinationCopy
)

// Result is what comparing a file's two copies found: the unified diff's hunks, or
// the copy that stopped one from being made. Oversized copies are never read, so a
// copy over the limit is never also called binary.
type Result struct {
	Text      string
	Oversized Copy
	Binary    Copy
}

// fetched is one copy of a file: its content, or that it was over the limit.
type fetched struct {
	content   []byte
	oversized bool
}

// side is where one copy of a file lives, which copy it is, and the scratch
// directory it is fetched into.
type side struct {
	root    string
	copy    Copy
	scratch string
}

// Between compares relPath's two copies, producing a unified diff from the
// destination to the source, which is what syncing it would do to the destination.
// The first copy that can't be diffed settles the result, so the second is never
// fetched.
func Between(ctx context.Context, r *command.Runner, source, destination, relPath string) (Result, error) {
	scratch, err := os.MkdirTemp("", "csync-diff-*")
	if err != nil {
		return Result{}, fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	contents := map[Copy][]byte{}
	for _, s := range fetchOrder(source, destination) {
		got, err := fetch(ctx, r, s.root, filepath.Join(scratch, s.scratch), relPath)
		if err != nil {
			return Result{}, err
		}
		if got.oversized {
			return Result{Oversized: s.copy}, nil
		}
		if isBinary(got.content) {
			return Result{Binary: s.copy}, nil
		}
		contents[s.copy] = got.content
	}
	text, err := unified(contents[DestinationCopy], contents[SourceCopy])
	return Result{Text: text}, err
}

// fetchOrder lists the two copies with a local one first, so a local copy that
// can't be diffed settles the question without a round trip to the remote.
func fetchOrder(source, destination string) []side {
	src := side{root: source, copy: SourceCopy, scratch: "source"}
	dst := side{root: destination, copy: DestinationCopy, scratch: "destination"}
	if operand.IsRemote(source) && !operand.IsRemote(destination) {
		return []side{dst, src}
	}
	return []side{src, dst}
}

// unified renders the diff from the destination copy to the source copy as hunks
// alone. The copies are left unnamed because whoever shows the diff knows each
// side's full path, which a name here could only repeat.
func unified(destinationCopy, sourceCopy []byte) (string, error) {
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:       splitLines(destinationCopy),
		B:       splitLines(sourceCopy),
		Context: 3,
	})
}

// splitLines splits content into the lines a diff compares. difflib's own splitter
// adds a line that isn't in the file, which throws off the hunk header. A last line
// with no newline carries the standard marker, so it compares as different from the
// same text with one and still prints on a line of its own.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(content), "\n")
	last := len(lines) - 1
	if lines[last] == "" {
		return lines[:last]
	}
	lines[last] += "\n\\ No newline at end of file\n"
	return lines
}

// isBinary reports whether content holds a NUL byte, the same test git uses, since
// text formats don't contain one and a diff of anything else would be unreadable.
func isBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0
}

// fetch copies relPath from root into dir, unless it is over SizeLimit. rsync skips
// such a file without an error, so its absence afterwards is the answer: a file truly
// missing from root fails the rsync instead. The read goes through an os.Root because
// the path came from the far side's rsync output, so it must not be able to reach
// outside the scratch directory.
func fetch(ctx context.Context, r *command.Runner, root, dir, relPath string) (fetched, error) {
	_, err := r.Run(ctx, "rsync", fetchArgs(root, dir), strings.NewReader(relPath+"\x00"))
	if err != nil {
		return fetched{}, err
	}
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return fetched{}, fmt.Errorf("create scratch directory: %w", err)
	}
	scratch, err := os.OpenRoot(dir)
	if err != nil {
		return fetched{}, fmt.Errorf("open fetched copy: %w", err)
	}
	defer scratch.Close()
	content, err := scratch.ReadFile(relPath)
	if errors.Is(err, fs.ErrNotExist) {
		return fetched{oversized: true}, nil
	}
	return fetched{content: content}, err
}

// fetchArgs builds the rsync arguments for fetching one listed path, which arrives
// NUL-terminated on stdin so a newline in its name can't smuggle in another entry.
// The `--` keeps a root beginning with `-` from being read as an option.
func fetchArgs(root, dir string) []string {
	return []string{"--files-from=-", "--from0", "--max-size=" + strconv.Itoa(SizeLimit), "--", root + "/", dir + "/"}
}
