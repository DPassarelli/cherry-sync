# Pre-Publish Binary Smoke Tests — Design Spec

Status: draft for review. Scope of this revision: **Tier 1** (artifact-execution gate) specified in full; **Tier 2** (real-transfer gate) sketched so the architecture is coherent end-to-end, but committed to detail in a later pass. We are building one tier at a time, Tier 1 first.

This document is self-contained on purpose — it can be reviewed in isolation without reading the rest of the project's design notes.

## 1. Why this exists

`csync` is an interactive wrapper around `rsync` over SSH: it compares a local and a remote directory, shows what differs, lets the user pick, and transfers the selection. It ships as cross-compiled binaries published to a GitHub Release by GoReleaser on a `v*` tag.

Today the release pipeline **builds and publishes those binaries without ever running them.** A binary that won't start on its target OS/arch — a bad cross-compile, a wrong-architecture artifact, a broken `init`, a future bad `-ldflags -X` version injection — would be discovered by a user, not by us. The goal is a pre-publish gate that proves **each artifact is sound before it becomes visible to anyone.**

"Sound" has two tiers, with very different costs:

- **Tier 1 — does it execute.** Run each artifact on its target OS/arch; assert it starts, runs its own Go code, and honors its documented argument contract. Catches the entire class of "the binary is broken." Needs an execution host per platform but **no remote and no network transfer.**
- **Tier 2 — does it actually sync.** Drive each artifact through real round-trip transfers (push, pull, no-op) against a real remote `rsync` over real SSH. Catches behavior that depends on the *remote rsync implementation* — see §3.

This spec details Tier 1. Tier 2 is outlined in §9.

## 2. Deployment topology (why the platform matrix matters)

The real-world use of this tool is a local machine driving a remote dev box: e.g. a **macOS laptop** running Apple's `openrsync` pushing/pulling to a **Linux box** (Proxmox container, cloud VM, Raspberry Pi) running **GNU `rsync`**. The two ends run *different rsync implementations*.

This matters because the one thing `csync` uniquely does — parse `rsync --itemize-changes` output and drive `rsync --files-from` — is sensitive to which rsync implementation produces that output. The implementations differ observably: GNU `rsync` emits an 11-character itemized-change code; Apple's `openrsync` emits a 9-character one. `csync`'s parser is written to be width-agnostic precisely for this reason, but that asymmetry has only ever been verified by hand on a developer's Mac, never in automation.

Tier 1 doesn't touch rsync at all, but it still has to run each artifact **on the architecture it targets**, which drives the execution-host matrix in §6. Tier 2 is where the rsync-implementation asymmetry gets exercised.

## 3. What only a real remote can prove (Tier 2 motivation)

The existing hermetic test harness simulates a remote by pointing rsync's remote-shell hook (`RSYNC_RSH`) at a fake shell that strips the host and execs locally. That runs the **identical rsync binary on both ends** — so it gives zero confidence about a cross-implementation transfer. The genuinely-untested, highest-value case is the asymmetric one: openrsync (local) ↔ GNU rsync (remote). Tier 2 closes that gap. Tier 1 does not attempt to; it is the cheap precondition that fails fast before Tier 2 spends anything.

## 4. Artifacts under test

`.goreleaser.yaml` builds one binary (`csync`, `CGO_ENABLED=0`, `-ldflags -s -w`) for the cross product of `{linux, darwin} × {amd64, arm64}` — four artifacts — and packages each as a `tar.gz`:

| OS | Arch | Archive (`name_template`) | Binary inside |
|---|---|---|---|
| linux | amd64 | `cherry-sync_<version>_linux_amd64.tar.gz` | `csync` |
| linux | arm64 | `cherry-sync_<version>_linux_arm64.tar.gz` | `csync` |
| darwin | amd64 | `cherry-sync_<version>_darwin_amd64.tar.gz` | `csync` |
| darwin | arm64 | `cherry-sync_<version>_darwin_arm64.tar.gz` | `csync` |

