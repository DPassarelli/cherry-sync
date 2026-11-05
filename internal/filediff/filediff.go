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
	"github.com/pmezard/go-difflib/difflib"
)

// SizeLimit is the largest copy a diff will read, in bytes. It keeps a slip of the
// "d" key from pulling a huge file over SSH just to scroll past it.
const SizeLimit = 200 * 1024

// Copies names which copies of a file a finding applies to.
type Copies int

// The possible answers to which copies a finding applies to.
const (
	NoCopies Copies = iota
	SourceCopy
	DestinationCopy
	BothCopies
)

// Result is what comparing a file's two copies found: the unified diff, or which
// copies stopped one from being made. Oversized copies are never read, so Binary
// can only speak for the copies under the limit.
type Result struct {
	Text      string
	Oversized Copies
	Binary    Copies
}

// fetched is one copy of a file: its content, or that it was over the limit.
type fetched struct {
	content   []byte
	oversized bool
}

// Between compares relPath's two copies, producing a unified diff from the
// destination to the source, which is what syncing it would do to the destination.
func Between(ctx context.Context, r *command.Runner, source, destination, relPath string) (Result, error) {
	scratch, err := os.MkdirTemp("", "csync-diff-*")
	if err != nil {
		return Result{}, fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	sourceCopy, err := fetch(ctx, r, source, filepath.Join(scratch, "source"), relPath)
	if err != nil {
		return Result{}, err
	}
	destinationCopy, err := fetch(ctx, r, destination, filepath.Join(scratch, "destination"), relPath)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Oversized: whichCopies(sourceCopy.oversized, destinationCopy.oversized),
		Binary:    whichCopies(isBinary(sourceCopy.content), isBinary(destinationCopy.content)),
	}
	if result.Oversized != NoCopies || result.Binary != NoCopies {
		return result, nil
	}
	result.Text, err = unified(relPath, destinationCopy.content, sourceCopy.content)
	return result, err
}

// unified renders the diff from the destination copy to the source copy.
func unified(relPath string, destinationCopy, sourceCopy []byte) (string, error) {
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        splitLines(destinationCopy),
		B:        splitLines(sourceCopy),
		FromFile: "destination: " + relPath,
		ToFile:   "source: " + relPath,
		Context:  3,
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

// whichCopies combines a check's answer for each copy.
func whichCopies(source, destination bool) Copies {
	switch {
	case source && destination:
		return BothCopies
	case source:
		return SourceCopy
	case destination:
		return DestinationCopy
	default:
		return NoCopies
	}
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
