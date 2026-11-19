#!/usr/bin/env bash
#
# verify.sh — every gate a change must clear before it is done, selectable by
# intent so the git hooks and each CI job run the same definition of a check
# instead of their own copies of its command.
#
# Checks keep running after one fails, so a single run reports every problem.
#
# shellcheck disable=SC2329 # check_* functions are called by constructed name.
set -uo pipefail

# Run from the repository root so ./... resolves regardless of where the script
# is invoked from.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" || exit 1

# all_checks is also the order checks run and appear in the summary.
all_checks="test format lint security manifest"

# usage prints the help text.
usage() {
  cat <<'EOF'
Usage: verify.sh [--test] [--format] [--lint] [--security] [--manifest]

Runs the selected checks (all of them when none is given), keeps going past
failures, then prints a summary. Exits 1 if any check failed.

  --test      behavior is correct (the full test suite)
  --format    code matches the canonical layout
  --lint      code avoids suspicious or non-idiomatic constructs
  --security  code and dependencies have no known security problems
  --manifest  declared dependencies match what the code uses
  -h, --help  print this help
EOF
}

# check_test runs the full suite.
#
# -count=1 is load-bearing. The godog suite builds the csync binary in TestMain
# and drives it by exec, importing no production package. Go's test cache keys
# a result on the package's own sources and its imports, so no production-code
# change invalidates the godog result: without -count=1, a change confined to
# cmd/ or internal/ can report a green `ok (cached)` from a stale build. CI's
# dashboard test run passes the same flag for the same reason.
check_test() {
  go test -count=1 ./...
}

# check_format fails on any file gofmt would rewrite. gofmt -l exits 0 even
# when it lists files, so the listing itself is the failure.
check_format() {
  local unformatted
  unformatted="$(gofmt -l .)" || return 1
  [ -z "$unformatted" ] && return 0
  echo "Not gofmt-clean (run gofmt -w on these):" >&2
  echo "$unformatted" >&2
  return 1
}

# check_lint runs the static checks for suspicious constructs.
check_lint() {
  go vet ./...
}

# check_security runs static security analysis at its pinned version.
check_security() {
  ./_scripts/gosec.sh
}

# check_manifest fails when go.mod or go.sum differ from what `go mod tidy`
# would write.
check_manifest() {
  go mod tidy -diff
}

for arg in "$@"; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
  esac
done

selected=""
for arg in "$@"; do
  case "$arg" in
    --test|--format|--lint|--security|--manifest) selected="$selected ${arg#--}" ;;
    *)
      echo "verify.sh: unknown option '$arg'" >&2
      usage >&2
      exit 2
      ;;
  esac
done
[ -n "$selected" ] || selected="$all_checks"

summary=""
failed=0
for check in $all_checks; do
  case " $selected " in
    *" $check "*) ;;
    *) continue ;;
  esac
  echo "==> $check"
  if "check_$check"; then
    summary="${summary}PASS  ${check}"$'\n'
  else
    summary="${summary}FAIL  ${check}"$'\n'
    failed=1
  fi
done

printf '\n%s' "$summary"
exit "$failed"
