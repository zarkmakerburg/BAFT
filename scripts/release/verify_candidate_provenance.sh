#!/usr/bin/env bash
# Fail closed unless an unsigned candidate matches a pristine approved Git
# commit rebuilt locally with the Go toolchain pinned in that commit.
# Usage: verify_candidate_provenance.sh CANDIDATE_DIR vX.Y.Z FULL_COMMIT_SHA
set -euo pipefail
umask 077

[[ $# -eq 3 ]] || { echo "usage: $0 CANDIDATE_DIR vX.Y.Z FULL_COMMIT_SHA" >&2; exit 2; }
candidate=$1
version=$2
commit=$3
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "invalid version" >&2; exit 2; }
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "full commit SHA required" >&2; exit 2; }
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$commit" ]] || { echo "checkout is not approved commit" >&2; exit 1; }
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] || {
  echo "checkout contains tracked or untracked modifications" >&2; exit 1;
}
[[ -d "$candidate" && -d "$candidate/dist" && ! -L "$candidate" && ! -L "$candidate/dist" ]] || {
  echo "missing or symlinked candidate directory" >&2; exit 1;
}
for f in CANDIDATE install.sh root.pub revocations.json; do
  [[ -f "$candidate/$f" && ! -L "$candidate/$f" ]] || { echo "missing or symlinked $f" >&2; exit 1; }
done
grep -Fxq "tag=$version" "$candidate/CANDIDATE" || { echo "candidate tag mismatch" >&2; exit 1; }
grep -Fxq "commit=$commit" "$candidate/CANDIDATE" || { echo "candidate commit mismatch" >&2; exit 1; }
grep -Fxq 'signed=false' "$candidate/CANDIDATE" || { echo "candidate must be unsigned" >&2; exit 1; }

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/source"
git -C "$repo_root" archive --format=tar "$commit" | tar -xf - -C "$scratch/source"
cmp -s "$candidate/root.pub" "$scratch/source/release/keys/root.pub" || { echo "candidate root mismatches approved source" >&2; exit 1; }
cmp -s "$candidate/revocations.json" "$scratch/source/release/keys/revocations.json" || { echo "candidate revocations mismatch approved source" >&2; exit 1; }
cmp -s "$candidate/install.sh" "$scratch/source/install.sh" || { echo "candidate installer mismatches approved source" >&2; exit 1; }

# Align Go patch version with the Actions build. Different Go patch releases
# can produce different bytes even if source, GOOS and GOARCH are identical.
go_version=$(awk '$1 == "go" { print $2; exit }' "$scratch/source/go.mod")
[[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "unable to pin Go version" >&2; exit 1; }
(
  cd "$scratch/source"
  env GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN="go$go_version" \
    bash scripts/release/build.sh "$version" "$scratch/expected" >/dev/null
)

shopt -s nullglob dotglob
candidate_entries=("$candidate/dist"/*)
expected_entries=("$scratch/expected"/*)
[[ ${#candidate_entries[@]} -eq ${#expected_entries[@]} ]] || {
  echo "candidate dist file count differs from independently built output" >&2; exit 1;
}
for expected in "${expected_entries[@]}"; do
  name=${expected##*/}
  provided="$candidate/dist/$name"
  [[ -f "$provided" && ! -L "$provided" ]] || { echo "missing or unsafe artifact: $name" >&2; exit 1; }
  cmp -s "$provided" "$expected" || { echo "artifact source/provenance mismatch: $name" >&2; exit 1; }
done
echo "PROVENANCE VERIFIED: all ${#expected_entries[@]} candidate assets match clean source $commit (Go $go_version)"
