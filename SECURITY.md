# Security

Cherry-Sync drives `rsync` and `ssh` against user-supplied paths, some of which
may be untrusted (typed by a careless user, expanded from an empty shell
variable, or — eventually — read from a remote file list). This document is the
single source of truth on the tool's security posture: the invariants every
change must hold, the trust boundaries we do and don't defend, and the catalog
of concerns we've found so far.

It is deliberately terse and concrete. New concerns join the catalog as they
surface; each one carries a pointer to the test that pins it.

## Invariants

These are non-negotiable. A change that breaks one is a bug, even if every test
still passes.

- **No shell, ever.** Always invoke external commands with `exec.Command` and an
  argument *slice*. Never build a command string and hand it to `sh -c` (or
  `bash -c`, etc.). With `exec.Command` the argv goes straight to `execve`, so
  shell metacharacters (`;`, backticks, `$(…)`, `&&`, quotes) in a path are
  inert — they reach `rsync` as literal bytes in one argument. This is why the
  classic "shell injection" class does not apply to us, and it must stay that
  way.
- **`--` before positional paths handed to rsync.** `rsync` parses options
  anywhere on its command line, so a path beginning with `-` (e.g. `-e`,
  `--rsh=…`) would otherwise be read as an *option* — and `rsync`'s `-e`/`--rsh`
  can run an arbitrary remote shell command. The `--` end-of-options separator
  immediately before the paths forecloses this. See `internal/compare`
  (`rsyncArgs`).
- **Validate every path operand before use.** At minimum, reject empty strings:
  an empty path becomes `"" + "/"` = `"/"`, pointing `rsync` at the filesystem
  root. Validation lives in `cli.Parse`; surface failures as a usage error
  (exit 2), not a partial run.
- **NUL-delimit any file list handed to rsync.** When the transfer phase drives
  `rsync --files-from`, use `--from0` and NUL-delimited paths. The default
  newline delimiter lets a filename containing a newline (legal on Unix) smuggle
  extra entries into the list. (Not yet implemented — see the catalog.)

## Trust boundaries and non-goals

What we defend, and what we explicitly don't:

- **Local invocation is trusted-ish, but inputs are not.** We assume the person
  running `csync` is authorized, but we do *not* assume their arguments are
  well-formed. Untrusted-input handling (the invariants above) is about
  robustness and avoiding footguns, not about defending against a hostile local
  user — who could just run `rsync` directly.
- **SSH is the trusted transport.** We rely on the user's existing `ssh`
  configuration and host keys. We do not add our own authentication, key
  management, or host-key verification on top; that is `ssh`'s job.
- **A hostile *remote* host is out of scope.** If the remote is compromised, it
  can return misleading dry-run output or malicious file contents. We surface
  `rsync`'s errors verbatim rather than swallowing them, but we do not attempt
  to sandbox or validate what the remote sends. A user syncing with a box they
  don't control is outside our threat model.
- **No secret handling.** `csync` stores no credentials and prints no secrets;
  authentication is delegated entirely to `ssh`.

## Concern catalog

### Closed

- **rsync argument injection via leading-dash paths.** A source/destination
  beginning with `-` parsed as an rsync option (worst case: `-e`/`--rsh`
  remote-shell execution). *Fixed* by the `--` separator in `rsyncArgs`.
  Pinned by `compare.TestRsyncArgs_SeparatesOptionsFromPaths`.
- **Empty path → filesystem root.** `csync "" host:/p` would make the source
  `"/"`. *Fixed* by the empty-path rejection in `cli.Parse`. Pinned by
  `cli.TestParse_EmptyPath_ReturnsError` and the scenario "Empty path argument —
  show usage and exit non-zero" in `features/invoke-command.feature`.
- **Shell injection.** Not applicable: we use `exec.Command` with an argv slice,
  never `sh -c`. Recorded here so the question doesn't get re-litigated — the
  defense is the "No shell, ever" invariant, not a patch.

### Open / deferred

- **Newline in a filename smuggling into `--files-from`.** Harmless to the
  current compare path, but the v0.1 transfer phase will drive `rsync` via
  `--files-from`, which is newline-delimited. Mitigation: `--from0` /
  NUL-delimited paths (see invariant). Address when the transfer phase lands.
- **Whitespace-only / all-blank paths.** We currently reject only the exactly
  empty string. `"   "` becomes `"   /"`, which is not filesystem root, so we
  left it alone to avoid rejecting legitimate-if-weird paths. Revisit if it
  proves to be a real footgun.
- **Path literally `--`.** With our added `--`, a user path of `--` becomes a
  literal path named `--`. Believed harmless; noted for completeness.
