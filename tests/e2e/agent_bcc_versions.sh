#!/usr/bin/env bash
# A real baft-agent reports its binaries to a real baft-bcc that holds a
# signed release catalog. /api/nodes/versions is checked after every step:
# verified, mismatch, stale, persistence across a BCC restart, a BCC outage,
# a catalog upgrade, an explicit rollback, a revoked release key and a revoked
# node. Everything runs on loopback with releases signed by throwaway keys
# (scripts/release/local_release.sh). Nothing here touches a real server,
# systemd or the network, and no root is needed.
#
#   tests/e2e/agent_bcc_versions.sh
set -Eeuo pipefail
cd "$(dirname "$0")/../.."

case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported machine $(uname -m)"; exit 1 ;;
esac
PORT=${BAFT_E2E_BCC_PORT:-18310}
BCC_URL="http://127.0.0.1:$PORT"
WORK="$(mktemp -d)"
BCC_PID=""
STEP="setup"
log() { printf '[agent-bcc-versions] %s\n' "$*"; }
fail() { echo "FAIL ($STEP): $*" >&2; exit 1; }
cleanup() {
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    for f in "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f"; tail -n 40 "$f"; }; done
    [[ -f "$WORK/versions.json" ]] && { echo "== last /api/nodes/versions"; cat "$WORK/versions.json"; }
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      echo "::error title=agent-bcc-versions::failed at step: $STEP"
      [[ -f "$WORK/bcc.log" ]] && tail -n 10 "$WORK/bcc.log" | sed 's/%/%25/g; s/^/::error title=bcc.log::/'
    fi
  fi
  [[ -n "$BCC_PID" ]] && kill "$BCC_PID" 2>/dev/null || true
  rm -rf "$WORK"
  exit $rc
}
trap cleanup EXIT

# ---- two signed releases under one throwaway root key -----------------------
STEP="build releases"
log "building signed releases v0.0.1-local and v0.0.2-local (one root key)"
BAFT_LOCAL_RELEASE_ROOT="$WORK/root" scripts/release/local_release.sh "$WORK/rel1" v0.0.1-local >"$WORK/rel1.log" 2>&1
BAFT_LOCAL_RELEASE_ROOT="$WORK/root" scripts/release/local_release.sh "$WORK/rel2" v0.0.2-local >"$WORK/rel2.log" 2>&1
COMMIT="$(git rev-parse HEAD)"
cmp -s "$WORK/rel1/dist/baft-linux-$ARCH" "$WORK/rel2/dist/baft-linux-$ARCH" &&
  fail "the two releases ship the same baft binary; the upgrade and rollback steps would prove nothing"

# The BCC catalog is a "current" link the operator rotates between releases.
# The revocation list and the anti-rollback state live outside the signed trees.
mkdir -p "$WORK/catalog" "$WORK/trust"
ln -s "$WORK/rel1/dist" "$WORK/catalog/current"
cp "$WORK/rel1/revocations.json" "$WORK/catalog/revocations.json"

# ---- the node: release binaries in a bin dir the agent hashes ---------------
NODE="$WORK/node"
mkdir -p "$NODE/bin" "$NODE/state" "$NODE/etc" "$NODE/baft-state" "$NODE/units"
install_release() { # <release dir>: put that release's binaries on the node
  local bin
  for bin in baft baft-agent baft-bcc; do
    install -m 0755 "$1/dist/$bin-linux-$ARCH" "$NODE/bin/$bin"
  done
}
install_release "$WORK/rel1"
cp "$WORK/rel1/root.pub" "$NODE/release-root.pub"

