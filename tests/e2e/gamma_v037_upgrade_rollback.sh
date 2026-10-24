#!/usr/bin/env bash
# Gamma P0-C disposable Linux drill against real v0.3.7 BCC and current candidate.
# Verifies forward schema migration, fail-closed older binary, cold snapshot
# rollback and re-upgrade without touching production, systemd or remote hosts.
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$(uname -s)" == Linux ]] || { echo "Linux required" >&2; exit 1; }
work="$(mktemp -d)"
old_src="$work/old-src"
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  git worktree remove --force "$old_src" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
fail() { echo "[bcc-schema-drill] FAIL: $*" >&2; for f in "$work"/*.log; do [[ -f "$f" ]] && tail -n 30 "$f" >&2; done; exit 1; }

if ! git cat-file -e 'v0.3.7^{commit}' 2>/dev/null; then git fetch origin tag v0.3.7; fi
old_sha="$(git rev-parse 'v0.3.7^{commit}')"
[[ "$old_sha" == 94c55dccf5af7cd126089374d991463931413121 ]] || fail "v0.3.7 tag moved: $old_sha"
git worktree add --detach "$old_src" "$old_sha" >/dev/null
go -C "$old_src" build -o "$work/old-bcc" ./cmd/baft-bcc
go build -o "$work/new-bcc" ./cmd/baft-bcc

state="$work/state.db"
access="$work/access.json"
jobkey="$work/jobkey"
token_file="$work/admin-token"
printf 'isolated-drill-token\n' >"$token_file"
chmod 0600 "$token_file"
"$work/old-bcc" access init --access-file "$access" >/dev/null
"$work/old-bcc" jobkey show --job-key-file "$jobkey" >/dev/null
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
url="http://127.0.0.1:$port"
start_bcc() {
  local binary="$1" logfile="$2" attempt
  "$binary" --listen "127.0.0.1:$port" --state-file "$state" \
    --admin-token-file "$token_file" --access-file "$access" --job-key-file "$jobkey" \
    >"$logfile" 2>&1 &
  server_pid=$!
  for attempt in $(seq 1 100); do
    if curl -fsS -H 'Authorization: Bearer isolated-drill-token' "$url/api/nodes" >/dev/null 2>&1; then return 0; fi
    kill -0 "$server_pid" 2>/dev/null || fail "$binary exited before serving"
    sleep 0.1
  done
  fail "$binary did not serve on loopback"
}
stop_bcc() {
  kill "$server_pid"
  wait "$server_pid" || true
  server_pid=""
}
schema() {
  python3 - "$state" <<'PY'
import sqlite3, sys
db = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
print(db.execute("SELECT MAX(version) FROM schema_migrations").fetchone()[0])
db.close()
PY
}
post_node() {
  local id="$1" code
  code="$(curl -sS -o "$work/post.json" -w '%{http_code}' \
    -H 'Authorization: Bearer isolated-drill-token' -H 'Content-Type: application/json' \
    -d "{\"ID\":\"$id\",\"Alias\":\"$id\",\"Address\":\"127.0.0.1:1\",\"Role\":\"foreign\"}" \
    "$url/api/nodes")"
  [[ "$code" == 201 ]] || fail "POST node $id returned $code: $(cat "$work/post.json")"
}
assert_nodes() {
  curl -fsS -H 'Authorization: Bearer isolated-drill-token' "$url/api/nodes" >"$work/nodes.json"
  python3 - "$work/nodes.json" "$@" <<'PY'
import json, sys
nodes = json.load(open(sys.argv[1]))
ids = {node.get("id", node.get("ID")) for node in nodes}
missing = set(sys.argv[2:]) - ids
if missing:
    raise SystemExit(f"missing nodes after recovery: {sorted(missing)}")
PY
}

start_bcc "$work/old-bcc" "$work/old.log"
OLD_SCHEMA="$(schema)"
[[ "$OLD_SCHEMA" =~ ^[0-9]+$ ]] || fail "old v0.3.7 schema missing"
post_node before-upgrade
stop_bcc
mkdir -p "$work/preupgrade"
cp -p "$state"* "$work/preupgrade/"
[[ -s "$work/preupgrade/$(basename "$state")" ]] || fail "cold database snapshot missing"
start_bcc "$work/new-bcc" "$work/new.log"
NEW_SCHEMA="$(schema)"
[[ "$NEW_SCHEMA" =~ ^[0-9]+$ && "$NEW_SCHEMA" -gt "$OLD_SCHEMA" ]] || fail "forward schema migration not observed"
post_node after-upgrade
stop_bcc

set +e
timeout 10s "$work/old-bcc" --listen "127.0.0.1:$port" --state-file "$state" \
  --admin-token-file "$token_file" --access-file "$access" --job-key-file "$jobkey" \
  >"$work/old-refusal.log" 2>&1
old_rc=$?
set -e
[[ "$old_rc" != 0 && "$old_rc" != 124 ]] || fail "old binary accepted migrated schema or hung"
grep -q 'newer than this build supports' "$work/old-refusal.log" ||
  fail "old binary refused for an unexpected reason"

start_bcc "$work/new-bcc" "$work/recovered.log"
assert_nodes before-upgrade after-upgrade
[[ "$(schema)" == "$NEW_SCHEMA" ]] || fail "recovery changed schema unexpectedly"
stop_bcc
# Replace the whole stopped-BCC state using the sealed pre-upgrade
# filesystem snapshot, then verify the old binary starts safely again.
rm -f -- "$state"*
cp -p "$work/preupgrade/"* "$work/"
start_bcc "$work/old-bcc" "$work/rollback.log"
[[ "$(schema)" == "$OLD_SCHEMA" ]] || fail "cold rollback schema mismatch"
assert_nodes before-upgrade
curl -fsS -H 'Authorization: Bearer isolated-drill-token' "$url/api/nodes" >"$work/rollback-nodes.json"
python3 - "$work/rollback-nodes.json" <<'PY'
import json,sys
nodes = json.load(open(sys.argv[1]))
ids = {n.get("id",n.get("ID")) for n in nodes}
assert "after-upgrade" not in ids, ids
PY
stop_bcc
start_bcc "$work/new-bcc" "$work/reupgrade.log"
[[ "$(schema)" == "$NEW_SCHEMA" ]] || fail "re-upgrade schema mismatch"
assert_nodes before-upgrade
stop_bcc
echo "[gamma-v037] PASS: v0.3.7 upgraded, old binary refused new state, cold snapshot rolled back, and re-upgrade succeeded"
