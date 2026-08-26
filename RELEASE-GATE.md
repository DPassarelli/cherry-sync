# Release Gate

Every published `csync` binary passes through a pre-publish gate: GoReleaser creates the GitHub Release as an invisible draft, each artifact is downloaded from that draft and checked on its target OS/arch, and the release is flipped live only when every check is green. A failure leaves the draft unpublished.

This document holds the reasoning, the trust boundaries, and the two runbooks that are not (and cannot be) code. It does not restate what the workflows and scripts already say. Where a mechanism has an owning file, this document points at it instead of copying it, because copies drift.

## 1. Why the gate exists

The pipeline used to build and publish binaries without ever running them. A bad cross-compile, a wrong-architecture artifact, a broken `init`, or a failed version injection would have been found by a user rather than by us.

"Sound" has two tiers with very different costs:

- **Tier 1, does it execute.** Run each artifact on its target OS/arch and assert it starts, runs its own Go code, and honors its documented argument contract. No remote, no network transfer. Built, and gating every release.
- **Tier 2, does it actually sync.** Drive each artifact through real transfers against a real remote `rsync` over real SSH. Not built. See §8.

macOS code signing was added later as a second, independent gate on the two darwin artifacts. It is not a smoke test, but it belongs to the same "do not publish a broken artifact" job.

## 2. Where the implementation lives

| Concern | Owning file |
|---|---|
| Build, signing, notarization, draft flag, asset names | `.goreleaser.yaml` |
| Gate wiring: draft, fan-out, promote | `.github/workflows/release.yml` |
| Tier 1 assertions (the argument contract) | `_scripts/smoketest.sh` |
| macOS signature assertions | `_scripts/verify-macos-signature.sh` |
| Ephemeral arm64 VM lifecycle | `_scripts/azure-smoketest-vm.sh` |
| Orphan reclamation | `.github/workflows/azure-smoketest-sweep.yml`, `_scripts/azure-smoketest-sweep.sh` |

Each of those files carries a header comment explaining what it asserts and why. Read the file, not a summary of it.

## 3. What the gate proves, and what it cannot

**Proved by Tier 1.** The artifact executes on its target architecture and honors its argument contract, including that the release tag was injected into the version string. The version assertion is what catches a broken `-ldflags -X` injection, which no other check can see.

**Proved by the signing gate.** The published darwin artifacts carry a real Developer ID signature from the expected team. This is not redundant with Tier 1: the Go linker ad-hoc-signs every arm64 Mach-O, so a wholly unsigned build still executes on the runner and still satisfies `codesign --verify`. The team identifier is the only thing that separates the two.

**Not provable on a runner: notarization.** A bare Mach-O cannot be stapled, so the only local evidence of a notarization ticket is `codesign`'s `=notarized` requirement, which consults the machine's ticket cache rather than Apple. It fails on an ephemeral runner however well notarized the binary is. This was demonstrated on the v1.0.0 release, where both runners rejected a binary Apple had accepted minutes earlier and which then satisfied the same requirement on a Mac that had evaluated it once. Notarization is therefore gated where the evidence is firsthand: GoReleaser blocks on Apple's verdict at build time and fails the release unless it comes back Accepted.

**Not covered at all: transfer behavior.** Tier 1 never invokes `compare` or `transfer`. See §8.

## 4. Decisions

**Draft, then promote.** GoReleaser publishes the release as a draft with assets attached. The checks run against that draft's assets. A final job flips `--draft=false` only if every check job succeeded. This gates on the exact bytes a user would download, including the upload step, rather than on GoReleaser's local `dist/` output.

**Scripts are the unit of work; the workflow is a thin caller.** Anything CI does must be reproducible by hand, so a failure can be investigated without a CI-only environment.

**Signing is checked separately and before the smoke test,** so a signing fault is reported as a signing fault. It is guarded by the same secret that switches signing on in `.goreleaser.yaml`, so a fork holding no Apple credentials builds unsigned and skips the check rather than failing a release it deliberately built that way. Only the emptiness of the secret crosses into the job, never its value.

**linux/arm64 needs an external host.** GitHub offers no native Linux/arm64 runner. The other three legs run on GitHub-hosted runners, with darwin/amd64 executing under Rosetta 2 on the Apple-silicon runner. linux/arm64 uses an ephemeral Azure arm64 VM, the only cloud dependency in the gate.

**Release assets lose their executable bit.** A release asset is an opaque blob, so whatever runs a downloaded binary must `chmod +x` it. The GitHub-runner legs do this after download; the Azure leg does it on the VM instead, because `scp` does not carry the mode across.

## 5. The Azure leg

Used only to *execute* the binary. The VM needs no Go toolchain and no rsync peer.

### 5.1 Least privilege

