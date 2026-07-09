# Binary Smoke Testing — Design & Rationale

**Tier 1** (the artifact-execution gate) is built and gates every release: on a `v*` tag each of the four published binaries is run on its target OS/arch — natively for linux/amd64 and darwin/arm64, via Rosetta for darwin/amd64, and on an ephemeral Azure VM for linux/arm64 — and the release stays an invisible draft unless all four pass. It went live with v0.7.0. **Tier 2** (a real-transfer gate) is not built; it's sketched in §9 so the architecture stays coherent and can be resumed without re-deriving it.

This document records why the gate exists and how it works. It's self-contained — readable without the rest of the project's design notes.

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

`.goreleaser.yaml` builds one binary (`csync`, `CGO_ENABLED=0`, `-ldflags -s -w`) for the cross product of `{linux, darwin} × {amd64, arm64}` — four artifacts — and publishes each as a bare executable (`formats: [binary]`), not an archive:

| OS | Arch | Release asset (`name_template`) |
|---|---|---|
| linux | amd64 | `cherry-sync_<version>_linux_amd64` |
| linux | arm64 | `cherry-sync_<version>_linux_arm64` |
| darwin | amd64 | `cherry-sync_<version>_darwin_amd64` |
| darwin | arm64 | `cherry-sync_<version>_darwin_arm64` |

A `checksums.txt` accompanies them. The smoketest downloads these published assets (see §7), so it exercises the exact bytes a user would receive — including the upload step, not just the local `dist/` output. A release asset is an opaque blob, so the executable bit does not survive the round trip: whatever runs the downloaded binary must `chmod +x` it first. The workflow's download step does that, and `smoketest.sh` fails fast with an explicit "binary is not executable" message if it is ever skipped.

## 5. Key decisions

The decisions that shaped Tier 1:

1. **Both tiers, Tier 1 first.** Tier 2 (§9) is deferred, not abandoned.
2. **linux/arm64 is in scope.** GitHub provides no native Linux/arm64 runner, so its execution host is an ephemeral Azure arm64 VM (§6, §8). This is the only cloud dependency in Tier 1.
3. **Gating mechanism is draft-then-promote** (§7): GoReleaser publishes the release as a *draft* (invisible to users); smoketest jobs run against the draft's assets; the release is flipped to non-draft only if every smoketest job is green.
4. **Trigger is the `v*` tag** (pre-publish, in `release.yml`), and the same work must be **drivable by hand** so a CI failure can be reproduced and investigated. This forces a script-first architecture (§7).

## 6. Tier 1 — definition and execution-host matrix

### 6.1 What Tier 1 asserts

The "documented argument contract" Tier 1 checks is the one `cmd/csync/main.go` implements today: invoked with no (or invalid) arguments, `csync` prints `usage: csync SOURCE DESTINATION` to **stderr** and exits **2**. Tier 1 asserts:

- the binary executes on its target OS/arch (it loads and runs Go code rather than failing to start),
- with no arguments it exits `2` and writes a line containing `usage:` to stderr, and
- with `--version` it exits `0` and writes a `cherry-sync` version line to stdout; when the caller names the expected version (the release workflow passes the tag), that line must contain it.

That is sufficient to catch a broken artifact. It deliberately does **not** invoke `compare`/`transfer` (those need rsync and a peer — Tier 2).

The `--version` assertion exists specifically to exercise the `-ldflags -X main.version` injection — the one failure class named in §1 that the no-argument path can't reach. A binary built without the tag injected renders `cherry-sync (dev build)` instead of `cherry-sync v1.2.3`; because the release legs pass `${GITHUB_REF_NAME}` (which the rendered line contains verbatim, since goreleaser strips the leading `v` and csync re-adds it), such a binary fails the gate and never publishes. A local hand-run passes no version and so only checks that `--version` exits `0` and prints the project line, keeping the script runnable against a dev build.

### 6.2 Execution hosts

The artifact under test is the *local* csync. For Tier 1 it only needs a host that can execute its arch — no remote.

| Artifact | Execution host | How it runs |
|---|---|---|
| linux/amd64 | `ubuntu-latest` GitHub runner | native |
| darwin/arm64 | `macos-latest` GitHub runner (Apple silicon) | native |
| darwin/amd64 | `macos-latest` GitHub runner | via Rosetta 2 — macOS runs the x86_64 binary transparently on exec once Rosetta is installed; no `arch` prefix needed. The leg runs `softwareupdate --install-rosetta --agree-to-license` first (a no-op if already present). |
| linux/arm64 | **ephemeral Azure arm64 VM** | `scp` the binary, run over SSH |

