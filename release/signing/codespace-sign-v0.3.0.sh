# One-off helper: sign and publish v0.3.0 from a Codespace (see release/SIGNING.md).
# Run with: bash release/signing/codespace-sign-v0.3.0.sh
(
set -euo pipefail
TAG=v0.3.0; C=c1f49b2cd9045ee1db8a665efb9fb70a01cfac8b; RUN=38271337223; R=zarkmakerburg/baft
REPO=${CODESPACE_VSCODE_FOLDER:-/workspaces/baft}
W=$(mktemp -d); echo "work dir: $W"
git -C "$REPO" fetch -q origin main --tags --force
[ "$(git -C "$REPO" rev-parse "$TAG^{commit}")" = "$C" ] || { echo "STOP: tag is not $C"; exit 1; }
git -C "$REPO" merge-base --is-ancestor "$C" origin/main || { echo "STOP: commit not on main"; exit 1; }
git -C "$REPO" worktree add -q --detach "$W/signer" v0.2.0
git -C "$REPO" worktree add -q --detach "$W/src" "$C"
(cd "$W/signer" && go build -o "$W/baft-release" ./cmd/baft-release)
gh run download "$RUN" -R "$R" -n unsigned-release-candidate -D "$W/cand"
cat "$W/cand/CANDIDATE"
grep -q "$C" "$W/cand/CANDIDATE" && grep -q "$TAG" "$W/cand/CANDIDATE" || { echo "STOP: CANDIDATE mismatch"; exit 1; }
cmp "$W/cand/root.pub" "$W/src/release/keys/root.pub"
cmp "$W/cand/revocations.json" "$W/src/release/keys/revocations.json"
gh release download v0.2.0 -R "$R" -p release-key.cert.json -D "$W"
read -rsp "Paste RELEASE key (not root), then Enter: " K </dev/tty; echo
BAFT_RELEASE_SIGNING_KEY="$K" "$W/baft-release" sign -dir "$W/cand/dist" -version "$TAG" -commit "$C" \
  -cert "$W/release-key.cert.json" -root-pub "$W/src/release/keys/root.pub" -build-flag -trimpath -build-flag -buildvcs=false
K=; unset K
"$W/baft-release" verify -dir "$W/cand/dist" -root-pub "$W/src/release/keys/root.pub" -revocations "$W/src/release/keys/revocations.json"
bash "$W/src/scripts/release/offline_bundle.sh" "$W/cand/dist" "$W/cand/revocations.json" "$W/cand/install.sh" "$W/out" "$W/cand/notices"
cat > "$W/notes.md" <<'EON'
## BAFT v0.3.0

Signed release. Verify before use: `baft-release verify -dir <download dir> -root-pub release/keys/root.pub -revocations release/keys/revocations.json`. `install.sh` does this itself and refuses anything that does not verify.

### Highlights since v0.2.0
- Late PONG no longer kills the session (implausible RTT restarts, #190) and symmetric FIN_ACK confirmation (#188, #189).
- Carriers: WebSocket, uTLS, h2/ws failover and transport selection (#93 #94 #96 #98 #100 #109).
- Multi-IR foundation, active/standby health and weighted distribution (#102 #112 #114).
- BCC: command center v3 UI, desired topology, first-class install and bootstrap (#106 #129 #140 #159).
- Operations: path discovery, connectivity matrix, Route Doctor, safe SSH port migration, config snapshot ledger (#154-#158).
- Installer: interactive role, IR install over SSH, path preflight; signed installer files now ship in the release (#84 #86 #133).
- Receive-window autotune and framing performance work (#83 #88 #89); security and release-provenance fixes (#121 #122 #135).

### Upgrading
Re-run `install.sh` on each server with the same flags as before. Update IR and EX together.
EON
gh release create "$TAG" -R "$R" --verify-tag --title "BAFT $TAG" --notes-file "$W/notes.md" \
  "$W"/cand/dist/* "$W"/out/* "$W"/cand/notices/*
git -C "$REPO" worktree remove --force "$W/signer"; git -C "$REPO" worktree remove --force "$W/src"
rm -rf "$W"
echo "RELEASE_OK $TAG"
)
