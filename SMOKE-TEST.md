# Smoke Testing for Binaries Before Publishing — Design Spec

Status: **Tier 1** (artifact-execution gate) is fully built — Phase 1 (both native legs) and the darwin/amd64 (Rosetta) leg are implemented and dry-run-validated through promote; the linux/arm64 (Azure) leg is now implemented too (§7.3) and awaits its first real tag-triggered run, which needs the one-time Azure setup in §7.3.2 completed under the `csync-smoketest` names. **Tier 2** (real-transfer gate) remains a sketch (§9) so the architecture is coherent end-to-end, committed to detail in a later pass.

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

A `checksums.txt` accompanies them. The smoketest downloads these published assets (see §7), so it exercises the exact bytes a user would receive — including the upload step, not just the local `dist/` output.

## 5. Locked decisions

These were settled in design discussion and are treated as fixed for this spec:

1. **Both tiers, built one at a time, Tier 1 first.**
2. **linux/arm64 is in scope.** GitHub provides no native Linux/arm64 runner, so its execution host is an ephemeral Azure arm64 VM (§6, §8). This is the only cloud dependency in Tier 1.
3. **Gating mechanism is draft-then-promote** (§7): GoReleaser publishes the release as a *draft* (invisible to users); smoketest jobs run against the draft's assets; the release is flipped to non-draft only if every smoketest job is green.
4. **Trigger is the `v*` tag** (pre-publish, in `release.yml`), and the same work must be **drivable by hand** so a CI failure can be reproduced and investigated. This forces a script-first architecture (§7).
5. **Tier 1 ships in two phases**, split along execution-host difficulty:
   - **Phase 1 — native GitHub runners only:** linux/amd64 (`ubuntu-latest`) and darwin/arm64 (`macos-latest`). No Azure, no Rosetta, no new cloud credential. This phase proves the full draft → smoketest → promote machinery and `_scripts/smoketest.sh` on the two hosts that run for free.
   - **Phase 2 — the harder hosts:** darwin/amd64 (Rosetta on `macos-latest`) and linux/arm64 (ephemeral Azure arm64 VM). This phase adds only execution hosts to an already-proven gate.

## 6. Tier 1 — definition and execution-host matrix

### 6.1 What Tier 1 asserts

The "documented argument contract" Tier 1 checks is the one `cmd/csync/main.go` implements today: invoked with no (or invalid) arguments, `csync` prints `usage: csync SOURCE DESTINATION` to **stderr** and exits **2**. Tier 1 asserts:

- the binary executes on its target OS/arch (it loads and runs Go code rather than failing to start), and
- with no arguments it exits `2` and writes a line containing `usage:` to stderr.

That is sufficient to catch a broken artifact. It deliberately does **not** invoke `compare`/`transfer` (those need rsync and a peer — Tier 2).

> Contract dependency: if a `--version` flag is added later (currently deferred until in-binary version injection exists), Tier 1 should also assert `csync --version` prints a non-empty version and exits `0`. Until then the no-argument usage path is the surface the smoketest exercises. Whoever adds `--version` updates this assertion in lockstep.

### 6.2 Execution hosts

The artifact under test is the *local* csync. For Tier 1 it only needs a host that can execute its arch — no remote.

| Phase | Artifact | Execution host | How it runs |
|---|---|---|---|
| 1 | linux/amd64 | `ubuntu-latest` GitHub runner | native |
| 1 | darwin/arm64 | `macos-latest` GitHub runner (Apple silicon) | native |
| 2 | darwin/amd64 | `macos-latest` GitHub runner | via Rosetta 2 — macOS runs the x86_64 binary transparently on exec once Rosetta is installed; no `arch` prefix needed. The leg runs `softwareupdate --install-rosetta --agree-to-license` first (a no-op if already present). |
| 2 | linux/arm64 | **ephemeral Azure arm64 VM** | `scp` the binary, run over SSH |

**Phase 1** covers the two artifacts that run natively on GitHub-hosted runners — no Azure, no Rosetta, no cloud credential. **Phase 2** adds the two harder hosts: darwin/amd64 (Rosetta) and linux/arm64 (the only artifact needing Azure, and only to *execute* the binary — no Go toolchain, no rsync peer). The Phase 2 cloud footprint is **one short-lived arm64 VM, alive for minutes.**

