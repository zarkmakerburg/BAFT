#!/usr/bin/env bash
# Sign an HQ-approved BAFT release candidate on Android/Termux.
# Usage: mobile_sign_termux.sh CANDIDATE_DIR VERSION COMMIT RELEASE_KEY RELEASE_CERT
set -euo pipefail
umask 077
[[ $# -eq 5 ]] || { echo "usage: $0 CANDIDATE_DIR vX.Y.Z FULL_COMMIT_SHA RELEASE_KEY RELEASE_CERT" >&2; exit 2; }
candidate=$1
version=$2
commit=$3
key=$4
cert=$5
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "invalid version" >&2; exit 2; }
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "full 40-character lowercase commit SHA required" >&2; exit 2; }
[[ -f "$key" && -f "$cert" && ! -L "$key" && ! -L "$cert" ]] || { echo "regular non-symlink release key and certificate required" >&2; exit 2; }
[[ -f "$candidate/CANDIDATE" && -d "$candidate/dist" && -f "$candidate/root.pub" && -f "$candidate/revocations.json" && -f "$candidate/install.sh" ]] || { echo "incomplete candidate" >&2; exit 1; }
# Do not follow symlinked candidate inputs: a malicious candidate could otherwise
# redirect checks or signing output outside the intended release directory.
for path in "$candidate" "$candidate/dist" "$candidate/CANDIDATE" "$candidate/root.pub" "$candidate/revocations.json" "$candidate/install.sh"; do
  [[ ! -L "$path" ]] || { echo "symlinked candidate input refused: $path" >&2; exit 1; }
done
for path in "$candidate/dist"/*; do
  [[ ! -L "$path" ]] || { echo "symlinked dist entry refused: $path" >&2; exit 1; }
done
for f in manifest.json release-key.cert.json SHA256SUMS; do
  [[ ! -e "$candidate/dist/$f" && ! -L "$candidate/dist/$f" ]] || { echo "candidate is already signed or contaminated: $f" >&2; exit 1; }
done
grep -Fxq "tag=$version" "$candidate/CANDIDATE" || { echo "candidate tag mismatch" >&2; exit 1; }
grep -Fxq "commit=$commit" "$candidate/CANDIDATE" || { echo "candidate commit mismatch" >&2; exit 1; }
grep -Fxq "signed=false" "$candidate/CANDIDATE" || { echo "not an unsigned candidate" >&2; exit 1; }
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$commit" ]] || { echo "signer checkout does not match approved commit" >&2; exit 1; }
# A candidate metadata file alone is not evidence that HQ approved a tag.
tagged_commit=$(git -C "$repo_root" rev-parse -q --verify "refs/tags/$version^{commit}") || {
  echo "approved release tag does not exist in local refs" >&2; exit 1;
}
[[ "$tagged_commit" == "$commit" ]] || { echo "release tag points to a different commit" >&2; exit 1; }
git -C "$repo_root" merge-base --is-ancestor "$commit" refs/remotes/origin/main || {
  echo "approved tag commit is not in the locally fetched origin/main history" >&2; exit 1;
}
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] || { echo "signer checkout has tracked or untracked modifications" >&2; exit 1; }
cmp -s "$candidate/root.pub" "$repo_root/release/keys/root.pub" || { echo "root key mismatch" >&2; exit 1; }
cmp -s "$candidate/revocations.json" "$repo_root/release/keys/revocations.json" || { echo "revocations mismatch" >&2; exit 1; }
for arch in amd64 arm64; do
  cmp -s "$candidate/install.sh" "$candidate/dist/baft-install-linux-$arch" || { echo "installer mismatch ($arch)" >&2; exit 1; }
done
# Fail closed if any downloaded binary differs from the independently
# rebuilt, clean, approved Git commit. No production key is read before this.
bash "$repo_root/scripts/release/verify_candidate_provenance.sh" "$candidate" "$version" "$commit"
# Also compile the signer from a pristine Git archive: neither untracked nor
# Git-ignored .go files in the operator checkout may enter the signer.
signer=$(mktemp -d)
trap 'rm -rf "$signer"' EXIT
mkdir -p "$signer/source"
git -C "$repo_root" archive --format=tar "$commit" | tar -xf - -C "$signer/source"
go_version=$(awk '$1 == "go" { print $2; exit }' "$signer/source/go.mod")
[[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "unrecognized Go version" >&2; exit 1; }
(cd "$signer/source" && env GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN="go$go_version" go build -o "$signer/baft-release" ./cmd/baft-release)
echo "Approved candidate: $version at $commit"
echo "Signing with existing root-certified release key (never uploaded)."
"$signer/baft-release" sign -dir "$candidate/dist" -version "$version" -commit "$commit" -cert "$cert" -key "$key" -root-pub "$candidate/root.pub"
"$signer/baft-release" verify -dir "$candidate/dist" -root-pub "$candidate/root.pub" -revocations "$candidate/revocations.json"
echo "SIGNED AND VERIFIED LOCALLY. Publication still requires explicit HQ approval."