# ---- BCC ---------------------------------------------------------------------
BCC_BIN="$WORK/rel1/dist/baft-bcc-linux-$ARCH"
echo admintok >"$WORK/admin.token"; chmod 600 "$WORK/admin.token"
"$BCC_BIN" access init --access-file "$WORK/s.json.access.json" >/dev/null
AGENT_TOKEN="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
printf '%s\n' "$AGENT_TOKEN" >"$NODE/token"; chmod 600 "$NODE/token"
start_bcc() {
  AGENT_TOK="$AGENT_TOKEN" "$BCC_BIN" --admin-token-file "$WORK/admin.token" --state-file "$WORK/s.json" \
    --listen "127.0.0.1:$PORT" \
    --component-catalog-dir "$WORK/catalog/current" \
    --component-catalog-root "$WORK/root/root.pub" \
    --component-catalog-revocations "$WORK/catalog/revocations.json" \
    --component-catalog-trust-state "$WORK/trust/catalog-state.json" >>"$WORK/bcc.log" 2>&1 &
  BCC_PID=$!
  local _
  for _ in $(seq 1 50); do
    curl -fsS -o /dev/null -H 'Authorization: Bearer admintok' "$BCC_URL/api/nodes" && return 0
    kill -0 "$BCC_PID" 2>/dev/null || fail "baft-bcc exited during startup"
    sleep 0.2
  done
  fail "baft-bcc did not come up"
}
stop_bcc() {
  kill "$BCC_PID"
  wait "$BCC_PID" 2>/dev/null || true
  BCC_PID=""
}
api() { curl -sS -H 'Authorization: Bearer admintok' "$@"; }
# status <code> <curl args...>: call the admin API and require that HTTP status.
status() {
  local want=$1; shift
  local got
  got="$(api -o "$WORK/api.out" -w '%{http_code}' "$@")"
  [[ "$got" == "$want" ]] || fail "HTTP $got (want $want) from $*: $(head -c 300 "$WORK/api.out")"
}
start_bcc
"$BCC_BIN" jobkey show --job-key-file "$WORK/s.json.job-key" >"$NODE/bcc-job.pub"
status 201 -d '{"ID":"node-e2e","Alias":"versions e2e","Address":"127.0.0.1:1","Role":"foreign","AgentTokenEnv":"AGENT_TOK"}' "$BCC_URL/api/nodes"

# ---- agent -------------------------------------------------------------------
# One poll by the real agent binary installed on the node. It sends its
# component digests before it pulls jobs. The test queues no jobs.
run_agent() {
  "$NODE/bin/baft-agent" --once --allow-insecure-http --bcc-url "$BCC_URL" --node-id node-e2e \
    --token-file "$NODE/token" --bcc-job-key "$NODE/bcc-job.pub" --release-root "$NODE/release-root.pub" \
    --state-dir "$NODE/state" --release-state "$NODE/release-state.json" --bin-dir "$NODE/bin" \
    --config-dir "$NODE/etc" --baft-state-dir "$NODE/baft-state" --unit-dir "$NODE/units" \
    --service baft-e2e-none --service-user nobody >>"$WORK/agent.log" 2>&1
}
# Reports must have strictly increasing observation times, so wait a second between them.
report() { sleep 1.1; run_agent || fail "agent poll failed (see agent.log)"; }

# ---- reading /api/nodes/versions --------------------------------------------
versions() { api -f "$BCC_URL/api/nodes/versions" >"$WORK/versions.json" || fail "GET /api/nodes/versions failed"; }
# field <key>: prints one value from the last /api/nodes/versions answer.
#   catalog_status | catalog_version | states
#   <key>:<component> for a component field (state, version, observed_at)
field() {
  python3 - "$WORK/versions.json" "$1" <<'PY'
import json,sys
j=json.load(open(sys.argv[1]))
key,_,comp=sys.argv[2].partition(":")
if key in ("catalog_status","catalog_version"):
    print(j[key])
    sys.exit()
n=next((x for x in j["nodes"] if x["node_id"]=="node-e2e"),None)
parts=n["components"] if n else []
if key=="states":
    c={x["component"]:x for x in parts}
    print(" ".join([j["catalog_status"]]+[c[k]["state"] for k in ("transport","agent","bcc")]))
else:
    c=next((x for x in parts if x["component"]==comp),{})
    print(c.get(key,""))
PY
}
# expect <catalog_status> <transport> <agent> <bcc> [version]: the state of every
# component and, for each verified component, the catalog version it matched.
expect() {
  local cat=$1 t=$2 a=$3 b=$4 ver=${5:-}
  versions
  local got
  got="$(field states)"
  [[ "$got" == "$cat $t $a $b" ]] || fail "catalog/transport/agent/bcc = '$got', want '$cat $t $a $b'"
  local k
  for k in transport agent bcc; do
    if [[ "$(field state:$k)" == verified ]]; then
      [[ "$(field version:$k)" == "$ver" ]] || fail "$k verified as '$(field version:$k)', want '$ver'"
    else
      [[ -z "$(field version:$k)" ]] || fail "$k is not verified but carries a version"
    fi
  done
  log "  $STEP: catalog=$cat transport=$t agent=$a bcc=$b${ver:+ ($ver)}"
}
observed_at() { versions; field observed_at:transport; }