**Implementation status:** all four legs are implemented in `release.yml`. Phase 1 (both native legs) and darwin/amd64 (Rosetta) are dry-run-validated green. The linux/arm64 (Azure) leg (`smoketest-linux-arm64` job + `_scripts/azure-smoketest-vm.sh`) is implemented and awaits its first real tag-triggered run, which requires the one-time Azure setup in §7.3.2 done under the `csync-smoketest` names.

## 7. Tier 1 — architecture (scripts + workflow)

Mirrors the pattern already used for the test-report dashboard: **the script is the unit of work; the workflow is a thin caller**, so what CI runs is exactly what a human can run by hand. Nothing the gate does is reproducible only inside Actions.

### 7.1 Scripts (the source of truth)

- **`_scripts/smoketest.sh <path-to-csync-binary>`** — the assertion runner. Executes the given binary with no arguments and asserts the §6.1 contract (exit `2`, `usage:` on stderr). Prints a clear pass/fail line and exits non-zero on failure. Host-agnostic: it does not care whether the binary arrived via download, `scp`, or a local build. This is the entire Tier 1 behavior; a human runs `./_scripts/smoketest.sh ./csync` directly.

  *Teeth requirement (house testing rule):* `smoketest.sh` must be verified against both a known-good binary (real `csync` → green) and a degenerate stand-in that violates the contract (e.g. `/bin/true`, which exits `0` and prints nothing → the script must go red). Confirm the failure is for the right reason before trusting the gate.

- **`_scripts/azure-smoketest-vm.sh up|down`** — provisions / tears down the ephemeral arm64 VM and prints connection details (public IP, SSH user). Parameterized (resource-group name, region, VM size, SSH key path) so a human can stand the same environment up locally for debugging and tear it down when done.

### 7.2 Workflow wiring (`release.yml`)

The release job is split so the smoketest gate sits between build and publish:

1. **Build (draft).** Set `release.draft: true` in `.goreleaser.yaml`. GoReleaser builds all four artifacts and creates the GitHub Release as a **draft** with assets attached — invisible to users. The existing release-notes extraction and pre-release-flag enforcement stay here, applied to the draft.
2. **Smoketest (fan-out).** One job per execution host in §6.2. Each downloads its artifact from the draft release (`gh release download <tag> --pattern 'cherry-sync_*_<os>_<arch>.tar.gz'`), unpacks the `csync` binary, and runs `_scripts/smoketest.sh` against it — directly on `ubuntu-latest`/`macos-latest`, or (Phase 2 only) by `scp`-then-SSH onto the Azure arm64 VM (`_scripts/azure-smoketest-vm.sh up`). In Phase 1 this is two jobs, both on native GitHub runners.
3. **Promote (gated).** A final job that `needs` every smoketest job. On success it flips the release live: `gh release edit <tag> --draft=false` (folded into the existing `gh release edit` enforcement step). If any smoketest job failed, the release stays a draft and the workflow fails.

