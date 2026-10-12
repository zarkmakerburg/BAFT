#!/usr/bin/env bash
# Sign and publish a BAFT release candidate (release/SIGNING.md), on macOS or Linux.
# Usage: bash release/signing/sign-release.sh vX.Y.Z
# Needs: git, gh (logged in), go, python3; on macOS also GNU tar and coreutils
# (brew install gnu-tar coreutils). Asks for the RELEASE key (never the root key)
# on the terminal; the key is never written to disk.
set -euo pipefail
TAG=${1:?usage: sign-release.sh vX.Y.Z}
R=zarkmakerburg/baft
SIGNER_REF=${BAFT_SIGNER_REF:-v0.2.0}
REPO=$(git rev-parse --show-toplevel)
for c in git gh go python3; do command -v "$c" >/dev/null || { echo "STOP: $c is not installed"; exit 1; }; done
W=$(mktemp -d); echo "work dir: $W"
cleanup() { git -C "$REPO" worktree remove --force "$W/signer" 2>/dev/null || true; git -C "$REPO" worktree remove --force "$W/src" 2>/dev/null || true; rm -rf "$W"; }
trap cleanup EXIT

# GNU tar and sha256sum for offline_bundle.sh on macOS.
mkdir -p "$W/bin"
if ! tar --version 2>/dev/null | grep -q GNU; then
  command -v gtar >/dev/null || { echo "STOP: GNU tar missing: brew install gnu-tar"; exit 1; }
  ln -s "$(command -v gtar)" "$W/bin/tar"
fi
if ! command -v sha256sum >/dev/null; then
  command -v gsha256sum >/dev/null || { echo "STOP: sha256sum missing: brew install coreutils"; exit 1; }
  ln -s "$(command -v gsha256sum)" "$W/bin/sha256sum"
fi
export PATH="$W/bin:$PATH"

git -C "$REPO" fetch -q origin main --tags --force
C=$(git -C "$REPO" rev-parse "$TAG^{commit}") || { echo "STOP: tag $TAG not found"; exit 1; }
git -C "$REPO" merge-base --is-ancestor "$C" origin/main || { echo "STOP: $TAG is not on main"; exit 1; }
echo "tag $TAG = $C"
RUN=$(gh run list -R "$R" --workflow release.yml --limit 50 --json databaseId,headBranch,headSha,conclusion \
  -q ".[] | select(.headBranch==\"$TAG\" and .headSha==\"$C\" and .conclusion==\"success\") | .databaseId" | head -n1)
[ -n "$RUN" ] || { echo "STOP: no successful release.yml run for $TAG yet"; exit 1; }
echo "candidate run $RUN"

git -C "$REPO" worktree add -q --detach "$W/signer" "$SIGNER_REF"
git -C "$REPO" worktree add -q --detach "$W/src" "$C"
(cd "$W/signer" && go build -o "$W/baft-release" ./cmd/baft-release)
gh run download "$RUN" -R "$R" -n unsigned-release-candidate -D "$W/cand"
cat "$W/cand/CANDIDATE"
grep -q "$C" "$W/cand/CANDIDATE" && grep -q "$TAG" "$W/cand/CANDIDATE" || { echo "STOP: CANDIDATE mismatch"; exit 1; }
ROOT="$W/src/release/keys/root.pub"; REV="$W/src/release/keys/revocations.json"
cmp "$W/cand/root.pub" "$ROOT"; cmp "$W/cand/revocations.json" "$REV"
gh release download v0.2.0 -R "$R" -p release-key.cert.json -D "$W"

read -rsp "Paste the RELEASE key (not root), then Enter: " K </dev/tty; echo
BAFT_RELEASE_SIGNING_KEY="$K" "$W/baft-release" sign -dir "$W/cand/dist" -version "$TAG" -commit "$C" \
  -cert "$W/release-key.cert.json" -root-pub "$ROOT" -build-flag -trimpath -build-flag -buildvcs=false
K=; unset K
"$W/baft-release" verify -dir "$W/cand/dist" -root-pub "$ROOT" -revocations "$REV"
bash "$W/src/scripts/release/offline_bundle.sh" "$W/cand/dist" "$W/cand/revocations.json" "$W/cand/install.sh" "$W/out" "$W/cand/notices"

PREV=$(git -C "$REPO" describe --tags --abbrev=0 "$C^" 2>/dev/null || echo v0.2.0)
{
  echo "## BAFT $TAG"; echo
  echo 'Signed release. Verify before use: `baft-release verify -dir <download dir> -root-pub release/keys/root.pub -revocations release/keys/revocations.json`. `install.sh` does this itself and refuses anything that does not verify.'
  echo
  if ! grep -q 'candidate; sign and verify' "$W/cand/notes.md"; then cat "$W/cand/notes.md"; else
    echo "### Changes since $PREV"
    git -C "$REPO" log --first-parent --format='%H' "$PREV..$C" | while read -r h; do
      subj=$(git -C "$REPO" log -1 --format=%s "$h"); body=$(git -C "$REPO" log -1 --format=%b "$h" | sed -n '/./{p;q;}')
      case "$subj" in
        "Merge pull request #"*) echo "- ${subj#Merge pull request } " | sed -E 's/^- (#[0-9]+) from .*/- \1/' | tr -d '\n'; echo " ${body}";;
        *) echo "- $subj";;
      esac
    done | head -n 60
  fi
  echo; echo '### Upgrading'; echo 'Re-run `install.sh` on each server with the same flags as before. Update IR and EX together.'
} > "$W/notes.md"

gh release create "$TAG" -R "$R" --verify-tag --title "BAFT $TAG" --notes-file "$W/notes.md" \
  "$W"/cand/dist/* "$W"/out/* "$W"/cand/notices/*
echo "RELEASE_OK $TAG $C"