# ---- scenarios ---------------------------------------------------------------
STEP="no report yet"
expect verified unknown unknown unknown
[[ "$(versions; field catalog_version)" == v0.0.1-local ]] || fail "catalog_version is not v0.0.1-local"

STEP="report before the platform is confirmed"
report
[[ -n "$(observed_at)" ]] || fail "the BCC did not store the agent's report"
expect verified unknown unknown unknown

STEP="platform confirmed, but the stored report predates the confirmation"
status 400 -d "{\"node_id\":\"node-e2e\",\"os\":\"linux\",\"arch\":\"$ARCH\"}" "$BCC_URL/api/nodes/platform"
status 200 -d "{\"node_id\":\"node-e2e\",\"os\":\"linux\",\"arch\":\"$ARCH\",\"confirmation\":\"verified-out-of-band\"}" "$BCC_URL/api/nodes/platform"
expect verified unknown unknown unknown

STEP="fresh report from the release-installed binaries"
report
expect verified verified verified verified v0.0.1-local

STEP="the report survives a BCC restart"
stop_bcc; start_bcc
expect verified verified verified verified v0.0.1-local

STEP="BCC outage"
before="$(observed_at)"
stop_bcc
sleep 1.1
if run_agent; then fail "the agent poll succeeded with the BCC down"; fi
start_bcc
expect verified verified verified verified v0.0.1-local
[[ "$(observed_at)" == "$before" ]] || fail "a report landed while the BCC was down"
report
[[ "$(observed_at)" != "$before" ]] || fail "the agent's first report after the outage was not accepted"
expect verified verified verified verified v0.0.1-local

STEP="tampered transport binary"
cp "$NODE/bin/baft" "$WORK/baft.good"
printf 'x' >>"$NODE/bin/baft"
report
expect verified mismatch verified verified v0.0.1-local
install -m 0755 "$WORK/baft.good" "$NODE/bin/baft"
report
expect verified verified verified verified v0.0.1-local

# The BCC judges freshness by when it accepted a report (15-minute window),
# never by the agent's clock. The e2e shifts the stored accept time and the
# platform confirmation back by the same amount, so that age is the only thing
# that can change the answer. Offsets are cumulative: 14 min, then 16 min.
age_back() { # <seconds>: move the stored receipt and platform confirmation into the past
  python3 - "$WORK/s.json" "$1" <<'PY'
import json,sqlite3,sys
from datetime import datetime,timedelta
db=sqlite3.connect(sys.argv[1]); shift=timedelta(seconds=int(sys.argv[2]))
def back(ts):
    t=datetime.fromisoformat(ts.replace("Z","+00:00"))-shift
    return t.isoformat().replace("+00:00","Z")
(doc,),=db.execute("SELECT doc FROM component_digest_receipts WHERE node_id='node-e2e'")
r=json.loads(doc); r["accepted_at"]=back(r["accepted_at"]); r["observed_at"]=back(r["observed_at"])
db.execute("UPDATE component_digest_receipts SET doc=? WHERE node_id='node-e2e'",(json.dumps(r),))
(doc,),=db.execute("SELECT doc FROM nodes WHERE id='node-e2e'")
n=json.loads(doc); n["platform"]["confirmed_at"]=back(n["platform"]["confirmed_at"])
db.execute("UPDATE nodes SET doc=? WHERE id='node-e2e'",(json.dumps(n),))
db.commit()
PY
}
STEP="report aged 14 minutes (still inside the window)"
stop_bcc; age_back 840; start_bcc
expect verified verified verified verified v0.0.1-local
STEP="report aged 16 minutes (stale)"
stop_bcc; age_back 120; start_bcc
expect verified unknown unknown unknown
STEP="a fresh report after the stale one"
report
expect verified verified verified verified v0.0.1-local

