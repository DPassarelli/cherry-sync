#!/usr/bin/env bash
#
# smoketest.sh — Tier 1 pre-publish smoketest for a single csync binary.
#
# Proves a built artifact is SOUND in the minimal sense: it executes on this
# host's OS/arch and honors its documented no-argument contract. It does NOT
# perform a real transfer (that is Tier 2), so no rsync peer or network is
# needed and this script runs anywhere the binary can — a GitHub runner, a
# provisioned VM, or a developer's laptop.
#
# Contract checked (as implemented in cmd/csync/main.go): invoked with no
# arguments, csync writes a line containing "usage:" to stderr and exits 2.
#
# Usage:
#   smoketest.sh <path-to-csync-binary>
#
# Exit status: 0 if the binary passes every check; 2 for misuse of this script;
# 1 for a smoketest failure. The script is deliberately self-contained — it
# depends on nothing in the repository, so it can be copied to a bare host and
# run there (Phase 2 scp's it onto an Azure VM).
set -euo pipefail

# Argument handling. All input is untrusted: require exactly one operand and
# confirm it names an executable file before running it.
if [ "$#" -ne 1 ]; then
  echo "usage: smoketest.sh <path-to-csync-binary>" >&2
  exit 2
fi
bin="$1"
if [ ! -f "$bin" ]; then
  echo "smoketest.sh: binary not found: $bin" >&2
  exit 1
fi
if [ ! -x "$bin" ]; then
  echo "smoketest.sh: binary is not executable: $bin" >&2
  exit 1
fi

# Run the binary under test. Capture stdout and stderr to separate files and the
# exit code without letting `set -e` abort on the (expected) non-zero exit. Feed
# /dev/null on stdin so the binary can never block on the interactive selection
# prompt, whatever arguments it ends up parsing.
out="$(mktemp)"
err="$(mktemp)"
trap 'rm -f "$out" "$err"' EXIT

rc=0
"$bin" >"$out" 2>"$err" </dev/null || rc=$?

# Assertions. Each failed check increments `failures` and prints why, so a run
# reports everything that's wrong rather than stopping at the first problem.
failures=0
check_fail() {
  echo "  FAIL: $1" >&2
  failures=$((failures + 1))
}

# 1. The no-argument invocation must exit 2.
if [ "$rc" -ne 2 ]; then
  check_fail "expected exit code 2, got $rc"
fi

# 2. The usage line must be written to stderr.
if ! grep -qF 'usage:' "$err"; then
  check_fail "expected a line containing 'usage:' on stderr; stderr was: $(cat "$err")"
fi

if [ "$failures" -ne 0 ]; then
  echo "SMOKETEST FAIL ($failures check(s) failed): $bin" >&2
  exit 1
fi
echo "SMOKETEST PASS: $bin"
