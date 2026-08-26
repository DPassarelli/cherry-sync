#!/usr/bin/env bash
#
# azure-smoketest-vm.sh — provision / tear down the ephemeral arm64 VM that the
# Tier 1 linux/arm64 smoketest runs the released binary on.
#
# GitHub offers no native Linux/arm64 runner, so this leg stands up a throwaway
# Azure arm64 VM, runs the binary on it, and deletes it. This script handles
# ONLY the per-run (ephemeral) resources — it assumes the stable resource group
# and vnet/subnet already exist (created once by hand; see RELEASE-GATE.md)
# and never touches them. It also assumes `az` is already authenticated (the
# workflow runs azure/login first; a human runs `az login`).
#
# Usage:
#   azure-smoketest-vm.sh up      # create the VM, wait for SSH, print host/user
#   azure-smoketest-vm.sh down    # delete this run's ephemeral resources
#
# All inputs come from environment variables with defaults (see the Configuration
# block below). Resource names follow the CAF pattern described in
# RELEASE-GATE.md and are fully determined by AZ_REGION_ABBR + AZ_INSTANCE,
# so `up` and `down` agree on names with no shared state. Self-contained so it
# can be run by hand for debugging.
set -euo pipefail

# ---- Configuration (environment with defaults) -----------------------------
AZ_REGION_ABBR="${AZ_REGION_ABBR:-eus}"
AZ_LOCATION="${AZ_LOCATION:-eastus}"
AZ_RG="${AZ_RG:-rg-csync-smoketest-${AZ_REGION_ABBR}}"
AZ_VM_SIZE="${AZ_VM_SIZE:-Standard_B2pts_v2}"
AZ_IMAGE="${AZ_IMAGE:-Canonical:ubuntu-24_04-lts:server-arm64:latest}"
AZ_INSTANCE="${AZ_INSTANCE:-${GITHUB_RUN_ID:-local}}"
AZ_ADMIN="${AZ_ADMIN:-csync}"

# Stable network (created once; referenced, never created/deleted here).
vnet="vnet-csync-smoketest-${AZ_REGION_ABBR}"
subnet="snet-csync-smoketest-${AZ_REGION_ABBR}"

# Ephemeral, per-run resources (created and destroyed here).
suffix="csync-smoketest-${AZ_REGION_ABBR}-${AZ_INSTANCE}"
vm="vm-${suffix}"
osdisk="osdisk-${suffix}"
nic="nic-${suffix}"
pip="pip-${suffix}"
nsg="nsg-${suffix}"