Three legs run on GitHub-hosted runners — linux/amd64 and darwin/arm64 natively, darwin/amd64 via Rosetta. linux/arm64 is the only one needing an external host, since GitHub offers no native Linux/arm64 runner: an ephemeral Azure arm64 VM, used only to *execute* the binary (no Go toolchain, no rsync peer), so the cloud footprint is one short-lived VM alive for minutes. All four run in `release.yml` — the linux/arm64 leg being the `smoketest-linux-arm64` job plus `_scripts/azure-smoketest-vm.sh`, whose one-time Azure setup is in §7.3.2 — and have gated real releases since v0.7.0.

## 7. Tier 1 — architecture (scripts + workflow)

Mirrors the pattern already used for the test-report dashboard: **the script is the unit of work; the workflow is a thin caller**, so what CI runs is exactly what a human can run by hand. Nothing the gate does is reproducible only inside Actions.

### 7.1 Scripts (the source of truth)

- **`_scripts/smoketest.sh <path-to-csync-binary> [expected-version]`** — the assertion runner. Executes the given binary with no arguments and with `--version`, asserting the §6.1 contract (no-arg: exit `2`, `usage:` on stderr; `--version`: exit `0`, `cherry-sync` line on stdout, and — when `expected-version` is given — that line contains it). Prints a clear pass/fail line and exits non-zero on failure. Host-agnostic: it does not care whether the binary arrived via download, `scp`, or a local build. A human runs `./_scripts/smoketest.sh ./csync` directly; the release legs additionally pass `${GITHUB_REF_NAME}` to gate version injection.

  *Teeth requirement (house testing rule):* `smoketest.sh` must be verified against both a known-good binary (real `csync` → green) and a degenerate stand-in that violates the contract (e.g. `/bin/true`, which exits `0` and prints nothing csync-shaped → the script must go red). For the version check specifically, verify that a dev-build binary passed an expected release version goes red (the broken-injection class) while the same binary with no expected version stays green. Confirm each failure is for the right reason before trusting the gate.

- **`_scripts/azure-smoketest-vm.sh up|down`** — provisions / tears down the ephemeral arm64 VM and prints connection details (public IP, SSH user). Parameterized (resource-group name, region, VM size, SSH key path) so a human can stand the same environment up locally for debugging and tear it down when done.

### 7.2 Workflow wiring (`release.yml`)

The release job is split so the smoketest gate sits between build and publish:

1. **Build (draft).** Set `release.draft: true` in `.goreleaser.yaml`. GoReleaser builds all four artifacts and creates the GitHub Release as a **draft** with assets attached — invisible to users. The existing release-notes extraction and pre-release-flag enforcement stay here, applied to the draft.
2. **Smoketest (fan-out).** One job per execution host in §6.2. Each downloads its artifact from the draft release (`gh release download <tag> --pattern 'cherry-sync_*_<os>_<arch>'`), renames it to `csync` and restores its executable bit, then runs `_scripts/smoketest.sh` against it — directly on `ubuntu-latest`/`macos-latest`, or for linux/arm64 by `scp`-then-SSH onto the Azure arm64 VM (`_scripts/azure-smoketest-vm.sh up`).
3. **Promote (gated).** A final job that `needs` every smoketest job. On success it flips the release live: `gh release edit <tag> --draft=false` (folded into the existing `gh release edit` enforcement step). If any smoketest job failed, the release stays a draft and the workflow fails.

