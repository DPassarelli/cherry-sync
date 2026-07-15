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
# arguments, csync writes an error to stderr that points the user at
# "csync --help", and exits 2.
# Invoked with --version it writes the project version line to stdout and exits
# 0; when the caller names the expected version (the release path passes the
# tag), that line must contain it — this is the only check that exercises the
# -ldflags version injection, whose failure the no-argument path can't see.
# Invoked with --license it writes the embedded MIT notice to stdout and exits 0.
#
# Usage:
#   smoketest.sh <path-to-csync-binary> [expected-version]
#
# Exit status: 0 if the binary passes every check; 2 for misuse of this script;
# 1 for a smoketest failure. The script is deliberately self-contained — it
# depends on nothing in the repository, so it can be copied to a bare host and
# run there (Phase 2 scp's it onto an Azure VM).
set -euo pipefail

# Argument handling. All input is untrusted: require the binary operand, accept
# an optional expected-version operand, and confirm the binary names an
# executable file before running it.
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "usage: smoketest.sh <path-to-csync-binary> [expected-version]" >&2
  exit 2
fi
bin="$1"
expected_version="${2:-}"
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
vout="$(mktemp)"
verr="$(mktemp)"
lout="$(mktemp)"
lerr="$(mktemp)"
trap 'rm -f "$out" "$err" "$vout" "$verr" "$lout" "$lerr"' EXIT

rc=0
"$bin" >"$out" 2>"$err" </dev/null || rc=$?

vrc=0
"$bin" --version >"$vout" 2>"$verr" </dev/null || vrc=$?

lrc=0
"$bin" --license >"$lout" 2>"$lerr" </dev/null || lrc=$?

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

# 2. The error must point the user at --help on stderr. csync no longer dumps a
# bare "usage:" block for a bad invocation (#91); it prints a one-line reason and
# a pointer to `csync --help`, which every usage error ends with, so that pointer
# is the stable signal to assert here.
if ! grep -qF -- '--help' "$err"; then
  check_fail "expected the error on stderr to point at 'csync --help'; stderr was: $(cat "$err")"
fi

# 3. --version must exit 0 and print the project version line to stdout.
if [ "$vrc" -ne 0 ]; then
  check_fail "--version: expected exit code 0, got $vrc"
fi
if ! grep -qF 'cherry-sync' "$vout"; then
  check_fail "--version: expected a line containing 'cherry-sync' on stdout; stdout was: $(cat "$vout")"
fi

# 4. When the caller states the expected release version, --version's output
# must contain it verbatim. This is the check that catches a broken injection: a
# binary built without the tag renders "(dev build)" and fails here. It is
# skipped for local hand-runs against a dev build, which pass no version.
if [ -n "$expected_version" ] && ! grep -qF "$expected_version" "$vout"; then
  check_fail "--version: expected output to contain '$expected_version'; stdout was: $(cat "$vout")"
fi

# 5. --license must exit 0 and print the copyright notice to stdout. The expected
# strings are literals rather than a comparison against the repository's LICENSE:
# this script is copied to a bare host with nothing beside it, and exact equality
# with the root LICENSE is already pinned by internal/license's unit test. What
# only this check can see is whether the notice survived into the *published*
# artifact — a shipped binary that cannot print its license is a compliance
# failure, not merely a bug, and one that cannot be taken back after download.
if [ "$lrc" -ne 0 ]; then
  check_fail "--license: expected exit code 0, got $lrc"
fi
if ! grep -qF 'MIT License' "$lout"; then
  check_fail "--license: expected the copyright notice ('MIT License') on stdout; stdout was: $(cat "$lout")"
fi

# 6. The permission notice must be present too, not just the title. MIT requires
# both the copyright and the permission notice accompany every copy, so assert a
# line from the warranty paragraph that ends the text — a truncated or partially
# embedded license passes check 5 but fails here.
if ! grep -qF 'THE SOFTWARE IS PROVIDED' "$lout"; then
  check_fail "--license: expected the permission notice ('THE SOFTWARE IS PROVIDED') on stdout; stdout was: $(cat "$lout")"
fi

if [ "$failures" -ne 0 ]; then
  echo "SMOKETEST FAIL ($failures check(s) failed): $bin" >&2
  exit 1
fi
echo "SMOKETEST PASS: $bin"
