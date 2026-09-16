// Invoking csync from a scenario: the When steps that start it, the environment
// a child is given, and the supervised run that captures its output and exit.

package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// iRun executes the `When I run "..."` step with no interactive input. It also
// backs the `... a second time` phrasing: the scenario's local and remote
// tempdirs persist in the context across steps, so a repeat invocation runs
// against the already-synced state — which is what idempotence checks assert on.
// With no stdin, a second run that does surface phantom changes reads EOF at the
// prompt and selects nothing rather than blocking.
func iRun(ctx context.Context, command string) (context.Context, error) {
	return runCsync(ctx, command, nil, "")
}

// iRunFromTheProjectDirectory backs `When I run "..." from the project
// directory`: it runs csync with its working directory set to the scenario's
// local tempdir, so cwd-only .csync.toml discovery finds the dotfile the
// config-write step placed there. The verb forms (push/pull) take no operands,
// so no stdin is fed.
func iRunFromTheProjectDirectory(ctx context.Context, command string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	return runCsync(ctx, command, nil, local)
}

// iRunFromTheProjectDirectoryAndRespond backs `When I run "..." from the project
// directory and respond with "..."`: like iRunFromTheProjectDirectory but feeds
// the prompt response on stdin, so a push/pull resolved from .csync.toml can be
// driven through the selection prompt. The `<empty>` sentinel stands for a bare
// Enter, as in iRunAndRespond.
func iRunFromTheProjectDirectoryAndRespond(ctx context.Context, command, response string) (context.Context, error) {
	local, _ := ctx.Value(localPathKey{}).(string)
	if local == "" {
		return ctx, fmt.Errorf("local path not set; missing Background step?")
	}
	if response == "<empty>" {
		response = ""
	}
	return runCsync(ctx, command, strings.NewReader(response+"\n"), local)
}

// iRunAndRespond executes the `When I run "..." and respond with "..."` step,
// feeding the response to csync on stdin as if typed at the prompt. The
// `<empty>` sentinel stands for an empty response (a bare Enter); a trailing
// newline is appended so the response reads as a completed line.
func iRunAndRespond(ctx context.Context, command, response string) (context.Context, error) {
	if response == "<empty>" {
		response = ""
	}
	return runCsync(ctx, command, strings.NewReader(response+"\n"), "")
}

// resolvedRemote returns the real remote operand that the Gherkin placeholder
// "user@host:/project" stands for in this scenario: a `fakehost:` path under
// @remote (so rsync runs in sender/receiver mode via RSYNC_RSH), a bare local
// path otherwise, or "" when no remote directory was set up. Both runCsync (for
// command-line operands) and the .csync.toml write step resolve through this, so
// a remote read from config points at the same place as one passed on argv.
func resolvedRemote(ctx context.Context) string {
	remotePath, _ := ctx.Value(remotePathKey{}).(string)
	if remotePath == "" {
		return ""
	}
	remoteMode, _ := ctx.Value(remoteModeKey{}).(bool)
	if remoteMode {
		return "fakehost:" + remotePath
	}
	return remotePath
}

// stateHome returns the directory the csync child sees as XDG_STATE_HOME, or ""
// when no scenario home was set up. It is deliberately NOT the home directory's
// ~/.local/state: csync falls back to that path when the variable is unset, so
// pointing both at the same place would let a csync that ignored XDG_STATE_HOME
// pass the location scenario by accident. Kept distinct, the two are
// distinguishable, and a run that lands in the wrong one is visible.
func stateHome(ctx context.Context) string {
	home, _ := ctx.Value(homeKey{}).(string)
	if home == "" {
		return ""
	}
	return filepath.Join(home, "xdg-state")
}

