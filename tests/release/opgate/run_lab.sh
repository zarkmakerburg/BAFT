#!/usr/bin/env bash
# Runs the pinned operational-ingress-v1 manifest tests inside a disposable
# network namespace and writes test2json output to <log>. It never decides the
# verdict: tests/release/opgate does. Missing packages/tests simply produce no
# events, which the checker reports as MISSING (gate red).
#
#   tests/release/opgate/run_lab.sh <manifest.json> <log>
#
# Safety: a private netns is the ONLY isolation provided here. This script does
# NOT create /baft-policy-lab or any other LAB sentinel on the host, and Linux
# UID 0 is never treated as an isolated root filesystem. Tests that need a
# scratch-container rootfs (installed policy provisioning) must not be listed
# for this runner.
set -uo pipefail
manifest=${1:?usage: run_lab.sh <manifest.json> <log>}
log=${2:?usage: run_lab.sh <manifest.json> <log>}
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)
cd "$repo_root" || exit 2
: >"$log"
if [[ -n "${BAFT_ISOLATED_POLICY_LAB:-}" ]]; then
  echo "refusing to run with BAFT_ISOLATED_POLICY_LAB set on the host" >&2
  exit 2
fi
work=$(mktemp -d)
ns="baft-opv1-$$-${GITHUB_RUN_ID:-local}"
cleanup() { sudo ip netns del "$ns" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
sudo ip netns add "$ns" || { echo "cannot create private netns; no tests run" >&2; exit 2; }
sudo ip netns exec "$ns" ip link set lo up

go_cache=$(go env GOCACHE)
go_path=$(go env GOPATH)
go_root=$(go env GOROOT)
go_bin=$(dirname "$(command -v go)")
echo "lab: go=$(go version) GOROOT=$go_root GOCACHE=$go_cache GOPATH=$go_path"
echo "lab: pid1=$(cat /proc/1/comm) host-netns=$(readlink /proc/1/ns/net) nft=$(command -v nft) ip=$(command -v ip)"
# The tests run with GOPROXY=off inside the netns (no network there) and build
# ./cmd/baft and ./cmd/baft-pair themselves, so every module must already be in
# the module cache. Download them here, outside the netns.
go mod download || { echo "go mod download failed" >&2; exit 2; }
# Read-only diagnostic: why the product's systemd socket inventory accepts or
# rejects this host's unit roots (changes nothing).
go run ./tests/e2e/operational/labdiag 2>&1 | head -40 || true
grep -rH '^ListenStream' /usr/lib/systemd/system /etc/systemd/system 2>/dev/null | sed 's|.*/||' | sort -u | head -60 || true
rc=0
while IFS= read -r pkg; do
  tests=$(jq -r --arg p "$pkg" '[.entries[]|select(.package==$p)|.test]|join("|")' "$manifest")
  bin="$work/$(echo "$pkg" | tr '/.' '__').test"
  echo "== $pkg"
  if ! go test -c -o "$bin" "$pkg"; then
    echo "RED: $pkg does not build or does not exist (its required tests are MISSING)" >&2
    rc=1
    continue
  fi
  (
    cd "$pkg" && sudo ip netns exec "$ns" env PATH="$go_bin:$PATH" HOME="$HOME" GOROOT="$go_root" GOTOOLCHAIN=local GOCACHE="$go_cache" \
      GOPATH="$go_path" GOPROXY=off BAFT_OPERATIONAL_INGRESS_V1=1 \
      "$bin" -test.run "^(${tests})\$" -test.v -test.count=1 -test.timeout=20m
  ) 2>&1 | go tool test2json -p "$pkg" >>"$log"
  [[ ${PIPESTATUS[0]} -eq 0 ]] || rc=1
done < <(jq -r '[.entries[].package]|unique|.[]' "$manifest")
# Concise diagnostics (the checker, not this summary, decides the verdict).
echo "=== lab summary ==="
jq -r 'select(.Test!=null and (.Test|contains("/")|not) and (.Action=="pass" or .Action=="fail" or .Action=="skip")) | "\(.Action|ascii_upcase) \(.Package) \(.Test)"' "$log" | sort | uniq -c | sort -rn | head -80
jq -r 'select(.Test==null and .Action=="output") | "\(.Package): \(.Output)"' "$log" | grep -v '^[^:]*: *$' | head -60
jq -r 'select(.Test!=null and .Action=="output" and (.Output|test("^\\s+\\S+\\.go:[0-9]+:|FAIL|panic"))) | "\(.Package) \(.Test): \(.Output)"' "$log" | head -80
exit "$rc"
