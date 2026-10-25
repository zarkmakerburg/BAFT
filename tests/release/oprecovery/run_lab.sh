#!/usr/bin/env bash
# Runs the restart-recovery LAB tests (tests/e2e/operational/recovery) and
# writes test2json output to <log>. The verdict is verdict.sh, not this script.
#
#   tests/release/oprecovery/run_lab.sh <log>
#
# Isolation: the orchestrating test runs in a PRIVATE MOUNT namespace
# (unshare -m; /etc and the unit directory are tmpfs copies, see oplab) and
# creates one private NETWORK namespace per test with `ip netns add` (deleted
# by the test; the same precedent as tests/release/opgate/run_lab.sh). Nothing
# is installed on, written to or provisioned on the host: no LAB sentinel, no
# host nft table, no host policy file. UID 0 is never treated as isolation.
set -uo pipefail
log=${1:?usage: run_lab.sh <log>}
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)
cd "$repo_root" || exit 2
: >"$log"
if [[ -n "${BAFT_ISOLATED_POLICY_LAB:-}" ]]; then
  echo "refusing to run with BAFT_ISOLATED_POLICY_LAB set on the host" >&2
  exit 2
fi
pkg=./tests/e2e/operational/recovery
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
go_cache=$(go env GOCACHE); go_path=$(go env GOPATH); go_root=$(go env GOROOT); go_bin=$(dirname "$(command -v go)")
echo "lab: go=$(go version) pid1=$(cat /proc/1/comm) nft=$(command -v nft) ip=$(command -v ip) unshare=$(command -v unshare)"
go mod download || { echo "go mod download failed" >&2; exit 2; }
go test -c -o "$work/recovery.test" "$pkg" || { echo "RED: $pkg does not build" >&2; exit 1; }
rc=0
(
  cd tests/e2e/operational/recovery && sudo unshare -m --propagation private \
    env PATH="$go_bin:$PATH" HOME="$HOME" GOROOT="$go_root" GOTOOLCHAIN=local GOCACHE="$go_cache" GOPATH="$go_path" GOPROXY=off \
    BAFT_OPERATIONAL_RECOVERY=1 "$work/recovery.test" -test.v -test.count=1 -test.timeout=25m
) 2>&1 | go tool test2json -p "$pkg" >>"$log"
[[ ${PIPESTATUS[0]} -eq 0 ]] || rc=1
jq -r 'select(.Test!=null and .Action=="output" and (.Output|test("FAIL|panic|recovery:|lab "))) | "\(.Test): \(.Output)"' "$log" | cut -c1-400 | head -80
exit "$rc"
