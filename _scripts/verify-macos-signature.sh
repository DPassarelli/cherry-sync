#!/usr/bin/env bash
#
# verify-macos-signature.sh — pre-publish signing check for a released macOS csync binary.
#
# Proves a published darwin artifact carries a real Developer ID signature, so a
# release can never silently ship a binary Gatekeeper refuses to run for want of
# one (#93). This complements smoketest.sh, which proves the binary executes but
# is blind to how it was signed: the Go linker ad-hoc-signs every arm64 Mach-O, so
# a wholly unsigned build still runs on the runner AND still passes
# `codesign --verify`. The team identifier is what separates the two — a signed
# build reports this team, an ad-hoc one reports "not set".
#
# Notarization is deliberately not asserted here, and cannot be: a bare Mach-O
# cannot be stapled, so the only local evidence of a ticket is codesign's
# `=notarized` requirement, which consults the machine's ticket cache rather than
# Apple. It passes only where that ticket has already been fetched, so it fails on
# an ephemeral CI runner however well notarized the binary is (proven on the
# v1.0.0 release: both runners rejected a binary Apple had accepted minutes
# earlier, which then satisfied the same requirement on a Mac that had evaluated
# it once). `spctl --assess` is no substitute — it only evaluates app bundles and
# rejects any bare executable whatever its signature ("the code is valid but does
# not seem to be an app"). Notarization is gated where the evidence is firsthand:
# GoReleaser waits on Apple's verdict at build time and fails the release unless
# it comes back Accepted. Every check below is local, so this needs no network.
#
# Usage:
#   verify-macos-signature.sh <path-to-csync-binary>
#
# Exit status: 0 if the binary passes every check; 2 for misuse of this script;
# 1 for a verification failure. macOS only — codesign ships nowhere else.
set -euo pipefail

# The Developer ID team the published binaries must be signed by. Pinned to this
# one team rather than accepting any Developer ID, so the check still fails if
# signing ever falls through to a different identity. A fork publishing under its
# own certificate changes this line.
readonly EXPECTED_TEAM_ID="KGPQDZ2Q7U"

# Argument handling. All input is untrusted: require the binary operand and
# confirm it names a real file before handing it to codesign.
if [ "$#" -ne 1 ]; then
  echo "usage: verify-macos-signature.sh <path-to-csync-binary>" >&2
  exit 2
fi
bin="$1"
if [ ! -f "$bin" ]; then
  echo "verify-macos-signature.sh: binary not found: $bin" >&2
  exit 1
fi

# Run every codesign invocation up front, capturing status without letting
# `set -e` abort on an (informative) non-zero exit. codesign writes its display
# output to stderr, hence the redirect on the -d call.
verify_out="$(mktemp)"
display_out="$(mktemp)"
trap 'rm -f "$verify_out" "$display_out"' EXIT

verify_rc=0
codesign --verify --strict --verbose=2 "$bin" >"$verify_out" 2>&1 || verify_rc=$?

display_rc=0
codesign --display --verbose=2 "$bin" >"$display_out" 2>&1 || display_rc=$?

# Assertions. Each failed check increments `failures` and prints why, so a run
# reports everything that is wrong rather than stopping at the first problem.
failures=0
check_fail() {
  echo "  FAIL: $1" >&2
  failures=$((failures + 1))
}

# 1. The signature must be structurally intact. This alone proves very little —
# an ad-hoc linker signature passes it — but a failure here means the binary was
# modified after signing, which the later checks would report more obscurely.
if [ "$verify_rc" -ne 0 ]; then
  check_fail "codesign --verify failed (exit $verify_rc): $(cat "$verify_out")"
fi

# 2. The signing identity must be a Developer ID Application certificate, not a
# development or ad-hoc one. Only a Developer ID signature is accepted by
# Gatekeeper on a machine that has never seen the developer's certificate.
if ! grep -qF 'Authority=Developer ID Application' "$display_out"; then
  check_fail "expected a 'Developer ID Application' authority; codesign reported: $(cat "$display_out")"
fi

# 3. The team identifier must be ours. This is the check that actually catches a
# release built without the signing secrets: such a build is ad-hoc signed by the
# Go linker and reports "TeamIdentifier=not set", which passes checks 1 and 2's
# structural intent but is exactly the binary #93 was filed about.
if ! grep -qF "TeamIdentifier=${EXPECTED_TEAM_ID}" "$display_out"; then
  check_fail "expected TeamIdentifier=${EXPECTED_TEAM_ID}; codesign reported: $(cat "$display_out")"
fi

if [ "$failures" -ne 0 ]; then
  echo "SIGNATURE VERIFY FAIL ($failures check(s) failed): $bin" >&2
  exit 1
fi
echo "SIGNATURE VERIFY PASS: $bin"
