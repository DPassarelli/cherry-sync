#!/usr/bin/env bash
#
# azure-smoketest-sweep.sh — independent backstop that reclaims orphaned
# ephemeral smoketest resources.
#
# The inline teardown (azure-smoketest-vm.sh down) is the primary cleanup; this
# script catches what it can't — a job cancelled mid-provision, a runner that
# died before `down` ran. It deletes every resource in the stable resource group
# tagged `lifecycle=ephemeral` whose `created` tag is older than a threshold.
# Stable resources are tagged `lifecycle=persistent` and are never touched.
# Assumes `az` is authenticated (the workflow runs azure/login; a human runs
# `az login`).
#
# Usage:
#   azure-smoketest-sweep.sh            # delete orphans older than the threshold
#   azure-smoketest-sweep.sh --dry-run  # list what would be deleted, delete nothing
#
# Config via environment (defaults shown):
#   AZ_REGION_ABBR=eus
#   AZ_RG=rg-csync-smoketest-${AZ_REGION_ABBR}
#   AZ_SWEEP_THRESHOLD_SECONDS=3600     # 1 hour
set -euo pipefail

AZ_REGION_ABBR="${AZ_REGION_ABBR:-eus}"
AZ_RG="${AZ_RG:-rg-csync-smoketest-${AZ_REGION_ABBR}}"
AZ_SWEEP_THRESHOLD_SECONDS="${AZ_SWEEP_THRESHOLD_SECONDS:-3600}"

dry_run=
case "${1:-}" in
  "")          ;;
  --dry-run)   dry_run=1 ;;
  *)           echo "usage: azure-smoketest-sweep.sh [--dry-run]" >&2; exit 2 ;;
esac

now="$(date -u +%s)"

# Delete in dependency order: a VM before its NIC, a NIC before the public IP
# and NSG it references, a disk after its VM. Deleting the VM reclaims its NIC
# and disk (delete-options set at create time), so the later types are mostly
# mopping up partial-provision orphans. Anything that fails (transient ordering)
# is retried on the next sweep.
types=(
  Microsoft.Compute/virtualMachines
  Microsoft.Network/networkInterfaces
  Microsoft.Network/publicIPAddresses
  Microsoft.Network/networkSecurityGroups
  Microsoft.Compute/disks
)

deleted=0
for type in "${types[@]}"; do
  while IFS=$'\t' read -r id created; do
    [ -n "$id" ] || continue
    if [ -z "$created" ] || [ "$created" = "None" ]; then
      echo "sweep: skipping (no created tag): $id" >&2
      continue
    fi
    created_epoch="$(date -u -d "$created" +%s 2>/dev/null || echo 0)"
    if [ "$created_epoch" -eq 0 ]; then
      echo "sweep: skipping (unparseable created='$created'): $id" >&2
      continue
    fi
    age=$(( now - created_epoch ))
    if [ "$age" -lt "$AZ_SWEEP_THRESHOLD_SECONDS" ]; then
      continue
    fi
    if [ -n "$dry_run" ]; then
      echo "sweep: WOULD delete (age ${age}s): $id"
      continue
    fi
    echo "sweep: deleting (age ${age}s): $id"
    if az resource delete --ids "$id" --output none; then
      deleted=$(( deleted + 1 ))
    else
      echo "sweep: warning: delete failed (will retry next sweep): $id" >&2
    fi
  done < <(
    az resource list \
      --resource-group "$AZ_RG" \
      --resource-type "$type" \
      --query "[?tags.lifecycle=='ephemeral'].[id, tags.created]" \
      --output tsv
  )
done

if [ -n "$dry_run" ]; then
  echo "sweep: dry run complete"
else
  echo "sweep: deleted ${deleted} resource(s)"
fi