A `checksums.txt` accompanies them. The smoke test downloads these published assets (see §7), so it exercises the exact bytes a user would receive — including the upload step, not just the local `dist/` output.

## 5. Locked decisions

These were settled in design discussion and are treated as fixed for this spec:

1. **Both tiers, built one at a time, Tier 1 first.**
2. **linux/arm64 is in scope.** GitHub provides no native Linux/arm64 runner, so its execution host is an ephemeral Azure arm64 VM (§6, §8). This is the only cloud dependency in Tier 1.
3. **Gating mechanism is draft-then-promote** (§7): GoReleaser publishes the release as a *draft* (invisible to users); smoke jobs run against the draft's assets; the release is flipped to non-draft only if every smoke job is green.
4. **Trigger is the `v*` tag** (pre-publish, in `release.yml`), and the same work must be **drivable by hand** so a CI failure can be reproduced and investigated. This forces a script-first architecture (§7).
5. **Tier 1 ships in two phases**, split along execution-host difficulty:
   - **Phase 1 — native GitHub runners only:** linux/amd64 (`ubuntu-latest`) and darwin/arm64 (`macos-latest`). No Azure, no Rosetta, no new cloud credential. This phase proves the full draft → smoke → promote machinery and `scripts/smoke.sh` on the two hosts that run for free.
   - **Phase 2 — the harder hosts:** darwin/amd64 (Rosetta on `macos-latest`) and linux/arm64 (ephemeral Azure arm64 VM). This phase adds only execution hosts to an already-proven gate.

## 6. Tier 1 — definition and execution-host matrix

### 6.1 What Tier 1 asserts

The "documented argument contract" Tier 1 checks is the one `cmd/csync/main.go` implements today: invoked with no (or invalid) arguments, `csync` prints `usage: csync SOURCE DESTINATION` to **stderr** and exits **2**. Tier 1 asserts:

- the binary executes on its target OS/arch (it loads and runs Go code rather than failing to start), and
- with no arguments it exits `2` and writes a line containing `usage:` to stderr.

That is sufficient to catch a broken artifact. It deliberately does **not** invoke `compare`/`transfer` (those need rsync and a peer — Tier 2).

> Contract dependency: if a `--version` flag is added later (currently deferred until in-binary version injection exists), Tier 1 should also assert `csync --version` prints a non-empty version and exits `0`. Until then the no-argument usage path is the smoke-able surface. Whoever adds `--version` updates this assertion in lockstep.

### 6.2 Execution hosts

The artifact under test is the *local* csync. For Tier 1 it only needs a host that can execute its arch — no remote.

| Phase | Artifact | Execution host | How it runs |
|---|---|---|---|
| 1 | linux/amd64 | `ubuntu-latest` GitHub runner | native |
| 1 | darwin/arm64 | `macos-latest` GitHub runner (Apple silicon) | native |
| 2 | darwin/amd64 | `macos-latest` GitHub runner | via Rosetta 2 (`arch -x86_64 ./csync`); install with `softwareupdate --install-rosetta --agree-to-license` if absent |
| 2 | linux/arm64 | **ephemeral Azure arm64 VM** | `scp` the binary, run over SSH |

**Phase 1** covers the two artifacts that run natively on GitHub-hosted runners — no Azure, no Rosetta, no cloud credential. **Phase 2** adds the two harder hosts: darwin/amd64 (Rosetta) and linux/arm64 (the only artifact needing Azure, and only to *execute* the binary — no Go toolchain, no rsync peer). The Phase 2 cloud footprint is **one short-lived arm64 VM, alive for minutes.**

## 7. Tier 1 — architecture (scripts + workflow)

Mirrors the pattern already used for the test-report dashboard: **the script is the unit of work; the workflow is a thin caller**, so what CI runs is exactly what a human can run by hand. Nothing the gate does is reproducible only inside Actions.

### 7.1 Scripts (the source of truth)

