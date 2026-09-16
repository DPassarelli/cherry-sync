// Given steps that build the local and remote directory trees a scenario
// compares, and the mutations (edit, add, delete, retouch) applied to them.

package acceptance_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// aLocalDirectoryWhosePathContainsASpace creates a local tempdir whose path holds a
// space and populates it like the plain local-directory step, re-stashing it under
// localPathKey (overriding the Background's). The space rides into the source operand
// csync hands rsync, so the run log must quote it to keep the operand one argument —
// which is what the space-fidelity scenario reads back out.
func aLocalDirectoryWhosePathContainsASpace(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, "src/main.go\nREADME.md")
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalDirectoryWhosePathContainsADoubleQuote creates a local tempdir whose path
// holds a double-quote character, populated like the plain local-directory step and
// re-stashed under localPathKey. The quote is the log's own delimiter: recorded naively
// it would forge a false argument boundary, so this is the counterpart to the space
// step — the space proves real boundaries are kept, the quote proves fake ones can't be
// minted.
func aLocalDirectoryWhosePathContainsADoubleQuote(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", `csync-q"-*`)
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, "src/main.go\nREADME.md")
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalSourcePathThatDoesNotExist stashes, as the local operand, a well-formed
// tempdir path that has been removed — so it is absent when rsync runs. It sets up a
// comparison that fails at rsync (a missing source errors out), the case that proves
// the run log records the real, non-zero exit code of a command that failed.
func aLocalSourcePathThatDoesNotExist(ctx context.Context) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-gone-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = os.RemoveAll(dir)
	if err != nil {
		return ctx, fmt.Errorf("removing %s: %w", dir, err)
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalDirectoryContainingTheseFiles creates a local tempdir populated with
// the (empty) files named in the DocString and stashes its path under
// localPathKey.
func aLocalDirectoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = writeFiles(dir, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalDirectoryInTheHomeDirectoryContainingTheseFiles builds the project inside
// the scenario's throwaway home rather than an unrelated tempdir, so a literal
// "~/project" operand has something to resolve to once csync expands it. The path
// is stashed under localPathKey like its plain twin.
func aLocalDirectoryInTheHomeDirectoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	home, _ := ctx.Value(homeKey{}).(string)
	if home == "" {
		return ctx, fmt.Errorf("scenario home directory not set")
	}
	dir := filepath.Join(home, "project")
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = writeFiles(dir, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// aLocalGitRepositoryContainingTheseFiles creates a local tempdir, initializes a
// git work tree in it, and populates it with the (empty) files named in the
// DocString — the local-side setup the .gitignore scenarios need so csync can ask
// git what to ignore. The path is stashed under localPathKey, like its non-git
// twin, so the remote-setup and file-mutation steps work against it unchanged.
func aLocalGitRepositoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	dir, err := os.MkdirTemp("", "csync-local-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		return ctx, fmt.Errorf("git init: %w", err)
	}
	err = writeFiles(dir, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, localPathKey{}, dir), nil
}

// theLocalFileContains writes the DocString to the named file in the local
// directory (e.g. ".gitignore"), establishing the ignore rules a scenario
// exercises. A trailing newline is appended so each line stands on its own. It
// backs both the "repository's" and the "directory's" phrasings: the same write
// serves a git work tree and a plain directory (the non-repo no-op scenario uses
// the latter to prove the gate is work-tree membership, not .gitignore presence).
func theLocalFileContains(ctx context.Context, name string, ds *godog.DocString) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, name)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte(strings.TrimSpace(ds.Content)+"\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	return ctx, nil
}

// aCsyncTomlInTheProjectDirectoryContaining writes the DocString to
// ./.csync.toml in the scenario's local directory — the saved-target file that
// `csync push`/`pull` read. The "user@host:/project" placeholder is rewritten to
// the scenario's real remote (the same substitution runCsync applies to a
// command-line operand) so a push/pull actually transfers against it; it is left
// verbatim when no remote was set up, which the resolution-display scenario
// relies on to assert the literal placeholder.
func aCsyncTomlInTheProjectDirectoryContaining(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	content := ds.Content
	remote := resolvedRemote(ctx)
	if remote != "" {
		content = strings.ReplaceAll(content, "user@host:/project", remote)
	}
	err := os.WriteFile(filepath.Join(local, ".csync.toml"), []byte(content), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write .csync.toml: %w", err)
	}
	return ctx, nil
}

// writeFiles creates each newline-separated relative path in content as an empty
// file under dir, making parent directories as needed; blank lines are skipped.
// Shared by the plain-directory and git-repository setup steps.
func writeFiles(dir, content string) error {
	for line := range strings.SplitSeq(strings.TrimSpace(content), "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" {
			continue
		}
		full := filepath.Join(dir, rel)
		err := os.MkdirAll(filepath.Dir(full), 0o755)
		if err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		err = os.WriteFile(full, []byte(""), 0o644)
		if err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
	}
	return nil
}

// allFilesIdenticalBetweenLocalAndRemote copies the local tree into a fresh
// remote tempdir so the two sides start identical, stashing the remote path
// under remotePathKey.
func allFilesIdenticalBetweenLocalAndRemote(ctx context.Context) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	remote, err := os.MkdirTemp("", "csync-remote-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	err = copyTree(local, remote)
	if err != nil {
		return ctx, fmt.Errorf("copy: %w", err)
	}
	return context.WithValue(ctx, remotePathKey{}, remote), nil
}

// anEmptyRemoteDirectory creates an empty remote tempdir and stashes its path
// under remotePathKey.
func anEmptyRemoteDirectory(ctx context.Context) (context.Context, error) {
	remote, err := os.MkdirTemp("", "csync-remote-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	return context.WithValue(ctx, remotePathKey{}, remote), nil
}

// aRemoteGitRepositoryContainingTheseFiles creates a remote tempdir, initializes
// a git work tree in it, and populates it with the (empty) files named in the
// DocString. It is the mirror of the local-repository step, for the scenarios
// where the repository is the side csync reads rather than the side it runs on —
// the local operand is then a plain directory, so any .git handling that consults
// the local side alone has nothing to find. The path is stashed under
// remotePathKey, like the other remote-setup steps.
func aRemoteGitRepositoryContainingTheseFiles(ctx context.Context, ds *godog.DocString) (context.Context, error) {
	remote, err := os.MkdirTemp("", "csync-remote-*")
	if err != nil {
		return ctx, fmt.Errorf("mktempdir: %w", err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = remote
	err = cmd.Run()
	if err != nil {
		return ctx, fmt.Errorf("git init: %w", err)
	}
	err = writeFiles(remote, ds.Content)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, remotePathKey{}, remote), nil
}

// theFileHasBeenChangedLocally overwrites the named file in the local tree so a
// later comparison reports it as modified.
func theFileHasBeenChangedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.WriteFile(full, []byte("modified\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	err = os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasADifferentMtimeButIdenticalContent stamps the named local file with a
// past modification time WITHOUT rewriting its bytes, so it differs from the remote
// copy in mtime alone. rsync's size+mtime quick-check then flags it for transfer
// even though the content is identical — the "phantom change" csync must not report.
// The harness otherwise creates and copies files within the same
// wall-clock second, so their mtimes compare equal at rsync's 1-second granularity;
// dating this one to the past forces the mtime-only delta the scenario needs.
func theFileHasADifferentMtimeButIdenticalContent(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasBeenAddedLocally writes a new file (creating parent dirs) into the
// local tree so a later comparison reports it as added.
func theFileHasBeenAddedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	full := filepath.Join(local, relPath)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte("new file\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	err = os.Chtimes(full, localChangeMtime, localChangeMtime)
	if err != nil {
		return ctx, fmt.Errorf("chtimes %s: %w", full, err)
	}
	return ctx, nil
}

// theFileHasBeenDeletedLocally removes the named file from the local tree, which
// (the two sides having started identical) leaves it present on the remote but
// gone from the local side — a deletion on a push. With --delete on the compare,
// a later comparison reports it as a removal.
func theFileHasBeenDeletedLocally(ctx context.Context, relPath string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	err := os.Remove(filepath.Join(local, relPath))
	if err != nil {
		return ctx, fmt.Errorf("remove %s: %w", relPath, err)
	}
	return ctx, nil
}

// theFileHasBeenAddedOnTheRemote writes a new file (creating parent dirs) into
// the remote tree so a later comparison reports it as remote-only.
func theFileHasBeenAddedOnTheRemote(ctx context.Context, relPath string) (context.Context, error) {
	remote, _ := ctx.Value(remotePathKey{}).(string)
	if remote == "" {
		return ctx, fmt.Errorf("remote path not set; missing 'identical between local and remote' step?")
	}
	full := filepath.Join(remote, relPath)
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	if err != nil {
		return ctx, fmt.Errorf("mkdir: %w", err)
	}
	err = os.WriteFile(full, []byte("remote only\n"), 0o644)
	if err != nil {
		return ctx, fmt.Errorf("write %s: %w", full, err)
	}
	return ctx, nil
}

// copyTree recursively copies the file tree rooted at src into dst, recreating
// directories and file contents (permissions are normalized, not preserved).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		err = os.MkdirAll(filepath.Dir(target), 0o755)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
