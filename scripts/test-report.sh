#!/usr/bin/env bash
#
# test-report.sh — run the test suite via gotestsum and render a compact
# dashboard panel: the environment (rsync/Go/runner), a Gherkin-vs-unit
# breakdown, any failures, and per-package detail.
#
# This is the single source of truth for that panel, so the local preview and
# the CI job summary can't drift. gotestsum streams its live per-test output to
# stdout (the GitHub Actions log in CI, your terminal locally); the panel is
# built from gotestsum's --jsonfile by ./cmd/testreport and written to
# $GITHUB_STEP_SUMMARY when set, otherwise printed to stdout. Run locally you
# see exactly what CI publishes — raw Markdown (pipe to a renderer such as
# `glow` if you want it formatted).
#
# REPORT_LABEL names the panel; CI passes the matrix leg name, local defaults to
# "local". Any extra arguments are forwarded to `go test` (e.g. -run TestFoo).
#
# Exit status is gotestsum's, so a failed run still emits its panel and then
# fails the step — the CI gate is never masked by the report.
set -euo pipefail

# Run from the repository root so ./... and ./cmd/testreport resolve regardless
# of where the script is invoked from.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Resolve gotestsum. Prefer PATH; otherwise fall back to where `go install`
# drops it (GOBIN, else GOPATH/bin) — that's exactly what the hint below tells
# you to run, and `go install` does not put that directory on your PATH.
gotestsum=gotestsum
if ! command -v gotestsum >/dev/null 2>&1; then
  gobin=""
  if command -v go >/dev/null 2>&1; then
    gobin="$(go env GOBIN)"
    [ -n "$gobin" ] || gobin="$(go env GOPATH)/bin"
  fi
  if [ -n "$gobin" ] && [ -x "$gobin/gotestsum" ]; then
    gotestsum="$gobin/gotestsum"
  else
    echo "gotestsum not found on PATH or in your Go bin dir. Install it with:" >&2
    echo "  go install gotest.tools/gotestsum@v1.13.0" >&2
    echo "(and consider adding \"\$(go env GOPATH)/bin\" to your PATH)." >&2
    exit 127
  fi
fi

label="${REPORT_LABEL:-local}"
json="$(mktemp)"
trap 'rm -f "$json"' EXIT

# Environment facts the test run itself can't know — the rsync implementation is
# the whole point of the CI matrix, so surface it in the panel rather than the
# log. RUNNER_OS/ARCH are set by GitHub Actions; fall back to uname locally.
rsync_id=""
if command -v rsync >/dev/null 2>&1; then
  # Read the full version output and take its first line by expansion — piping to
  # `head` would SIGPIPE rsync, and `pipefail` would abort the script on that.
  rsync_ver="$(rsync --version 2>&1)"
  rsync_id="$(command -v rsync) — ${rsync_ver%%$'\n'*}"
fi
go_id="$(go version 2>/dev/null | awk '{print $3}')"
if [ -n "${RUNNER_OS:-}" ]; then
  runner="${RUNNER_OS} ${RUNNER_ARCH:-}"
else
  runner="$(uname -srm)"
fi

# Stream live output to stdout (Actions log / terminal) and capture the JSON the
# panel is built from. Capture gotestsum's exit code instead of letting `set -e`
# abort on a test failure.
status=0
"$gotestsum" --format pkgname --jsonfile "$json" -- ./... "$@" || status=$?

sink="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
go run ./cmd/testreport \
  --label "$label" \
  --rsync "$rsync_id" \
  --go "$go_id" \
  --runner "$runner" \
  <"$json" >>"$sink"

exit "$status"