- **`scripts/smoke.sh <path-to-csync-binary>`** — the assertion runner. Executes the given binary with no arguments and asserts the §6.1 contract (exit `2`, `usage:` on stderr). Prints a clear pass/fail line and exits non-zero on failure. Host-agnostic: it does not care whether the binary arrived via download, `scp`, or a local build. This is the entire Tier 1 behavior; a human runs `./scripts/smoke.sh ./csync` directly.

  *Teeth requirement (house testing rule):* `smoke.sh` must be verified against both a known-good binary (real `csync` → green) and a degenerate stand-in that violates the contract (e.g. `/bin/true`, which exits `0` and prints nothing → the script must go red). Confirm the failure is for the right reason before trusting the gate.

- **`scripts/azure-smoke-vm.sh up|down`** — provisions / tears down the ephemeral arm64 VM and prints connection details (public IP, SSH user). Parameterized (resource-group name, region, VM size, SSH key path) so a human can stand the same environment up locally for debugging and tear it down when done.

### 7.2 Workflow wiring (`release.yml`)

The release job is split so the smoke gate sits between build and publish:

1. **Build (draft).** Set `release.draft: true` in `.goreleaser.yaml`. GoReleaser builds all four artifacts and creates the GitHub Release as a **draft** with assets attached — invisible to users. The existing release-notes extraction and pre-release-flag enforcement stay here, applied to the draft.
2. **Smoke (fan-out).** One job per execution host in §6.2. Each downloads its artifact from the draft release (`gh release download <tag> --pattern 'cherry-sync_*_<os>_<arch>.tar.gz'`), unpacks the `csync` binary, and runs `scripts/smoke.sh` against it — directly on `ubuntu-latest`/`macos-latest`, or (Phase 2 only) by `scp`-then-SSH onto the Azure arm64 VM (`scripts/azure-smoke-vm.sh up`). In Phase 1 this is two jobs, both on native GitHub runners.
3. **Promote (gated).** A final job that `needs` every smoke job. On success it flips the release live: `gh release edit <tag> --draft=false` (folded into the existing `gh release edit` enforcement step). If any smoke job failed, the release stays a draft and the workflow fails.

