// Package filediff shows what differs inside one file between a source and a
// destination. rsync fetches both copies, so a remote side needs nothing beyond what
// a sync already requires.
package filediff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/command"
	"github.com/pmezard/go-difflib/difflib"
)

// Between returns a unified diff of relPath from its destination copy to its source
// copy, which is what syncing it would do to the destination.
func Between(ctx context.Context, r *command.Runner, source, destination, relPath string) (string, error) {
	scratch, err := os.MkdirTemp("", "csync-diff-*")
	if err != nil {
		return "", fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	sourceCopy, err := fetch(ctx, r, source, filepath.Join(scratch, "source"), relPath)
	if err != nil {
		return "", err
	}
	destinationCopy, err := fetch(ctx, r, destination, filepath.Join(scratch, "destination"), relPath)
	if err != nil {
		return "", err
	}
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(destinationCopy)),
		B:        difflib.SplitLines(string(sourceCopy)),
		FromFile: "destination: " + relPath,
		ToFile:   "source: " + relPath,
		Context:  3,
	})
}

// fetch copies relPath from root into dir and returns its content. The read goes
// through an os.Root because the path came from the far side's rsync output, so it
// must not be able to reach outside the scratch directory.
func fetch(ctx context.Context, r *command.Runner, root, dir, relPath string) ([]byte, error) {
	_, err := r.Run(ctx, "rsync", fetchArgs(root, dir), strings.NewReader(relPath+"\x00"))
	if err != nil {
		return nil, err
	}
	fetched, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open fetched copy: %w", err)
	}
	defer fetched.Close()
	return fetched.ReadFile(relPath)
}

// fetchArgs builds the rsync arguments for fetching one listed path, which arrives
// NUL-terminated on stdin so a newline in its name can't smuggle in another entry.
// The `--` keeps a root beginning with `-` from being read as an option.
func fetchArgs(root, dir string) []string {
	return []string{"--files-from=-", "--from0", "--", root + "/", dir + "/"}
}
