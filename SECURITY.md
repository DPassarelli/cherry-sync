# Security

Cherry-Sync drives `rsync` and `ssh` against user-supplied paths, some of which may be untrusted (typed by a careless user, expanded from an empty shell variable, or — eventually — read from a remote file list). This document is the single source of truth on the tool's security posture: the invariants every change must hold, the trust boundaries we do and don't defend, and the catalog of concerns we've found so far.

It is deliberately principle-first: it records threats and the invariants that counter them, not the code that implements them — so a reviewer reasons from the threat model rather than from our current implementation. New concerns join the catalog as they surface; closed ones are held closed by regression tests.

## Invariants

These are non-negotiable. A change that breaks one is a bug, even if every test still passes.

- **No shell, ever.** Always invoke external commands with `exec.Command` and an argument *slice*. Never build a command string and hand it to `sh -c` (or `bash -c`, etc.). With `exec.Command` the argv goes straight to `execve`, so shell metacharacters (`;`, backticks, `$(…)`, `&&`, quotes) in a path are inert — they reach `rsync` as literal bytes in one argument. This is why the classic "shell injection" class does not apply to us, and it must stay that way.
- **An end-of-options (`--`) separator before user paths.** `rsync` parses options anywhere on its command line, so a path beginning with `-` would otherwise be read as an *option*. The danger is concrete: rsync's remote-shell option (`-e`/`--rsh`) executes an arbitrary command on the far side. A `--` end-of-options marker immediately before the user-supplied paths forecloses this — everything after it is a path, never an option.
- **Validate every path operand before use.** At minimum, reject empty strings: an empty path becomes `"" + "/"` = `"/"`, pointing `rsync` at the filesystem root. Validation belongs at the input boundary; surface failures as a usage error (exit 2), not a partial run.
- **NUL-delimit any file list handed to rsync.** When a transfer is driven from a list of paths, delimit it with NUL bytes rather than newlines. The default newline delimiter lets a filename containing a newline (legal on Unix) smuggle extra entries into the list. (Not yet implemented — see the catalog.)

## Trust boundaries and non-goals

What we defend, and what we explicitly don't:

- **Local invocation is trusted-ish, but inputs are not.** We assume the person running `csync` is authorized, but we do *not* assume their arguments are well-formed. Untrusted-input handling (the invariants above) is about robustness and avoiding footguns, not about defending against a hostile local user — who could just run `rsync` directly.
- **SSH is the trusted transport.** We rely on the user's existing `ssh` configuration and host keys. We do not add our own authentication, key management, or host-key verification on top; that is `ssh`'s job.
- **A hostile *remote* host is out of scope.** If the remote is compromised, it can return misleading dry-run output or malicious file contents. We surface `rsync`'s errors verbatim rather than swallowing them, but we do not attempt to sandbox or validate what the remote sends. A user syncing with a box they don't control is outside our threat model.
- **No secret handling.** `csync` stores no credentials and prints no secrets; authentication is delegated entirely to `ssh`.

## Concern catalog

### Closed

- **rsync argument injection via leading-dash paths.** A source or destination beginning with `-` parsed as an rsync option — worst case, a remote-shell option executing an arbitrary command on the far side. *Closed* by the end-of-options separator invariant, and held closed by a behavioral test that fails if the separator is removed.
- **Empty path → filesystem root.** An empty path operand would expand to `"/"`, retargeting the operation at the root of the filesystem. *Closed* by the path-validation invariant (empty-string rejection), held closed by unit and behavioral regression tests. The operand normalization in `internal/operand` preserves this invariant: a path that reduces to nothing after resolving a `~` home shortcut or collapsing trailing slashes (a bare `~`, a `host:` with no path, or a run of slashes) becomes `"."` — the directory itself — never the empty string.
- **Shell injection.** Not applicable: we use `exec.Command` with an argv slice, never `sh -c`. Recorded here so the question doesn't get re-litigated — the defense is the "No shell, ever" invariant, not a patch.
- **Filter-rule metacharacter in a deletion path.** A selected deletion is applied with an rsync filter rule (`--delete --include=<path> … --exclude='*'`), in which `*`, `?`, and `[` are glob metacharacters — an unescaped one in a filename would make the include match, and `--delete` remove, files the user never chose. The filenames themselves still reach rsync as inert argv entries (the "No shell, ever" invariant holds), but rsync's *own* pattern matcher reinterprets them. *Closed for now* by dropping any delete candidate whose name contains such a character — and any leading space, which the `*deleting` itemize parser can't reliably tell apart from the code column's padding — at detection, so it is never offered or applied; held closed by unit and behavioral tests. Making those names removable (escaping the metacharacters, parsing the leading-space case by fixed column width) is deferred to a tracked issue.

### Open / deferred

- **Newline in a filename smuggling into the transfer list.** Harmless on the current compare-only path, but the v0.1 transfer phase will drive `rsync` from a list of paths, newline-delimited by default. Mitigation: NUL-delimited paths (see invariant). Address when the transfer phase lands.
- **Whitespace-only / all-blank paths.** We currently reject only the exactly empty string. `"   "` becomes `"   /"`, which is not filesystem root, so we left it alone to avoid rejecting legitimate-if-weird paths. Revisit if it proves to be a real footgun.
- **Path literally `--`.** With the end-of-options separator in place, a user path of `--` resolves to a literal path named `--`. Believed harmless; noted for completeness.

## Automated checks

`gosec` runs as a deterministic gate in the `lefthook` pre-push hook — static analysis, no LLM, no session bias, and it can't be forgotten because it gates the push itself. It flags any `exec.Command` with variable arguments as G204.

The single rsync invocation with variable arguments carries a justified, per-site `#nosec G204`: its safety rests on the invariants above and is *proven* by a behavioral test that goes red if the end-of-options separator is removed.

Suppress G204 only per-site, with a justification that points at the reasoning. Never disable it globally — that would blind the gate to the same pattern everywhere else in the tree.
