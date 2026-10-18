#!/usr/bin/env bash
# BAFT v0.3.5: sign the CI candidate offline with release key 702c... and,
# after a typed confirmation, publish the GitHub release.
# Run on the Mac with the BAFT-KEYS disk image mounted:
#   bash sign-and-publish-v0.3.5.sh
# Needs: git, gh (logged in), go, python3, sha256sum/gtar (brew coreutils gnu-tar).
# The private key is read only by the signer built from the tagged source;
# it is never copied, printed or uploaded.
set -euo pipefail
V=v0.3.5
C=0f6d2c94b5a6691924bfbd2ec9ca572b1121b0c5
REPO=zarkmakerburg/BAFT
KEY=${BAFT_RELEASE_KEY_FILE:-/Volumes/BAFT-KEYS/release-20261012.key}
CERT=${BAFT_RELEASE_CERT:-/Volumes/BAFT-KEYS/release-20261012.cert.json}
W=${BAFT_RELEASE_WORK:-$HOME/code/baft-release-$V}

say() { printf '\n== %s\n' "$*"; }
for f in "$KEY" "$CERT"; do
  [[ -f "$f" ]] || { echo "missing $f (is BAFT-KEYS mounted?)"; ls /Volumes/BAFT-KEYS 2>/dev/null; exit 1; }
done
grep -q 702c62788d60920da8ea80aab93ca3e1 "$CERT" ||
  python3 - "$CERT" <<'PY' || { echo "certificate is not for key 702c62788d60920da8ea80aab93ca3e1"; exit 1; }
import base64, json, sys
c = json.load(open(sys.argv[1]))
p = json.loads(base64.b64decode(c["payload"]))
sys.exit(0 if p.get("key_id") == "702c62788d60920da8ea80aab93ca3e1" else 1)
PY

say "fresh checkout of $V"
rm -rf "$W"; mkdir -p "$W"; cd "$W"
git clone -q "https://github.com/$REPO.git" src
cd src
git checkout -q "$V"
[[ "$(git rev-parse HEAD)" == "$C" ]] || { echo "tag $V is not $C"; exit 1; }

say "download the unsigned CI candidate"
RUN=$(gh run list -R "$REPO" --workflow release.yml --limit 20 --json databaseId,headSha,conclusion \
  --jq ".[] | select(.headSha==\"$C\" and .conclusion==\"success\") | .databaseId" | head -n1)
[[ -n "$RUN" ]] || { echo "no successful release.yml run for $C; re-run it on the tag (the artifact lives 1 day)"; exit 1; }
gh run download "$RUN" -R "$REPO" -n unsigned-release-candidate -D "$W/candidate"

say "verify provenance, sign, verify"
bash scripts/release/mobile_sign_termux.sh "$W/candidate" "$V" "$C" "$KEY" "$CERT"

say "offline bundle"
bash scripts/release/offline_bundle.sh "$W/candidate/dist" "$W/candidate/revocations.json" \
  "$W/candidate/install.sh" "$W/out" "$W/candidate/notices"
( cd "$W/candidate/dist" && sha256sum -c SHA256SUMS )

say "release notes from CHANGELOG"
awk '/^## \[Unreleased\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.md | grep -v '^Candidate for v0.3.5' >"$W/notes.md"
printf '\nSigned offline with release key 702c62788d60920da8ea80aab93ca3e1. Source commit %s.\n' "$C" >>"$W/notes.md"

ls -1 "$W/candidate/dist" "$W/out"
printf '\nSigned and verified. Type PUBLISH to create the public GitHub release %s: ' "$V"
read -r answer
[[ "$answer" == PUBLISH ]] || { echo "not published; signed files are in $W"; exit 0; }
gh release create "$V" -R "$REPO" --verify-tag --title "BAFT $V" --notes-file "$W/notes.md" \
  "$W"/candidate/dist/* "$W"/out/baft-offline-"$V".tar.gz "$W"/out/baft-offline-"$V".tar.gz.sha256 \
  "$W"/candidate/notices/LICENSE "$W"/candidate/notices/COPYRIGHT "$W"/candidate/notices/THIRD_PARTY_LICENSES.md
say "published: https://github.com/$REPO/releases/tag/$V"