There is no spare subscription for this work, so a role assignment at subscription scope would let a compromised CI token reach unrelated resource groups. The scope is instead a single dedicated resource group, with the principal granted **Contributor on that group only**. Contributor excludes `Microsoft.Authorization/*/write`, so the principal cannot broaden its own scope or hand out role assignments.

It can delete its own resource group, which is still inside the blast-radius guarantee. Teardown deliberately deletes only the ephemeral resources and leaves the group standing. Denying even that would need a custom role omitting `Microsoft.Resources/subscriptions/resourceGroups/delete`, which is gold-plating over the standard answer.

No client secret is stored anywhere. GitHub Actions mints a short-lived OIDC token that `azure/login` exchanges for an Azure access token.

### 5.2 OIDC subject stability

An Azure federated credential matches an exact OIDC `subject` claim. For a tag-triggered run the subject is `repo:OWNER/REPO:ref:refs/tags/<tag>`, different for every tag, so a per-tag credential is unworkable. Running the job inside a GitHub Environment named `release` makes the subject the stable `repo:OWNER/REPO:environment:release`.

That environment carries a deployment branch policy restricted to the tag pattern `v*`, so the stable subject is reachable only from a release tag and not from an arbitrary branch. The release workflow triggers only on `v*` tags, so this blocks nothing it does.

The **sweeper deliberately does not use an Environment.** Each Environment run logs a Deployment, and a twice-daily scheduled job would inflate the repository's deployment count (#78). It has no per-tag problem anyway, since scheduled runs execute on the default branch, so it authenticates through a second federated credential trusting the branch subject `repo:OWNER/REPO:ref:refs/heads/main`.

### 5.3 SSH exposure

The load-bearing control is key-only auth with an ephemeral keypair. The workflow generates a throwaway key per run, injects the public half, disables password authentication, and the private half is gone when the job ends. There is nothing to brute-force even for the few minutes the VM lives.

The per-run NSG rule opening port 22 to a single `/32` is defense in depth on top of that, keeping the VM off internet scanners. The source is discovered at runtime rather than allowlisted, because GitHub documents its hosted-runner egress pool as unsuitable for allowlisting. This assumes the runner's outbound NAT address is what the VM sees as the SSH source, which holds for Azure-hosted runners in practice.

### 5.4 Resource naming and lifecycle

Names follow the Microsoft Cloud Adoption Framework pattern `<type>-<workload>-<purpose>-<region>[-<instance>]`, with workload `csync`, purpose `smoketest`, and the CAF region abbreviation. The AD app registration and service principal are not ARM resources, so they keep their given name, `cherry-sync-test`.

Resources split by lifecycle because the RG-scoped principal cannot create resource groups:

- **Stable**, created once by hand (§6) and never touched by the script: resource group, virtual network, subnet. Tagged `lifecycle=persistent`.
- **Ephemeral**, created and destroyed per run: VM, OS disk, NIC, public IP, NSG. Tagged `lifecycle=ephemeral` plus a `created` timestamp, and suffixed with the GitHub run ID so concurrent runs never collide. A local hand-run uses the instance token `local`.

Every ephemeral name is derived from the region abbreviation and the instance token, so `up` and `down` reconstruct identical names with no stored state. Teardown runs unconditionally (`if: always()`). Holding a failed VM for debugging buys nothing, since a Tier 1 failure is a broken-artifact failure reproducible elsewhere, and the ephemeral key is gone when the job ends.

### 5.5 The invariant that lets the sweeper run uncoordinated

The sweeper reclaims anything tagged `lifecycle=ephemeral` whose `created` tag is older than its threshold, catching orphans from a cancelled or crashed job that never ran its teardown step. It keys on the tag, so it never touches the stable group or network.

**The release job's `timeout-minutes` must stay below the sweeper's age threshold.** A live job's resources are then always younger than the threshold, and a sweep that happens to run concurrently skips them. This coupling spans two files and is the reason no mutex is needed. Change either number and check the other.

An Azure budget alert on the subscription is a further out-of-band net.

## 6. One-time setup, run by hand

This is the complete privileged setup. It runs once, in Azure Cloud Shell, which is pre-authenticated, so no Azure credential ever lands on a developer machine or in CI. Everything after this is created by the RG-scoped service principal.

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
# NOT tied to an Environment (§5.2). Scheduled runs execute on the default
# branch, so the subject is the branch ref.
az ad app federated-credential create --id "$APP_ID" --parameters '{
  "name": "cherry-sync-sweeper-main",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:DPassarelli/cherry-sync:ref:refs/heads/main",
  "audiences": ["api://AzureADTokenExchange"]
}'

# The dedicated resource group + the stable vnet/subnet the VM attaches to.
# No NSG here: the firewall is created per-run and scoped to one source IP (§5.3).
az group create --name "$RG" --location "$REGION" \
  --tags purpose=csync-smoketest lifecycle=persistent
