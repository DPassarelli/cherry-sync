#!/usr/bin/env bash
#
# gosec.sh — run gosec static security analysis over the module.
#
# This is the single source of truth for the gosec version and invocation, so
# the pre-push hook and the CI job can't drift. Pinned via `go run @version` so
# the gate is reproducible without a separate global install (the first run
# downloads and builds gosec, then it's cached). `-quiet` prints only on
# findings.
set -euo pipefail

# Run from the repository root so ./... resolves regardless of where the script
# is invoked from.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

exec go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 -quiet ./...