Downloading from the draft release (rather than reusing GoReleaser's local `dist/`) means the bytes the smoketest runs are the bytes that promotion reveals — the upload itself is covered.

### 7.3 The linux/arm64 leg: an ephemeral Azure VM

The other three legs have no Azure dependency. This one needs an external host because GitHub offers no native Linux/arm64 runner. It does the same thing every other leg does — execute the artifact and assert the §6.1 contract — just on a VM stood up and torn down per run. It needs **no Go toolchain and no rsync peer** on the VM; it only runs the binary.

Because it needs an OIDC token, a GitHub Environment, and SSH — none of which the GitHub-runner legs need — it is a **separate job**, not another `matrix` leg under `smoketest`.

**Resource naming (Microsoft Cloud Adoption Framework).** Every Azure resource this leg creates follows the CAF pattern `<type>-<workload>-<purpose>-<region>[-<instance>]`, using the CAF resource-type abbreviations. The workload token is `csync` and the purpose token is `smoketest`; `<region>` is the CAF region abbreviation for the chosen region (`eus` for `eastus`). The AD app registration and service principal aren't ARM resources, so they're out of scope for this convention and keep their given name, `cherry-sync-test`.

Because the service principal is scoped to a single resource group and can't create resource groups (§7.3.1), the resources split into **stable** (created once by hand in §7.3.2, no instance suffix) and **ephemeral** (created and destroyed per run by the script in §7.3.3, carrying the GitHub run ID as the instance token so repeated or concurrent runs never collide):

| Lifecycle | Resource | CAF name |
|---|---|---|
| stable | resource group | `rg-csync-smoketest-<region>` |
| stable | virtual network | `vnet-csync-smoketest-<region>` |
| stable | subnet | `snet-csync-smoketest-<region>` |
| ephemeral | virtual machine | `vm-csync-smoketest-<region>-<runid>` |
| ephemeral | OS managed disk | `osdisk-csync-smoketest-<region>-<runid>` |
| ephemeral | network interface | `nic-csync-smoketest-<region>-<runid>` |
| ephemeral | public IP address | `pip-csync-smoketest-<region>-<runid>` |
| ephemeral | network security group | `nsg-csync-smoketest-<region>-<runid>` |

For a local manual run `<runid>` defaults to `local`.

#### 7.3.1 Authentication — OIDC, no stored secret

GitHub Actions mints a short-lived OIDC token that `azure/login` exchanges for an Azure access token; there is no client secret stored anywhere. This is the one new credential surface the gate introduces, so it is scoped deliberately narrowly (consistent with the project's security posture).

**The subject-stability problem.** An Azure *federated credential* matches an exact OIDC `subject` claim. For a tag-triggered run the subject is `repo:OWNER/REPO:ref:refs/tags/<tag>` — different for every tag, so a per-tag credential is unworkable. The fix is to run the Azure job inside a GitHub **Environment** (e.g. `release`): the subject then becomes the stable `repo:OWNER/REPO:environment:release`, constant across tags. The job declares `environment: release`. (A side benefit: Environments can require a reviewer, giving an optional manual approval gate before any cloud spend.) The scheduled **sweeper** does the opposite on purpose — it must *not* use an Environment, because each Environment run logs a Deployment and a scheduled job would inflate the repo's deployment count (issue #78). It doesn't have the per-tag problem anyway: scheduled runs execute on the default branch, so it trusts the stable branch subject `repo:OWNER/REPO:ref:refs/heads/main` via a second federated credential (§7.3.2, §7.3.6).

**Least privilege — a dedicated resource group, not the subscription.** There is no spare subscription set aside for this work; the subscription also holds hand-built and other-project resource groups. A role assignment at *subscription* scope would let a compromised CI token reach all of them, so the scope is instead a **single dedicated resource group**, `rg-csync-smoketest-<region>`, created once by hand (§7.3.2). The principal is **Contributor scoped to that resource group only** — enough to create and delete the ephemeral VM, disk, NIC, public IP, and per-run NSG inside it, and structurally incapable of touching any other resource group.

Two properties of this boundary are worth stating:
- **Contributor can't escalate itself.** The Contributor role excludes `Microsoft.Authorization/*/write`, so the principal can't broaden its own scope or hand out role assignments. The resource-group wall holds.
- **It *can* delete its own resource group** (deleting an RG is a Contributor action and the assignment is on that RG) — still within the blast-radius guarantee, since that's only *this* RG. Teardown (§7.3.5) deliberately deletes the ephemeral resources and leaves `rg-csync-smoketest-<region>` standing as the stable container. If even self-deletion of the group must be denied, swap Contributor for a custom role omitting `Microsoft.Resources/subscriptions/resourceGroups/delete`; that's gold-plating, and RG-scoped Contributor is the standard answer.

#### 7.3.2 One-time setup (run by a human, once)

```sh
SUBSCRIPTION_ID=$(az account show --query id -o tsv)
TENANT_ID=$(az account show --query tenantId -o tsv)

REGION=eastus          # region
REGION_ABBR=eus        # its CAF abbreviation, used in every resource name
RG=rg-csync-smoketest-${REGION_ABBR}

# App registration + service principal (not an ARM resource; keeps the name cherry-sync-test)
az ad app create --display-name cherry-sync-test
APP_ID=$(az ad app list --display-name cherry-sync-test --query "[0].appId" -o tsv)
az ad sp create --id "$APP_ID"

# Federated credential trusting this repo's `release` environment (stable subject
# for the tag-triggered release smoketest job)
az ad app federated-credential create --id "$APP_ID" --parameters '{
  "name": "cherry-sync-release-env",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:DPassarelli/cherry-sync:environment:release",
  "audiences": ["api://AzureADTokenExchange"]
}'

# Second federated credential for the scheduled sweeper, which is deliberately
# NOT tied to an Environment (an Environment logs a Deployment every run, and a
# scheduled job would inflate the repo's deployment count — issue #78). Scheduled
# runs execute on the default branch, so the subject is the branch ref.
az ad app federated-credential create --id "$APP_ID" --parameters '{
  "name": "cherry-sync-sweeper-main",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:DPassarelli/cherry-sync:ref:refs/heads/main",
  "audiences": ["api://AzureADTokenExchange"]
}'

# The dedicated resource group + the stable vnet/subnet the VM attaches to (CAF-named).
# No NSG here: the firewall is created per-run and scoped to one source IP (§7.3.3).
az group create --name "$RG" --location "$REGION" \
  --tags purpose=csync-smoketest lifecycle=persistent
az network vnet create --resource-group "$RG" --location "$REGION" \
  --name "vnet-csync-smoketest-${REGION_ABBR}" \
  --subnet-name "snet-csync-smoketest-${REGION_ABBR}" \
  --tags purpose=csync-smoketest lifecycle=persistent

# Grant Contributor ON THE RESOURCE GROUP ONLY — never the subscription (§7.3.1)
az role assignment create --assignee "$APP_ID" --role Contributor \
  --scope "/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RG"

# The three identifiers GitHub needs (store as repository secrets — see below)
echo "AZURE_CLIENT_ID=$APP_ID"
echo "AZURE_TENANT_ID=$TENANT_ID"
echo "AZURE_SUBSCRIPTION_ID=$SUBSCRIPTION_ID"
```

Then, in the GitHub repo: create an Environment named `release` (the release smoketest job enters it for its stable OIDC subject; it needs no secrets of its own), and add `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` as **repository** secrets. Repository secrets are readable by every job, including the environment-free sweeper (§7.3.6, issue #78) — environment-scoped secrets would not be. They're identifiers, not cryptographic secrets (OIDC stores no client secret), so repository-level exposure is fine. Both workflows reference them as `secrets.AZURE_*`, so keeping them as secrets (rather than repository *variables*) means no reference changes.

This is the **complete** privileged setup: it runs once, by hand, in Azure Cloud Shell — which is pre-authenticated, so no Azure credential ever lands on a developer machine or in CI. Everything after this — every per-run resource — is created by the RG-scoped service principal.

**One account prerequisite: arm64 VM quota.** New/pay-as-you-go subscriptions default to a **0-core limit** for most VM families, so the first `az vm create` fails preflight with `QuotaExceeded` even though everything else is valid. Before the first real run, request a quota increase for the chosen arm64 family — `standardBpsv2Family` (the family `Standard_B2pts_v2` belongs to) — in the region, for at least the VM's vCPU count (2). Portal → Quotas → Compute, or the link in the error. Small arm64 bumps are usually auto-approved in minutes. Check current limits with `az vm list-usage --location eastus --query "[?contains(name.value, 'Bpsv2')]" -o table`.

#### 7.3.3 Provisioning script — `_scripts/azure-smoketest-vm.sh up|down`

Self-contained (no repo dependencies, like `smoketest.sh`) so a human can drive the identical provisioning by hand. It operates **inside the pre-existing `rg-csync-smoketest-<region>`** — it never creates or deletes that group (the RG-scoped principal couldn't recreate it anyway), and it creates/destroys only the ephemeral resources, named per the CAF table above. Parameterized via environment variables with sane defaults:

| Var | Purpose | Default |
|---|---|---|
| `AZ_RG` | the stable resource group to work in | `rg-csync-smoketest-${AZ_REGION_ABBR}` |
| `AZ_REGION_ABBR` | CAF region abbreviation, for resource names | `eus` |
| `AZ_LOCATION` | region | `eastus` |
| `AZ_VM_SIZE` | arm64 VM size | `Standard_B2pts_v2` |
| `AZ_IMAGE` | arm64 Ubuntu image URN | `Canonical:ubuntu-24_04-lts:server-arm64:latest` |
| `AZ_INSTANCE` | per-run instance token (name suffix) | `${GITHUB_RUN_ID:-local}` |
| `AZ_ADMIN` | admin username | `csync` |
| `AZ_SSH_PUBKEY` | path to the public key to inject | (required) |
| `AZ_SSH_SOURCE` | single source the NSG allows on port 22 (CIDR) | the caller's public IP, as a `/32` |

The ephemeral resource names are derived once from `AZ_REGION_ABBR` and `AZ_INSTANCE` (e.g. `vm-csync-smoketest-${AZ_REGION_ABBR}-${AZ_INSTANCE}`), so both `up` and `down` reconstruct the same names with no stored state.

- **`up`** creates a per-run **NSG whose only inbound rule allows TCP 22 from `AZ_SSH_SOURCE` alone** — closed to the rest of the internet — then the public IP, the NIC (on the stable subnet, with that NSG attached to the NIC), and the VM, injecting the public key and **disabling password authentication** so the ephemeral key is the only way in. Each resource is **tagged `purpose=csync-smoketest`, `lifecycle=ephemeral`, and `created=<ISO8601>`** (the sweeper keys off `lifecycle` + `created`). It waits until SSH answers, then prints `host=<public-ip>` and `user=<admin>` to stdout and `$GITHUB_OUTPUT`. When `AZ_SSH_SOURCE` is unset the script discovers it (`curl -fsS https://api.ipify.org`) and appends `/32`; the workflow lets it default this way so the allowed source is exactly the runner that will connect.
- **`down`** deletes the run's ephemeral resources by their deterministic names in dependency order — VM, then NIC, then public IP and NSG — and **leaves the resource group and the stable vnet/subnet intact**. The NIC is deleted *explicitly* (not just via the VM's `--nic-delete-option`): if `up` failed at `az vm create` — e.g. a core-quota error — the VM never existed but the NIC did, and an orphaned NIC blocks deletion of the public IP and NSG it holds. On the happy path the VM delete already reaped the NIC and OS disk via `--nic-delete-option Delete` / `--os-disk-delete-option Delete`, so the explicit NIC delete just warns "not found". Every delete is best-effort (warn and continue); the sweeper backstops anything missed.

**SSH exposure model.** The load-bearing control is key-only auth with an ephemeral keypair: the workflow generates a throwaway key per run, injects the public half, disables password auth, and the private half is gone when the job ends — so there is nothing to brute-force even for the few minutes the VM lives. The per-run `/32` NSG rule is defense-in-depth on top of that, narrowing the already-key-gated surface to the single connecting host and keeping the VM off internet scanners. The source is discovered at runtime rather than allowlisted from a static range because GitHub's hosted runners egress from a large, rotating pool of addresses that GitHub documents as unsuitable for allowlisting; the runtime lookup yields the exact `/32` every run. (This assumes the runner's outbound NAT address is the one the VM sees as the SSH source — true for Azure-hosted runners in practice.) Making the NSG an ephemeral, per-NIC resource — rather than per-run rules on a shared NSG — means cleanup rides the same `lifecycle=ephemeral` tag as everything else, with no un-taggable orphan rules left pointing at a `/32` Azure may later reassign.

#### 7.3.4 The smoketest job (separate job in `release.yml`)

Implemented as the `smoketest-linux-arm64` job in `.github/workflows/release.yml` (see there for the exact YAML, kept single-sourced rather than duplicated here). Its shape:

- `needs: build`, `runs-on: ubuntu-latest`, `environment: release` (the stable OIDC subject), and job-level `permissions: { id-token: write, contents: write }` — the OIDC token for `azure/login`, and draft-release download.
- **Steps:** checkout → `azure/login@v3` (OIDC, no stored secret) → download the `cherry-sync_*_linux_arm64` binary from the draft → *provision* → *smoketest over SSH* → *tear down*.
- The **provision** step generates an ephemeral ed25519 keypair and runs `_scripts/azure-smoketest-vm.sh up`, which publishes `host`/`user` as step outputs (via `$GITHUB_OUTPUT`). Keeping provisioning in its own step means a provisioning failure fails *there*, rather than masquerading as a later SSH/DNS error. The **smoketest** step `scp`s the binary and `smoketest.sh` to the VM (using `steps.provision.outputs.host`/`user`) and runs the smoketest over SSH.
- The **tear-down** step is separate and `if: always()`, so the VM is deleted even when the smoketest step fails (§7.3.5) — it just calls `_scripts/azure-smoketest-vm.sh down`, which reconstructs the same resource names from `GITHUB_RUN_ID`.

`promote` gates on this job alongside the matrix: `needs: [smoketest, smoketest-linux-arm64]`.

#### 7.3.5 Teardown — always delete

Tier 1 tears down unconditionally (`if: always()`): `down` deletes the run's ephemeral resources and leaves the resource group and its stable network standing. Holding a failed VM for debugging buys nothing here — a Tier 1 failure is a broken-artifact failure, reproducible without that exact VM, and the ephemeral SSH key is gone when the job ends, so a held VM isn't even reachable without resetting credentials. (Hold-on-failure may be worth revisiting for Tier 2, where a failure can be environment-dependent and the VM worth keeping.) The independent sweeper (§7.3.6) backstops the case where the `always()` step never runs at all — a cancelled or crashed job.

#### 7.3.6 Sweeper — independent backstop

A separate scheduled workflow (`.github/workflows/azure-smoketest-sweep.yml`, **twice-daily** `cron`) logs in via OIDC and deletes any resource **tagged `lifecycle=ephemeral`** whose `created` tag is older than the threshold (1h — comfortably longer than a run, so it never touches a resource from an in-flight release). The two schedules can overlap by chance (a `v*` tag pushed near the sweep's cron time), but that is safe by construction: the release smoketest job carries `timeout-minutes: 30`, so a live job's resources are always younger than the 1h threshold and a concurrent sweep skips them. Keeping the job timeout below the sweep threshold is the invariant that lets the two run uncoordinated — no mutex needed. Unlike the release smoketest job it is **not** run inside the `release` Environment: an Environment records a Deployment on every run, so a scheduled job would steadily inflate the repo's deployment count (issue #78). It instead authenticates with a federated credential scoped to the branch subject `repo:…:ref:refs/heads/main` (scheduled runs execute on the default branch — §7.3.1, §7.3.2). The cadence is twice a day, so an orphan from a crashed run lives at most about half a day before it's reclaimed — an accepted tradeoff, since orphans arise only when a release run dies mid-provision (rare) and each is a single cheap VM. Keying on `lifecycle=ephemeral` means it reaps only orphaned per-run resources (VM, disk, NIC, public IP, NSG) and never the stable RG/vnet/subnet (tagged `lifecycle=persistent`). Logic lives in `_scripts/azure-smoketest-sweep.sh` (list within the RG by type, tag, and age; delete each in dependency order), with a `--dry-run` mode to list-without-deleting so the age logic can be eyeballed by hand. This catches leaks the inline teardown can't — a job cancelled mid-provision, a runner that died. An Azure **budget alert** on the subscription is a further, out-of-band net.

#### 7.3.7 Manual drive

Every piece is hand-runnable: `az login` yourself, then `AZ_SSH_PUBKEY=~/.ssh/id.pub _scripts/azure-smoketest-vm.sh up`, `scp`/`ssh` the binary + `smoketest.sh`, and `_scripts/azure-smoketest-vm.sh down`. The workflow is a thin caller over the same scripts — no CI-only state.

### 7.4 Failure handling and manual replication

A red smoketest job (a) leaves the release unpublished as a draft, (b) on the Azure leg, leaves the VM up with printed connection details, and (c) is reproducible by hand: download the same artifact and run `./_scripts/smoketest.sh ./csync`, or stand up the VM with `./_scripts/azure-smoketest-vm.sh up` and repeat there. There is no CI-only state.

## 8. Cost and risk summary

- **Most legs have zero cloud footprint.** Three of the four (linux/amd64, darwin/arm64, and darwin/amd64 via Rosetta) run on GitHub-hosted runners — no Azure, no cloud credential — costing only GitHub Actions minutes.
- **The Azure leg is pennies per release.** One arm64 burstable VM (`Standard_B2pts_v2`) for the few minutes a run takes.
- **Flakiness is contained.** The Azure leg adds a network/provisioning dependency, but runs **only on `v*` tags** (and manual dispatch), never on `pull_request`, so it cannot make day-to-day `go test` flaky. A provisioning failure blocks a *release*, which is the correct place to absorb that risk.
- **Security.** The only new privilege is an OIDC service principal scoped **Contributor on a single dedicated resource group** (`rg-csync-smoketest-<region>`), never the subscription — so a compromised CI token can't reach any other resource group. No static cloud secret is stored. The VM accepts only an ephemeral per-run SSH key (password auth disabled), and its per-run NSG opens port 22 to just the connecting runner's `/32` (§7.3.3).

## 9. Tier 2 — forward look (not yet committed to detail)

Recorded here so the architecture is coherent and Tier 2 can be resumed without re-deriving it.

Tier 2 reuses the existing Gherkin suite rather than reimplementing assertions, because the harness (`acceptance_tests/features_test.go`) already **builds and executes a real `csync` binary** (`go build` → `exec.Command(csyncBinary, ...)`) and already centralizes remote handling in its `runCsync` step. Two seams make the suite drive a *released artifact* against a *real remote*:

1. **`CSYNC_BINARY` override** — if set, the harness drives that binary instead of building one from source. Turns "test the source" into "test the shipped artifact." One small, test-first-able change to the build step in `acceptance_tests/features_test.go`.
2. **`CSYNC_REMOTE=ssh` provider** — a real-SSH remote provider alongside today's fake-rsh one, pointing at a provisioned remote.

A curated subset of scenarios is tagged `@smoketest` (target ~3–5: one push, one pull, an identical-bytes no-op, and the non-ASCII-filename transfer, which is openrsync-sensitive). Those scenarios then run at two fidelities from one spec: hermetically on every PR (fake-rsh, today) and for-real pre-release via `CSYNC_BINARY=<artifact> CSYNC_REMOTE=ssh go test -godog.tags=@smoketest`. The same command reproduces a Tier 2 CI failure locally.

**Topology requirement and a trap to avoid.** The high-value case is openrsync (local) ↔ GNU rsync (remote), and getting it faithfully in automation has a hard constraint:

- The faithful `openrsync` is **Apple's**, which exists only on macOS. The portable Linux `openrsync` is a different, feature-incomplete fork that does not implement `--itemize-changes` / `--files-from` / `--from0`, so a Linux "openrsync" VM would represent a build `csync` can't even drive. **Do not install openrsync on a Linux VM to fake the asymmetry.**
- Therefore the openrsync *driver* must be a real Mac. The **GitHub `macos-latest` runner** is that Mac, for free. The Azure VM serves as the **GNU-rsync remote** the Mac drives against. (A Linux runner driving the same Azure remote gives the lower-value GNU↔GNU case — still real SSH and two real filesystems, but not the implementation asymmetry.)

A Tier 2 wrinkle to settle when we get there: the harness needs a Go toolchain wherever it runs, which is a question for the linux/arm64 leg specifically (install Go on the VM, or cross-compile the test binary with `go test -c` and ship it). Out of scope for Tier 1.

## 10. Validation

Tier 1 was exercised end-to-end before it was trusted. The draft → smoketest → promote machinery was proven on the `v0.2.2-rc1` dry run, which surfaced one real bug worth recording: the `promote` job had no `actions/checkout`, so `gh` couldn't infer the repo (`fatal: not a git repository`). It now names the repo explicitly — `gh release edit "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" --draft=false` — and needs no checkout, being an API-only job. The full four-leg gate, including darwin/amd64 via Rosetta and the linux/arm64 Azure leg, then ran green on the real `v0.7.0` tag: the arm64 VM provisioned, the binary passed over SSH, the VM tore down, and the release promoted as designed.

One optional tightening remains open: swapping the RG-scoped Contributor role for a custom role that omits resource-group self-delete (§7.3.1). It's gold-plating — RG-scoped Contributor is the standard least-privilege answer — and hasn't been done.
