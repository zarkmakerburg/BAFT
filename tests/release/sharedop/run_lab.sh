#!/usr/bin/env bash
# Supplementary shared-listener acceptance. Each exact test gets a fresh netns;
# Agent TestMain supplies the private mount namespace and disposable policy.
# Never provision a policy, sentinel or nft rule in the host namespaces.
set -euo pipefail
umask 022
out=${1:?usage: run_lab.sh <evidence-directory>}
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)
cd "$repo_root"
mkdir -p "$(dirname "$out")"
# Reusing old verdicts after a setup/build failure must never look green.
mkdir "$out"
out=$(cd "$out" && pwd -P)
if [[ -n "${BAFT_ISOLATED_POLICY_LAB:-}" || -n "${BAFT_OPV1_MNT_CHILD:-}" ]]; then
  echo 'refusing inherited LAB isolation claims' >&2
  exit 2
fi
head=$(git rev-parse HEAD)
test "$head" = "${EXPECT_SHA:-$head}"
if [[ "${SHAREDOP_REQUIRE_CLEAN:-0}" == 1 ]]; then
  test -z "$(git status --porcelain)" || { echo 'checkout must be clean' >&2; exit 2; }
fi
if [[ $EUID -eq 0 ]]; then root=(); else root=(sudo -n); fi
work=$(mktemp -d)
ns=''
cleanup() {
  if [[ -n "$ns" ]]; then "${root[@]}" ip netns del "$ns" >/dev/null 2>&1 || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
python3 tests/release/sharedop/evidence.py snapshot "$out/source-before.json"
go mod download
CGO_ENABLED=0 go build -trimpath -o "$work/baft" ./cmd/baft
CGO_ENABLED=0 go build -trimpath -o "$work/baft-pair" ./cmd/baft-pair
CGO_ENABLED=0 go test -c -o "$work/agent.test" ./internal/agent
chmod 755 "$work" "$work/baft" "$work/baft-pair" "$work/agent.test"
sha256sum "$work/baft" "$work/baft-pair" "$work/agent.test" >"$out/binaries.sha256"
go version >"$out/go-version.txt"
rc=0
while IFS= read -r name; do
  ns="baft-sharedop-$$"
  if ! "${root[@]}" ip netns add "$ns"; then
    echo 'cannot create private network namespace' >&2
    exit 2
  fi
  "${root[@]}" ip netns exec "$ns" ip link set lo up
  printf '%s\n' "private-netns=$ns test=$name" >>"$out/namespaces.txt"
  # Binary builds and module access happen above, outside the isolated netns.
  # There must be no supervising process left in this netns: the product's
  # installed inventory deliberately requires its test process to be alone.
  set +e
  (
    cd internal/agent
    "${root[@]}" ip netns exec "$ns" env \
      BAFT_OPERATIONAL_INGRESS_V1=1 BAFT_AGENT_TEST_SKIP_FLOW_BINARIES=1 \
      BAFT_BIN="$work/baft" BAFT_PAIR_BIN="$work/baft-pair" \
      GOPROXY=off GOTOOLCHAIN=local \
      "$work/agent.test" -test.run "^${name}\$" -test.v -test.count=1 -test.timeout=10m
  ) >"$out/$name.log" 2>&1
  test_rc=$?
  set -e
  printf '%s\n' "$test_rc" >"$out/$name.exit"
  if ! go tool test2json -p ./internal/agent <"$out/$name.log" >"$out/$name.json"; then rc=1; fi
  if [[ $test_rc -ne 0 ]]; then rc=1; fi
  "${root[@]}" ip netns del "$ns"
  ns=''
done < <(python3 tests/release/sharedop/verdict.py --list-tests)
python3 tests/release/sharedop/evidence.py snapshot "$out/source-after.json"
python3 tests/release/sharedop/evidence.py seal "$out"
if ! python3 tests/release/sharedop/verdict.py "$out"; then rc=1; fi
exit "$rc"