Downloading from the draft release (rather than reusing GoReleaser's local `dist/`) means the bytes the smoketest runs are the bytes that promotion reveals — the upload itself is covered.

### 7.3 Phase 2 leg: linux/arm64 via an ephemeral Azure VM

Phase 1 and the darwin/amd64 (Rosetta) leg have no Azure dependency. This section is the full design for the one remaining leg, which needs an external host because GitHub offers no native Linux/arm64 runner. The leg does the same thing every other leg does — execute the artifact and assert the §6.1 contract — just on a VM we stand up and tear down per run. It needs **no Go toolchain and no rsync peer** on the VM; it only runs the binary.

Because it needs an OIDC token, a GitHub Environment, and SSH — none of which the GitHub-runner legs need — it is a **separate job**, not another `matrix` leg under `smoketest`.

**Resource naming (Microsoft Cloud Adoption Framework).** Every Azure resource this leg creates follows the CAF pattern `<type>-<workload>-<purpose>-<region>[-<instance>]`, using the CAF resource-type abbreviations. The workload token is `csync` and the purpose token is `smoketest`; `<region>` is the CAF region abbreviation for the chosen region (e.g. `eus` for `eastus`, pending the §10 region decision). The AD app registration and service principal aren't ARM resources, so they're out of scope for this convention and keep their given name, `cherry-sync-test`.

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

**The subject-stability problem.** An Azure *federated credential* matches an exact OIDC `subject` claim. For a tag-triggered run the subject is `repo:OWNER/REPO:ref:refs/tags/<tag>` — different for every tag, so a per-tag credential is unworkable. The fix is to run the Azure job inside a GitHub **Environment** (e.g. `release`): the subject then becomes the stable `repo:OWNER/REPO:environment:release`, constant across tags. The job declares `environment: release`. (A side benefit: Environments can require a reviewer, giving an optional manual approval gate before any cloud spend.)

**Least privilege — a dedicated resource group, not the subscription.** There is no spare subscription set aside for this work; the subscription also holds hand-built and other-project resource groups. A role assignment at *subscription* scope would let a compromised CI token reach all of them, so the scope is instead a **single dedicated resource group**, `rg-csync-smoketest-<region>`, created once by hand (§7.3.2). The principal is **Contributor scoped to that resource group only** — enough to create and delete the ephemeral VM, disk, NIC, public IP, and per-run NSG inside it, and structurally incapable of touching any other resource group.

Two properties of this boundary are worth stating:
- **Contributor can't escalate itself.** The Contributor role excludes `Microsoft.Authorization/*/write`, so the principal can't broaden its own scope or hand out role assignments. The resource-group wall holds.
- **It *can* delete its own resource group** (deleting an RG is a Contributor action and the assignment is on that RG) — still within the blast-radius guarantee, since that's only *this* RG. Teardown (§7.3.5) deliberately deletes the ephemeral resources and leaves `rg-csync-smoketest-<region>` standing as the stable container. If even self-deletion of the group must be denied, swap Contributor for a custom role omitting `Microsoft.Resources/subscriptions/resourceGroups/delete`; that's gold-plating, and RG-scoped Contributor is the standard answer.

#### 7.3.2 One-time setup (run by a human, once)

```sh
SUBSCRIPTION_ID=$(az account show --query id -o tsv)
TENANT_ID=$(az account show --query tenantId -o tsv)

REGION=eastus          # the §10 region decision …
REGION_ABBR=eus        # … and its CAF abbreviation, used in every resource name
RG=rg-csync-smoketest-${REGION_ABBR}

# App registration + service principal (not an ARM resource; keeps the name cherry-sync-test)
az ad app create --display-name cherry-sync-test
APP_ID=$(az ad app list --display-name cherry-sync-test --query "[0].appId" -o tsv)
az ad sp create --id "$APP_ID"

# Federated credential trusting this repo's `release` environment (stable subject)
az ad app federated-credential create --id "$APP_ID" --parameters '{
  "name": "cherry-sync-release-env",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:DPassarelli/cherry-sync:environment:release",
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

# The three identifiers GitHub needs (store as secrets on the `release` environment)
echo "AZURE_CLIENT_ID=$APP_ID"
echo "AZURE_TENANT_ID=$TENANT_ID"
echo "AZURE_SUBSCRIPTION_ID=$SUBSCRIPTION_ID"
```

Then, in the GitHub repo: create an Environment named `release` and add `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` as environment secrets. (They're identifiers, not cryptographic secrets, but storing them on the environment keeps them with the federation that trusts it.)

This is the **complete** privileged setup: it runs once, by hand, in Azure Cloud Shell — which is pre-authenticated, so no Azure credential ever lands on a developer machine or in CI. Everything after this — every per-run resource — is created by the RG-scoped service principal.

**One account prerequisite: arm64 VM quota.** New/pay-as-you-go subscriptions default to a **0-core limit** for most VM families, so the first `az vm create` fails preflight with `QuotaExceeded` even though everything else is valid. Before the first real run, request a quota increase for the chosen arm64 family — `standardBpsv2Family` (the family `Standard_B2pts_v2` belongs to) — in the region, for at least the VM's vCPU count (2). Portal → Quotas → Compute, or the link in the error. Small arm64 bumps are usually auto-approved in minutes. Check current limits with `az vm list-usage --location eastus --query "[?contains(name.value, 'Bpsv2')]" -o table`.

#### 7.3.3 Provisioning script — `_scripts/azure-smoketest-vm.sh up|down`

Self-contained (no repo dependencies, like `smoketest.sh`) so a human can drive the identical provisioning by hand. It operates **inside the pre-existing `rg-csync-smoketest-<region>`** — it never creates or deletes that group (the RG-scoped principal couldn't recreate it anyway), and it creates/destroys only the ephemeral resources, named per the CAF table above. Parameterized via environment variables with sane defaults:

| Var | Purpose | Default |
|---|---|---|
| `AZ_RG` | the stable resource group to work in | `rg-csync-smoketest-${AZ_REGION_ABBR}` |
| `AZ_REGION_ABBR` | CAF region abbreviation, for resource names | (decision — §10) |
| `AZ_LOCATION` | region | (decision — §10) |
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
- **Steps:** checkout → `azure/login@v2` (OIDC, no stored secret) → download + unpack the `cherry-sync_*_linux_arm64.tar.gz` artifact from the draft → *provision* → *smoketest over SSH* → *tear down*.
- The **provision** step generates an ephemeral ed25519 keypair and runs `_scripts/azure-smoketest-vm.sh up`, which publishes `host`/`user` as step outputs (via `$GITHUB_OUTPUT`). Keeping provisioning in its own step means a provisioning failure fails *there*, rather than masquerading as a later SSH/DNS error. The **smoketest** step `scp`s the binary and `smoketest.sh` to the VM (using `steps.provision.outputs.host`/`user`) and runs the smoketest over SSH.
- The **tear-down** step is separate and `if: always()`, so the VM is deleted even when the smoketest step fails (§7.3.5) — it just calls `_scripts/azure-smoketest-vm.sh down`, which reconstructs the same resource names from `GITHUB_RUN_ID`.

`promote` gates on this job alongside the matrix: `needs: [smoketest, smoketest-linux-arm64]`.

#### 7.3.5 Teardown — for Tier 1, always delete

The earlier draft of this section proposed *holding the VM on failure* for debugging. On reflection that belongs to **Tier 2**, not Tier 1, for two reasons:

1. **Tier 1 failures aren't environment-specific.** The check is "does the arm64 binary run and print usage." A failure is a broken-artifact failure, reproducible without that exact VM — so there's little to debug *on the VM*.
2. **The SSH key is ephemeral.** It's generated in the job and gone when the job ends, so a held VM isn't even reachable without resetting credentials via `az vm user update` — friction that buys nothing for a Tier-1-class failure.

So Tier 1 tears down unconditionally (`if: always()`) — `down` deletes the run's ephemeral resources and leaves `rg-csync-smoketest-<region>` and its stable network standing. Hold-on-failure is reconsidered for Tier 2, where a transfer failure can be environment-dependent and the VM is worth keeping. The independent sweeper (next) still backstops the case where the `always()` step never runs at all (a cancelled or crashed job).

#### 7.3.6 Sweeper — independent backstop

A separate scheduled workflow (`.github/workflows/azure-smoketest-sweep.yml`, **hourly** `cron`) logs in with the same OIDC identity and deletes any resource **tagged `lifecycle=ephemeral`** whose `created` tag is older than the threshold (1h — comfortably longer than a run, short enough to bound cost). The cadence is hourly rather than daily on purpose: a 1h age threshold only bounds cost if the sweep runs about that often, otherwise an orphan from a crashed run could live until the next daily pass. Keying on `lifecycle=ephemeral` means it reaps only orphaned per-run resources (VM, disk, NIC, public IP, NSG) and never the stable RG/vnet/subnet (tagged `lifecycle=persistent`). Logic lives in `_scripts/azure-smoketest-sweep.sh` (list within the RG by type, tag, and age; delete each in dependency order), with a `--dry-run` mode to list-without-deleting so the age logic can be eyeballed by hand. This catches leaks the inline teardown can't — a job cancelled mid-provision, a runner that died. An Azure **budget alert** on the subscription is a further, out-of-band net.

#### 7.3.7 Manual drive

Every piece is hand-runnable: `az login` yourself, then `AZ_SSH_PUBKEY=~/.ssh/id.pub _scripts/azure-smoketest-vm.sh up`, `scp`/`ssh` the binary + `smoketest.sh`, and `_scripts/azure-smoketest-vm.sh down`. The workflow is a thin caller over the same scripts — no CI-only state.

### 7.4 Failure handling and manual replication

A red smoketest job (a) leaves the release unpublished as a draft, (b) on the Azure leg, leaves the VM up with printed connection details, and (c) is reproducible by hand: download the same artifact and run `./_scripts/smoketest.sh ./csync`, or stand up the VM with `./_scripts/azure-smoketest-vm.sh up` and repeat there. There is no CI-only state.

## 8. Tier 1 — cost and risk summary

- **Phase 1 — zero cloud footprint.** Both legs run on GitHub-hosted runners. No Azure account, no OIDC credential, no teardown, no Rosetta dependency. Cost is GitHub Actions minutes only.
- **Cost (Phase 2).** One B-/D-series arm64 VM for the minutes a run takes — pennies per release. Everything else still runs on GitHub-hosted runners.
- **Flakiness (Phase 2).** The Azure leg adds a network/provisioning dependency. It runs **only on `v*` tags** (and manual dispatch), never on `pull_request`, so it cannot make day-to-day `go test` flaky. A provisioning failure blocks a *release*, which is the correct place to absorb that risk.
- **Security (Phase 2).** The only new privilege is an OIDC service principal scoped **Contributor on a single dedicated resource group** (`rg-csync-smoketest-<region>`), never the subscription — so a compromised CI token can't reach any other resource group. No static cloud secret is stored. The smoketest VM accepts only an ephemeral per-run SSH key (password auth disabled) and its per-run NSG opens port 22 to just the connecting runner's `/32` (§7.3.3).

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

## 10. Open questions

Phase 1 — **resolved by the v0.2.2-rc1 dry run:**

- **`gh release download` from a draft** — works with `contents: write`; the pattern `cherry-sync_*_<os>_<arch>.tar.gz` resolves correctly.
- **Promote step** — the failure mode found was unrelated to idempotency: the `promote` job had no `actions/checkout`, so `gh` couldn't infer the repo (`fatal: not a git repository`). Fixed by naming it explicitly: `gh release edit "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" --draft=false` (no checkout needed for an API-only job). A second rc confirmed promote then publishes correctly.

Phase 2 — darwin/amd64:

- **Rosetta on `macos-latest`:** the leg is implemented with an unconditional `softwareupdate --install-rosetta` step; confirm on a real rc run that it goes green (whether Rosetta is preinstalled or the install step suffices).

Phase 2 — linux/arm64 (Azure), all resolved; the leg is built (§7.3). Recorded here for provenance:

- **Region** (`AZ_LOCATION` + `AZ_REGION_ABBR`) — *resolved:* `eastus`/`eus`. The stable RG and vnet/subnet are already provisioned there.
- **VM size** (`AZ_VM_SIZE`) — *resolved:* `Standard_B2pts_v2` (cheap arm64 burstable).
- **Image** (`AZ_IMAGE`) — *resolved:* `Canonical:ubuntu-24_04-lts:server-arm64:latest`. Verify the URN resolves in `eastus` (`az vm image show --urn …`) before the first run; `:latest` floats to the newest patch, which is fine for a throwaway VM.
- **Role scope** — *resolved:* Contributor scoped to a single dedicated resource group, `rg-csync-smoketest-<region>` (no spare subscription exists; RG scope is the least-privilege boundary — §7.3.1). A custom role omitting RG self-delete remains an optional tightening.
- **Environment** — *resolved:* use the `release` GitHub Environment (required for the stable federated subject `repo:…:environment:release`), **no required reviewer**. The federation subject restriction + RG-scoped Contributor + 1h sweeper already bound the risk, so a manual approval gate would only add friction to an otherwise-unattended tag-triggered release. (A reviewer can be added later if a manual "approve the spend" checkpoint is ever wanted.) Still to confirm on the first real run: that the subject authenticates on a *tag-triggered* job running in the environment.
- **Sweeper threshold** — *resolved:* 1h (comfortably longer than a run, short enough to bound cost).