case "${1:-}" in
  up)
    : "${AZ_SSH_PUBKEY:?AZ_SSH_PUBKEY must point to the public key to inject}"
    if [ ! -f "$AZ_SSH_PUBKEY" ]; then
      echo "azure-smoketest-vm.sh: public key not found: $AZ_SSH_PUBKEY" >&2
      exit 1
    fi

    # SSH source allow-list. Default to the caller's own public egress IP as a
    # /32, so port 22 is reachable only from the host that will connect (the
    # runner). Key-only auth is the real control; this is defense in depth.
    if [ -z "${AZ_SSH_SOURCE:-}" ]; then
      AZ_SSH_SOURCE="$(curl -fsS https://api.ipify.org)/32"
    fi

    created="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    tags=(purpose=csync-smoketest lifecycle=ephemeral "created=${created}")

    # 1. Per-run NSG: deny-by-default plus one inbound SSH rule scoped to the
    #    allowed source. Attached to the NIC below.
    az network nsg create \
      --resource-group "$AZ_RG" --location "$AZ_LOCATION" \
      --name "$nsg" --tags "${tags[@]}" --output none
    az network nsg rule create \
      --resource-group "$AZ_RG" --nsg-name "$nsg" --name allow-ssh \
      --priority 1000 --access Allow --protocol Tcp --direction Inbound \
      --source-address-prefixes "$AZ_SSH_SOURCE" --destination-port-ranges 22 \
      --output none

    # 2. Public IP.
    az network public-ip create \
      --resource-group "$AZ_RG" --location "$AZ_LOCATION" \
      --name "$pip" --sku Standard --tags "${tags[@]}" --output none

    # 3. NIC on the stable subnet, behind the per-run NSG, with the public IP.
    az network nic create \
      --resource-group "$AZ_RG" --location "$AZ_LOCATION" \
      --name "$nic" --vnet-name "$vnet" --subnet "$subnet" \
      --network-security-group "$nsg" --public-ip-address "$pip" \
      --tags "${tags[@]}" --output none

    # 4. VM on that NIC. SSH-key auth only. The OS disk and NIC are set to delete
    #    with the VM, so `down` (and the sweeper) reclaim them by deleting the VM
    #    alone; only the public IP and NSG need explicit deletion.
    az vm create \
      --resource-group "$AZ_RG" --location "$AZ_LOCATION" \
      --name "$vm" --image "$AZ_IMAGE" --size "$AZ_VM_SIZE" \
      --authentication-type ssh \
      --admin-username "$AZ_ADMIN" --ssh-key-values "$AZ_SSH_PUBKEY" \
      --nics "$nic" --nic-delete-option Delete \
      --os-disk-name "$osdisk" --os-disk-delete-option Delete \
      --tags "${tags[@]}" --output none

    host="$(az network public-ip show \
      --resource-group "$AZ_RG" --name "$pip" --query ipAddress -o tsv)"

    # Wait until SSH actually answers before handing the host back.
    echo "azure-smoketest-vm.sh: waiting for SSH on ${host}:22 ..." >&2
    ready=
    for _ in $(seq 1 30); do
      # $host is passed to the inner shell as $0 (positional), deliberately NOT
      # expanded in this outer shell — so the single quotes are intentional.
      # shellcheck disable=SC2016
      if timeout 5 bash -c 'cat < /dev/null > /dev/tcp/"$0"/22' "$host" 2>/dev/null; then
        ready=1
        break
      fi
      sleep 5
    done
    if [ -z "$ready" ]; then
      echo "azure-smoketest-vm.sh: SSH never came up on ${host}:22" >&2
      exit 1
    fi

    # Connection details for the caller: stdout always, plus $GITHUB_OUTPUT when
    # run as its own workflow step.
    echo "host=${host}"
    echo "user=${AZ_ADMIN}"
    if [ -n "${GITHUB_OUTPUT:-}" ]; then
      {
        echo "host=${host}"
        echo "user=${AZ_ADMIN}"
      } >> "$GITHUB_OUTPUT"
    fi
    ;;

  down)
    # Best-effort teardown: warn and continue so one missing resource can't
    # strand the others, and so an `if: always()` teardown step never fails the
    # job. Anything missed here is caught by the sweeper within the hour.
    #
    # Delete in dependency order: VM, then NIC, then the public IP and NSG the
    # NIC referenced. The NIC is deleted EXPLICITLY rather than relying on the
    # VM's --nic-delete-option: if `up` failed at `az vm create` (e.g. a quota
    # error), the VM never existed but the NIC did, and an orphaned NIC blocks
    # deletion of the public IP and NSG it holds. On the happy path the VM
    # delete already reaped the NIC (and OS disk) via delete-options, so this NIC
    # delete simply warns "not found" and moves on.
    warn_delete() {
      "$@" || echo "azure-smoketest-vm.sh: warning: '$*' failed (continuing)" >&2
    }
    warn_delete az vm delete --resource-group "$AZ_RG" --name "$vm" --yes
    warn_delete az network nic delete --resource-group "$AZ_RG" --name "$nic"
    warn_delete az network public-ip delete --resource-group "$AZ_RG" --name "$pip"
    warn_delete az network nsg delete --resource-group "$AZ_RG" --name "$nsg"
    ;;

  *)
    echo "usage: azure-smoketest-vm.sh up|down" >&2
    exit 2
    ;;
esac