az network vnet create --resource-group "$RG" --location "$REGION" \
  --name "vnet-csync-smoketest-${REGION_ABBR}" \
  --subnet-name "snet-csync-smoketest-${REGION_ABBR}" \
  --tags purpose=csync-smoketest lifecycle=persistent

# Grant Contributor ON THE RESOURCE GROUP ONLY, never the subscription (§5.1)
az role assignment create --assignee "$APP_ID" --role Contributor \
  --scope "/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RG"

# The three identifiers GitHub needs
echo "AZURE_CLIENT_ID=$APP_ID"
echo "AZURE_TENANT_ID=$TENANT_ID"
echo "AZURE_SUBSCRIPTION_ID=$SUBSCRIPTION_ID"
```

Then, on the GitHub side:

```sh
# Create the `release` Environment, then restrict it to release tags (§5.2)
gh api --method PUT repos/DPassarelli/cherry-sync/environments/release --input - <<'JSON'
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
JSON

gh api --method POST \
  repos/DPassarelli/cherry-sync/environments/release/deployment-branch-policies \
  --input - <<'JSON'
{"name":"v*","type":"tag"}
JSON
```

Add `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, and `AZURE_SUBSCRIPTION_ID` as **repository** secrets, not environment secrets: repository secrets are readable by every job including the environment-free sweeper (§5.2), and these are identifiers rather than cryptographic material, since OIDC stores no client secret.

The Apple signing credentials (`MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_ISSUER_ID`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_KEY`) are also repository secrets. `MACOS_SIGN_P12` doubles as the on/off switch for both signing and its verification, so a fork or a secretless run builds unsigned instead of failing.

**Account prerequisite: arm64 VM quota.** New and pay-as-you-go subscriptions default to a 0-core limit for most VM families, so the first `az vm create` fails preflight with `QuotaExceeded` even when everything else is valid. Request a quota increase for the arm64 family in the chosen region for at least the VM's vCPU count, via Portal > Quotas > Compute. Small arm64 bumps are usually auto-approved in minutes. Check current limits with `az vm list-usage --location eastus -o table`.

## 7. Driving it by hand

Every piece is hand-runnable, which is the point of the script-first split.

```sh
# Tier 1 assertions against any binary, including a local dev build
./_scripts/smoketest.sh ./csync

# ...and with the release-version check, as the release legs run it
./_scripts/smoketest.sh ./csync v1.2.3

# macOS signing check (macOS only)
./_scripts/verify-macos-signature.sh ./csync

# The arm64 leg, end to end
az login
AZ_SSH_PUBKEY=~/.ssh/id.pub _scripts/azure-smoketest-vm.sh up
# scp the binary and smoketest.sh to the printed host, run it there
_scripts/azure-smoketest-vm.sh down

# List what the sweeper would reclaim, without deleting
_scripts/azure-smoketest-sweep.sh --dry-run
```

To reproduce a CI failure, download the same asset from the still-unpublished draft release and run the same script against it. There is no CI-only state.

## 8. Tier 2, the unbuilt half

Recorded so it can be resumed without re-deriving it.

**Why it matters.** The real deployment shape is a macOS laptop running Apple's `openrsync` against a Linux box running GNU `rsync`. The two ends run different rsync implementations, and the one thing `csync` uniquely does, parsing `--itemize-changes` and driving `--files-from`, is sensitive to which implementation produced that output. GNU emits an 11-character itemized code and Apple's `openrsync` a 9-character one. The parser is written to be width-agnostic for exactly this reason, but the asymmetry has only ever been checked by hand on a developer's Mac.

The hermetic suite cannot close this gap. It simulates a remote by pointing `RSYNC_RSH` at a fake shell that strips the host and execs locally, so the identical rsync binary runs on both ends.

**The seams.** The acceptance harness already builds and executes a real `csync` binary and centralizes remote handling in one step definition, so two changes would make it drive a released artifact against a real remote: a `CSYNC_BINARY` override to skip the build, and a `CSYNC_REMOTE=ssh` provider alongside today's fake one. A curated subset of scenarios tagged `@smoketest` (one push, one pull, an identical-bytes no-op, and the non-ASCII filename transfer, which is openrsync-sensitive) would then run at two fidelities from one spec.

**A trap to avoid.** The faithful `openrsync` is Apple's, which exists only on macOS. The portable Linux `openrsync` is a different, feature-incomplete fork with no `--itemize-changes`, `--files-from`, or `--from0`, so a Linux "openrsync" VM would represent a build `csync` cannot even drive. Do not install openrsync on a Linux VM to fake the asymmetry. The openrsync driver must be a real Mac, and the `macos-latest` runner is that Mac for free, with an Azure VM as the GNU-rsync remote it drives against.

**An open question.** The harness needs a Go toolchain wherever it runs, which is unresolved for the arm64 leg: either install Go on the VM, or cross-compile the test binary with `go test -c` and ship it.