// csyncEnv builds the environment for the csync child: the ambient environment
// with HOME and XDG_STATE_HOME redirected into the scenario's throwaway home, and
// RSYNC_RSH added under @remote.
//
// The two variables are filtered out of the ambient environment before being set,
// rather than appended to override by last-duplicate-wins. Filtering is what makes
// the fallback testable: under `XDG_STATE_HOME is not set` the child must see no
// XDG_STATE_HOME at all, which an append can never achieve if the developer's own
// shell exported one. It also makes the child's view of both variables the same
// whoever runs the suite.
func csyncEnv(ctx context.Context) []string {
	env := os.Environ()
	home, _ := ctx.Value(homeKey{}).(string)
	if home != "" {
		env = withoutVars(env, "HOME", "XDG_STATE_HOME")
		env = append(env, "HOME="+home)
		noXdg, _ := ctx.Value(noXdgKey{}).(bool)
		if !noXdg {
			env = append(env, "XDG_STATE_HOME="+stateHome(ctx))
		}
	}
	remoteMode, _ := ctx.Value(remoteModeKey{}).(bool)
	if remoteMode {
		// rsync reads RSYNC_RSH as its remote shell; fakeRsh execs locally so the
		// `fakehost:` operand transfers on this machine over the real remote code
		// path. csync's own rsync child inherits this environment.
		rsh := fakeRsh
		failMeasure, _ := ctx.Value(failMeasureModeKey{}).(bool)
		if failMeasure {
			rsh = failMeasureRsh
		}
		stallMode, _ := ctx.Value(stallModeKey{}).(bool)
		if stallMode {
			// Swap in the shell that stops answering after the comparison, and shorten
			// csync's own bound so the scenario doesn't wait out the shipped default.
			rsh = stallRsh
			env = append(env, "CSYNC_STALL_TIMEOUT="+stallTestTimeout)
		}
		env = append(env, "RSYNC_RSH="+rsh)
	}
	return env
}

// withoutVars returns env with every `NAME=...` entry for the named variables
// removed, so a caller can set them cleanly without an ambient value surviving.
func withoutVars(env []string, names ...string) []string {
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	kept := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if drop[name] {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// runCsync splits the command, substitutes the Gherkin placeholders
// (`./project`, `user@host:/project`, `<empty>`) with the scenario's real
// tempdir paths, runs the csync binary with the given stdin, and stashes the
// captured streams and exit code in the context.
func runCsync(ctx context.Context, command string, stdin io.Reader, dir string) (context.Context, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ctx, fmt.Errorf("empty command")
	}
	if parts[0] != "csync" {
		return ctx, fmt.Errorf("expected command to start with %q, got %q", "csync", parts[0])
	}

	args := parts[1:]
	// <empty> is a sentinel for an empty-string argument: the step regex and
	// strings.Fields can't carry a literal "" through the Gherkin command, so
	// scenarios write <empty> and we substitute it here.
	subs := map[string]string{"<empty>": ""}
	localPath, _ := ctx.Value(localPathKey{}).(string)
	if localPath != "" {
		subs["./project"] = localPath
	}
	remote := resolvedRemote(ctx)
	if remote != "" {
		subs["user@host:/project"] = remote
	}
	for i, a := range args {
		replacement, ok := subs[a]
		if ok {
			args[i] = replacement
		}
	}

	runCtx, cancelRun := context.WithTimeout(ctx, runWait)
	defer cancelRun()
	cmd := exec.CommandContext(runCtx, csyncBinary, args...)
	cmd.Stdin = stdin
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = csyncEnv(ctx)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()

	// A csync that never exits is a failure of the thing under test, not a slow
	// machine: report it as one rather than letting the killed child's signal be
	// read as an ordinary non-zero exit.
	if runCtx.Err() != nil {
		return ctx, fmt.Errorf("csync did not exit within %s; stderr so far:\n%s", runWait, stderrBuf.String())
	}

	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return ctx, fmt.Errorf("exec failed: %w", err)
		}
		exitCode = exitErr.ExitCode()
	}

	result := runResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}
	ctx = context.WithValue(ctx, invokedArgsKey{}, args)
	return context.WithValue(ctx, outputKey{}, result), nil
}