STEP="catalog upgraded to v0.0.2-local, node still on v0.0.1-local"
ln -sfn "$WORK/rel2/dist" "$WORK/catalog/current"
# Both releases come from the same commit. baft-agent has no version stamp, so
# its bytes match in both releases. baft and baft-bcc embed the version, so they must not match.
expect verified mismatch verified mismatch v0.0.2-local
[[ "$(versions; field catalog_version)" == v0.0.2-local ]] || fail "catalog_version is not v0.0.2-local"

STEP="node upgraded to v0.0.2-local"
install_release "$WORK/rel2"
report
expect verified verified verified verified v0.0.2-local

STEP="operator accepts v0.0.2-local"
status 400 -d "{\"version\":\"v0.0.2-local\",\"commit\":\"$COMMIT\"}" "$BCC_URL/api/nodes/versions/accept"
status 200 -d "{\"version\":\"v0.0.2-local\",\"commit\":\"$COMMIT\",\"confirmation\":\"ACCEPT v0.0.2-local $COMMIT\"}" "$BCC_URL/api/nodes/versions/accept"
[[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' "$WORK/trust/catalog-state.json")" == v0.0.2-local ]] ||
  fail "trust state did not record the accepted release"

STEP="catalog rolled back to v0.0.1-local without operator approval"
ln -sfn "$WORK/rel1/dist" "$WORK/catalog/current"
expect unavailable unknown unknown unknown
[[ -z "$(versions; field catalog_version)" ]] || fail "an unverified catalog still reports a version"

STEP="rollback approval needs the explicit downgrade confirmation"
status 409 -d "{\"version\":\"v0.0.1-local\",\"commit\":\"$COMMIT\",\"confirmation\":\"ACCEPT v0.0.1-local $COMMIT\"}" "$BCC_URL/api/nodes/versions/accept"
status 400 -d "{\"version\":\"v0.0.1-local\",\"commit\":\"$COMMIT\",\"allow_downgrade\":true,\"confirmation\":\"ACCEPT v0.0.1-local $COMMIT\"}" "$BCC_URL/api/nodes/versions/accept"
expect unavailable unknown unknown unknown
status 200 -H "X-BAFT-Downgrade-Confirmation: DOWNGRADE v0.0.1-local $COMMIT" \
  -d "{\"version\":\"v0.0.1-local\",\"commit\":\"$COMMIT\",\"allow_downgrade\":true,\"confirmation\":\"ACCEPT v0.0.1-local $COMMIT\"}" "$BCC_URL/api/nodes/versions/accept"
expect verified mismatch verified mismatch v0.0.1-local

STEP="node rolled back to v0.0.1-local"
install_release "$WORK/rel1"
report
expect verified verified verified verified v0.0.1-local

STEP="the release key of v0.0.1-local is revoked"
KEYID="$(python3 -c '
import json,base64,sys
e=json.load(open(sys.argv[1]))
print(json.loads(base64.b64decode(e["payload"]))["key_id"])' "$WORK/rel1/dist/release-key.cert.json")"
go run ./cmd/baft-release revoke -root-key "$WORK/root/root.key" -in "$WORK/catalog/revocations.json" \
  -key-id "$KEYID" -valid-days 30 -out "$WORK/revocations.new.json" >>"$WORK/rel1.log"
mv "$WORK/revocations.new.json" "$WORK/catalog/revocations.json"
expect unavailable unknown unknown unknown
STEP="the revoked key stays refused after a BCC restart"
stop_bcc; start_bcc
expect unavailable unknown unknown unknown

STEP="v0.0.2-local (a different, unrevoked release key) is still trusted"
ln -sfn "$WORK/rel2/dist" "$WORK/catalog/current"
install_release "$WORK/rel2"
report
expect verified verified verified verified v0.0.2-local

STEP="node revoked"
before="$(observed_at)"
status 200 -d '{"node_id":"node-e2e","reason":"e2e"}' "$BCC_URL/api/nodes/revoke"
expect verified unknown unknown unknown
sleep 1.1
if run_agent; then fail "a revoked node's agent poll succeeded"; fi
[[ "$(observed_at)" == "$before" ]] || fail "the BCC accepted a report from a revoked node"
expect verified unknown unknown unknown

log "PASS"
