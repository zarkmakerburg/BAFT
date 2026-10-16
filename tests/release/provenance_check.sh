#!/usr/bin/env bash
# Test independent asset provenance, including rejection of modified assets.
# Uses no production signing material and never signs anything.
set -euo pipefail
umask 077
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$repo_root"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
version=v0.9.8-provenance-test
commit=$(git rev-parse HEAD)
candidate="$work/candidate"
mkdir -p "$candidate/dist"
go_version=$(awk '$1 == "go" { print $2; exit }' go.mod)
env GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN="go$go_version" \
  bash scripts/release/build.sh "$version" "$candidate/dist" >/dev/null
cp install.sh "$candidate/install.sh"
cp release/keys/root.pub "$candidate/root.pub"
cp release/keys/revocations.json "$candidate/revocations.json"
printf 'tag=%s\ncommit=%s\nsigned=false\nsigning_boundary=external-offline\n' "$version" "$commit" > "$candidate/CANDIDATE"

bash scripts/release/verify_candidate_provenance.sh "$candidate" "$version" "$commit"

reject() {
  local label=$1 needle=$2 output
  shift 2
  if output=$("$@" 2>&1); then
    echo "FAIL: $label unexpectedly accepted" >&2
    exit 1
  fi
  if ! grep -Fq "$needle" <<<"$output"; then
    echo "FAIL: $label rejected for unexpected reason: $output" >&2
    exit 1
  fi
  echo "PASS rejected: $label"
}

reject "wrong approved SHA" "checkout is not approved commit" \
  bash scripts/release/verify_candidate_provenance.sh "$candidate" "$version" "0000000000000000000000000000000000000000"
cp "$candidate/root.pub" "$work/original-root"
printf 'different\n' > "$candidate/root.pub"
reject "candidate untrusted root" "candidate root mismatches approved source" \
  bash scripts/release/verify_candidate_provenance.sh "$candidate" "$version" "$commit"
cp "$work/original-root" "$candidate/root.pub"

cp "$candidate/install.sh" "$work/original-installer"
printf 'different\n' > "$candidate/install.sh"
reject "candidate installer changed" "candidate installer mismatches approved source" \
  bash scripts/release/verify_candidate_provenance.sh "$candidate" "$version" "$commit"
cp "$work/original-installer" "$candidate/install.sh"

printf 'x' >> "$candidate/dist/baft-linux-amd64"
reject "tampered downloaded binary" "artifact source/provenance mismatch: baft-linux-amd64" \
  bash scripts/release/verify_candidate_provenance.sh "$candidate" "$version" "$commit"

echo "PROVENANCE GATE TESTS PASSED"
