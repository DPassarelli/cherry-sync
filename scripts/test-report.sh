#!/usr/bin/env bash
#
# test-report.sh — run the test suite via gotestsum and render the compact
# report that CI publishes.
#
# This is the single source of truth for that report, so the local preview and
# the CI job summary can't drift. When $GITHUB_STEP_SUMMARY is set (i.e. in
# GitHub Actions), the report is appended there and gotestsum streams to the
# Actions log; run locally, the identical report is printed to the terminal so
# you can see exactly what CI will show — raw Markdown (pipe to a renderer such
# as `glow` if you want it formatted).
#
# REPORT_LABEL names the panel; CI passes the matrix leg name, local defaults to
# "local". Any extra arguments are forwarded to `go test` (e.g. -run TestFoo).
#
# Exit status is gotestsum's, so a failed run still emits its report and then
# fails the step — the CI gate is never masked by the report.
set -euo pipefail

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
log="$(mktemp)"
trap 'rm -f "$log"' EXIT

# pkgname format is marker-free (no ::group:: workflow commands), so the same
# text serves both a live stream and the rendered report. Capture gotestsum's
# exit code rather than letting the pipe abort under `set -e`.
status=0
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  # CI: stream to the Actions log, capture a copy for the job summary.
  "$gotestsum" --format pkgname -- ./... "$@" 2>&1 | tee "$log" || status=$?
  sink="$GITHUB_STEP_SUMMARY"
else
  # Local: capture quietly, then print the report once (no duplicate stream).
  "$gotestsum" --format pkgname -- ./... "$@" >"$log" 2>&1 || status=$?
  sink=/dev/stdout
fi

{
  printf '## Test results — %s\n\n' "$label"
  printf '```\n'
  cat "$log"
  printf '```\n'
} >>"$sink"

exit "$status"