Downloading from the draft release (rather than reusing GoReleaser's local `dist/`) means the bytes smoked are the bytes that promotion reveals — the upload itself is covered.

### 7.3 Azure access and teardown safety (Phase 2 only)

Phase 1 has no Azure dependency; this section applies only when Phase 2 adds the linux/arm64 leg.


- **Auth.** GitHub → Azure via OIDC federation (`azure/login` with a federated credential scoped to this repo) — no long-lived secret in CI. The service principal is least-privilege: rights to create and delete resource groups within a dedicated subscription/scope, nothing more. This is the one new credential surface the gate introduces; it is kept narrow deliberately, consistent with the project's security posture.
- **Teardown.** Cost is not the primary risk; *leaked infrastructure* is. Three layers:
  1. **Immediate on success.** Tear down in a step with `if: success()` (`az group delete --yes --no-wait`).
  2. **Held on failure for debugging.** With `if: failure()`, leave the VM up and print its connection details. This is the deliberate replication window — a developer SSHes into the exact environment that failed. (This, not a default timer, is what the "keep it for ~24h" idea is for: a failure-path debug affordance, not the normal path.)
  3. **Independent backstop.** A separate scheduled workflow sweeps any smoke resource group older than ~24h (matched by a naming convention such as `csync-smoke-<run-id>` and/or a `delete-after` tag), so a crashed or cancelled run can never leak indefinitely. An Azure budget alert is a further safety net.

### 7.4 Failure handling and manual replication

A red smoke job (a) leaves the release unpublished as a draft, (b) on the Azure leg, leaves the VM up with printed connection details, and (c) is reproducible by hand: download the same artifact and run `./scripts/smoke.sh ./csync`, or stand up the VM with `./scripts/azure-smoke-vm.sh up` and repeat there. There is no CI-only state.

## 8. Tier 1 — cost and risk summary

- **Phase 1 — zero cloud footprint.** Both legs run on GitHub-hosted runners. No Azure account, no OIDC credential, no teardown, no Rosetta dependency. Cost is GitHub Actions minutes only.
- **Cost (Phase 2).** One B-/D-series arm64 VM for the minutes a run takes — pennies per release. Everything else still runs on GitHub-hosted runners.
- **Flakiness (Phase 2).** The Azure leg adds a network/provisioning dependency. It runs **only on `v*` tags** (and manual dispatch), never on `pull_request`, so it cannot make day-to-day `go test` flaky. A provisioning failure blocks a *release*, which is the correct place to absorb that risk.
- **Security (Phase 2).** A scoped OIDC service principal with RG create/delete is the only new privilege; no static cloud secret is stored.

## 9. Tier 2 — forward look (not yet committed to detail)

Recorded here so the architecture is coherent and Tier 2 can be resumed without re-deriving it.

Tier 2 reuses the existing Gherkin suite rather than reimplementing assertions, because the harness (`features_test.go`) already **builds and executes a real `csync` binary** (`go build` → `exec.Command(csyncBinary, ...)`) and already centralizes remote handling in its `runCsync` step. Two seams make the suite drive a *released artifact* against a *real remote*:

1. **`CSYNC_BINARY` override** — if set, the harness drives that binary instead of building one from source. Turns "test the source" into "test the shipped artifact." One small, test-first-able change to the build step in `features_test.go`.
2. **`CSYNC_REMOTE=ssh` provider** — a real-SSH remote provider alongside today's fake-rsh one, pointing at a provisioned remote.

A curated subset of scenarios is tagged `@smoke` (target ~3–5: one push, one pull, an identical-bytes no-op, and the non-ASCII-filename transfer, which is openrsync-sensitive). Those scenarios then run at two fidelities from one spec: hermetically on every PR (fake-rsh, today) and for-real pre-release via `CSYNC_BINARY=<artifact> CSYNC_REMOTE=ssh go test -godog.tags=@smoke`. The same command reproduces a Tier 2 CI failure locally.

**Topology requirement and a trap to avoid.** The high-value case is openrsync (local) ↔ GNU rsync (remote), and getting it faithfully in automation has a hard constraint:

- The faithful `openrsync` is **Apple's**, which exists only on macOS. The portable Linux `openrsync` is a different, feature-incomplete fork that does not implement `--itemize-changes` / `--files-from` / `--from0`, so a Linux "openrsync" VM would represent a build `csync` can't even drive. **Do not install openrsync on a Linux VM to fake the asymmetry.**
- Therefore the openrsync *driver* must be a real Mac. The **GitHub `macos-latest` runner** is that Mac, for free. The Azure VM serves as the **GNU-rsync remote** the Mac drives against. (A Linux runner driving the same Azure remote gives the lower-value GNU↔GNU case — still real SSH and two real filesystems, but not the implementation asymmetry.)

A Tier 2 wrinkle to settle when we get there: the harness needs a Go toolchain wherever it runs, which is a question for the linux/arm64 leg specifically (install Go on the VM, or cross-compile the test binary with `go test -c` and ship it). Out of scope for Tier 1.

## 10. Open questions

Phase 1:

- **`gh release download` from a draft:** confirm the workflow token can list and download draft-release assets by tag (expected: yes, with `contents: write`), and pin the exact `--pattern`.
- **Promote step idempotency:** ensure flipping `--draft=false` composes cleanly with the existing post-release `gh release edit` enforcement (notes + pre-release flag) rather than racing it.

Phase 2:

- **darwin/amd64 on Apple-silicon runners:** confirm whether `macos-latest` ships Rosetta 2 by default or needs the explicit install step; pin the approach once verified on a real runner.
- **Azure region / VM size:** pick the cheapest arm64 size that boots quickly in a nearby region; confirm the chosen image ships an SSH server out of the box.
