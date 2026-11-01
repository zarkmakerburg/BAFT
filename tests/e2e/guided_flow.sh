#!/usr/bin/env bash
# Guided tunnel flow, end to end, on one exact commit (v0.5.0 product path).
#
# Runs the same BCC API sequence the Quick Tunnel wizard (web/bcc-action-
# wizard.js) sends, against a real baft-bcc and two agents installed with
# install.sh --agent-only from a signed release of this commit, on one
# throwaway systemd host that plays both servers:
#
#   connection test (signed path probe, IR -> EX) -> evidence-checked
#   auto-select -> reviewed plan with a public client port -> deploy by
#   plan hash -> tunnel active -> operational public ingress applied and
#   independently verified by BCC (ingress.ready) -> real client traffic
#   through the public port, the tunnel and the EX service -> decommission
#   -> public port closed again -> no pairing secret listed.
#
# The IR node is registered with this host's non-loopback address because
# BCC's ingress observer refuses loopback by design. Nothing here is a
# multi-host or physical LAB result: both "servers" share one kernel.
#
#   sudo tests/e2e/guided_flow.sh <release-dir from local_release.sh>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
REL="$(cd "${1:?usage: guided_flow.sh <release-dir>}" && pwd)"
VERSION="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$REL/dist/manifest.json")"
COMMIT="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["commit"])' "$REL/dist/manifest.json")"
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; *) echo "unsupported arch"; exit 1 ;; esac
WORK="$(mktemp -d)"
PIDS=()
BCC=http://127.0.0.1:18300
EX_PORT=18543        # BAFT inter-server port on EX (tested, then auto-selected)
CLIENT_PORT=24443    # public client port on IR (operational ingress)
TARGET_PORT=2543     # EX service destination (echo)
UNITS=(baft-agent-gfex baft-agent-gfir)
log() { printf '[guided] %s\n' "$*"; }
fail() {
  echo "FAIL: $*" >&2
  # fail can run inside $(...); keep the reason for the parent's ERR trap.
  [[ -n "${WORK:-}" ]] && printf '%s\n' "$*" >>"$WORK/fail-reason" 2>/dev/null || true
  [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '%s\n' "$*" | head -c 3000 | tr '\n' ' ' | sed 's/%/%25/g; s/^/::error title=FAIL::/; s/$/\n/'
  exit 1
}
cleanup() {
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    for f in "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f"; tail -n 60 "$f"; }; done
    for u in "${UNITS[@]}"; do echo "== journal $u"; journalctl -u "$u" -n 80 --no-pager 2>/dev/null || true; done
    echo "== tunnels"; curl -fsS -H 'Authorization: Bearer admintok' "$BCC/api/tunnels" 2>/dev/null | head -c 20000 || true
    echo; echo "== path probes"; curl -fsS -H 'Authorization: Bearer admintok' "$BCC/api/path-probes" 2>/dev/null | head -c 8000 || true
    echo; echo "== nodes"; curl -fsS -H 'Authorization: Bearer admintok' "$BCC/api/nodes" 2>/dev/null | head -c 8000 || true
    echo; nft list table inet baft 2>/dev/null || true
    echo "== policy path"; ls -ldn /etc /etc/baft /etc/baft/ingress /etc/baft/ingress/managed-node.json 2>&1 || true
    echo "== policy"; cat /etc/baft/ingress/managed-node.json 2>&1 | head -c 2000 || true
    echo; echo "== agent netns"; readlink "/proc/$(systemctl show -p MainPID --value baft-agent-gfir 2>/dev/null)/ns/net" 2>&1 || true; readlink /proc/self/ns/net || true
    echo "== agent unit sandboxing"; systemctl cat baft-agent-gfir 2>/dev/null | grep -E '^(User|Protect|Private|ReadWrite|ReadOnly|Inaccessible|Capability|NoNewPriv)' || true
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      journalctl -u baft-agent-gfir --no-pager -o cat 2>/dev/null | grep -A3 -E 'operational ingress disabled|public_ingress' | tail -n 8 | cut -c1-1500 | sed 's/%/%25/g; s/^/::error title=ir-ingress::/' || true
      { ls -ldn /etc/baft /etc/baft/ingress /etc/baft/ingress/managed-node.json 2>&1; systemctl cat baft-agent-gfir 2>/dev/null | grep -E '^(User|Protect|Private|ReadWrite|ReadOnly|Inaccessible|Capability)'; } | head -n 12 | sed 's/%/%25/g; s/^/::error title=ir-policy::/' || true
    fi
  fi
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  exit "$rc"
}
trap cleanup EXIT
# Any unexpected command failure becomes a visible, located annotation.
trap 'fail "line $LINENO: unexpected failure of: $BASH_COMMAND. $(tail -n 3 "$WORK/fail-reason" 2>/dev/null | tr "\n" " ")"' ERR

api() { curl -fsS -H 'Authorization: Bearer admintok' "$@"; }
get() { # path -> body. BCC rate-limits /api/ per client address and both
  # agents share 127.0.0.1 here, so a 429 is waited out, never treated as data.
  local out code i
  for i in $(seq 1 20); do
    out="$(curl -sS -H 'Authorization: Bearer admintok' -w '\n%{http_code}' "$BCC$1")" || { sleep 2; continue; }
    code="${out##*$'\n'}"; out="${out%$'\n'*}"
    if [[ "$code" == 429 ]]; then sleep 3; continue; fi
    [[ "$code" =~ ^2 ]] || fail "GET $1 -> HTTP $code: $(printf '%s' "$out" | head -c 1500)"
    printf '%s' "$out"; return 0
  done
  fail "GET $1: still rate-limited or unreachable after retries"
}
post() { # path json -> body; any HTTP error fails with BCC's own reason.
  # A 429 means BCC did not process the request, so retrying it is safe.
  local out code i
  for i in $(seq 1 20); do
    out="$(curl -sS -H 'Authorization: Bearer admintok' -H 'Content-Type: application/json' -d "$2" -w '\n%{http_code}' "$BCC$1")" || fail "POST $1: curl failed"
    code="${out##*$'\n'}"; out="${out%$'\n'*}"
    if [[ "$code" == 429 ]]; then sleep 3; continue; fi
    [[ "$code" =~ ^2 ]] || fail "POST $1 -> HTTP $code: $(printf '%s' "$out" | head -c 1500)"
    printf '%s' "$out"; return 0
  done
  fail "POST $1: still rate-limited after retries"
}
jget() { python3 -c 'import json,sys;d=json.load(sys.stdin)
for k in sys.argv[1:]:
    d=d[int(k)] if isinstance(d,list) else (d.get(k) if isinstance(d,dict) else None)
    if d is None: break
print("" if d is None else (json.dumps(d) if isinstance(d,(dict,list)) else d))' "$@"; }

HOST_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -1)"
[[ -n "$HOST_IP" && "$HOST_IP" != 127.* ]] || fail "this host has no non-loopback IPv4 address for the IR node"
command -v nft >/dev/null || fail "nft is required for operational public ingress"

log "release $VERSION built from $COMMIT; host address $HOST_IP"
[[ "$COMMIT" == "$(git rev-parse HEAD)" ]] || fail "the release was not built from the checked-out commit"
mkdir -p "$WORK/www/$VERSION"
cp "$REL"/dist/* "$WORK/www/$VERSION/"
cp "$REL/revocations.json" "$WORK/www/"
(cd "$WORK/www" && exec python3 -m http.server 18301 --bind 127.0.0.1) >"$WORK/www.log" 2>&1 & PIDS+=($!)

log "BCC (the release's own baft-bcc binary)"
BCC_BIN="$WORK/baft-bcc"; install -m 0755 "$REL/dist/baft-bcc-linux-$ARCH" "$BCC_BIN"
echo admintok >"$WORK/admin.token"; chmod 600 "$WORK/admin.token"
"$BCC_BIN" access init --access-file "$WORK/s.json.access.json" >/dev/null
TOK_EX="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"; TOK_IR="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
TOK_EX="$TOK_EX" TOK_IR="$TOK_IR" "$BCC_BIN" --admin-token-file "$WORK/admin.token" --state-file "$WORK/s.json" \
  --listen 127.0.0.1:18300 >"$WORK/bcc.log" 2>&1 & PIDS+=($!)
for _ in $(seq 1 30); do api -o /dev/null "$BCC/api/nodes" 2>/dev/null && break; sleep 1; done
JOB_KEY="$("$BCC_BIN" jobkey show --job-key-file "$WORK/s.json.job-key")"
post /api/nodes '{"ID":"ex-gf","Alias":"EX","Address":"127.0.0.1:22","Role":"foreign","AgentTokenEnv":"TOK_EX"}' >/dev/null
post /api/nodes "{\"ID\":\"ir-gf\",\"Alias\":\"IR\",\"Address\":\"$HOST_IP:22\",\"Role\":\"worker\",\"AgentTokenEnv\":\"TOK_IR\"}" >/dev/null

enroll() { # name token metrics-port extra-install-flags...
  local n="$1" tok="$2" mport="$3"; shift 3
  printf '%s\n' "$tok" >"$WORK/token-$n"; chmod 600 "$WORK/token-$n"
  env BAFT_INSTALL_FROM=release BAFT_RELEASE_URL="file://$REL/dist" BAFT_ROOT_PUB="$(cat "$REL/root.pub")" \
    BAFT_REVOCATIONS_URL="file://$REL/revocations.json" BAFT_BCC_JOB_KEY="$JOB_KEY" BAFT_AGENT_TOKEN_FILE="$WORK/token-$n" \
    BAFT_AGENT_ALLOW_HTTP=1 BAFT_AGENT_RELEASE_BASE_URL=http://127.0.0.1:18301 \
    BAFT_AGENT_REVOCATIONS_URL=http://127.0.0.1:18301/revocations.json \
    BAFT_AGENT_DIR="/etc/baft-agent-gf$n" BAFT_AGENT_STATE_DIR="/var/lib/baft-agent-gf$n" BAFT_AGENT_UNIT="baft-agent-gf$n" \
    BAFT_AGENT_INTERVAL=2s BAFT_CONFIG_DIR="/etc/baft-gf$n" BAFT_STATE_DIR="/var/lib/baft-gf$n" BAFT_SERVICE="baft-gf$n" \
    BAFT_METRICS_LISTEN="127.0.0.1:$mport" BAFT_INGRESS_MARKER="/var/lib/baft-agent-gf$n/ingress-chain.owner" \
    bash install.sh --agent-only --bcc-url "$BCC" --node-id "$n-gf" "$@" >"$WORK/install-$n.log" 2>&1 \
    || fail "install.sh --agent-only failed for $n: $(tail -n 5 "$WORK/install-$n.log" | tr '\n' ' ')"
  systemctl is-active "baft-agent-gf$n" >/dev/null || fail "agent $n is not running"
}
log "install.sh --agent-only: EX plain, IR with the ingress chain and --provision-ingress-policy"
enroll ex "$TOK_EX" 9391 --no-ingress-chain
enroll ir "$TOK_IR" 9392 --provision-ingress-policy

caps() { get /api/nodes | python3 -c 'import json,sys
for n in json.load(sys.stdin):
    if n.get("ID",n.get("id"))==sys.argv[1]: print(",".join(n.get("capabilities") or []))' "$1"; }
log "both agents report fresh capabilities; IR must report operational-ingress-v1"
for _ in $(seq 1 60); do
  IRC="$(caps ir-gf)"; EXC="$(caps ex-gf)"
  [[ ",$IRC," == *",operational-ingress-v1,"* && -n "$EXC" ]] && break
  sleep 2
done
log "IR capabilities: ${IRC:-none}; EX capabilities: ${EXC:-none}"
[[ ",$IRC," == *",operational-ingress-v1,"* ]] || fail "IR agent never reported operational-ingress-v1 (installed policy / chain / journal not usable): $(journalctl -u baft-agent-gfir -n 20 --no-pager | grep -i ingress | tail -n 5 | tr '\n' ' ')"

python3 tests/e2e/echo.py serve "$TARGET_PORT" >"$WORK/target.log" 2>&1 & PIDS+=($!)
sleep 1

log "step 1 — connection test: signed path probe IR -> EX on $EX_PORT"
PROBE="$(post /api/path-probes "{\"source_node\":\"ir-gf\",\"destination_node\":\"ex-gf\",\"family\":\"4\",\"candidate_ports\":[$EX_PORT]}")"
PID="$(printf '%s' "$PROBE" | jget id)"
for _ in $(seq 1 90); do
  PROBE="$(get "/api/path-probes?id=$PID")"
  case "$(printf '%s' "$PROBE" | jget phase)" in complete|failed|cancelled) break ;; esac
  sleep 3
done
printf '%s' "$PROBE" >"$WORK/probe.json"
CLASS="$(printf '%s' "$PROBE" | jget candidates 0 class)"
log "probe phase=$(printf '%s' "$PROBE" | jget phase) class=$CLASS rtt=$(printf '%s' "$PROBE" | jget candidates 0 connect_rtt_ms)ms attempts=$(printf '%s' "$PROBE" | jget candidates 0 attempts) throughput_kbps=$(printf '%s' "$PROBE" | jget candidates 0 throughput_kbps)"
[[ "$(printf '%s' "$PROBE" | jget phase)" == complete && "$CLASS" == FULL_DATA ]] || fail "connection test did not prove FULL_DATA on $EX_PORT: $PROBE"
[[ "$(printf '%s' "$PROBE" | jget recommended_port)" == "$EX_PORT" ]] || fail "probe did not recommend $EX_PORT"

log "step 2 — evidence-checked auto-select (carrier h2)"
SEL=""
for _ in $(seq 1 15); do
  AUTO="$(post /api/tunnels/auto-select '{"ex_node":"ex-gf","ir_node":"ir-gf","family":"4","carrier":"h2"}')"
  SEL="$(printf '%s' "$AUTO" | jget selected)"
  [[ -n "$SEL" ]] && break
  sleep 2
done
[[ -n "$SEL" ]] || fail "auto-select refused fresh FULL_DATA evidence: $AUTO"
[[ "$(printf '%s' "$SEL" | jget class)" == FULL_DATA && "$(printf '%s' "$SEL" | jget state)" == PASS ]] || fail "auto-select chose a port without fresh FULL_DATA: $SEL"
# Like the wizard, use whatever port the evidence check selected: automatic
# path discovery may have proven another port (e.g. 8443) on the same pair.
EX_PORT="$(printf '%s' "$SEL" | jget port)"
log "auto-select chose EX port $EX_PORT"

log "step 3 — reviewed plan, exactly as the wizard builds it"
REQ="$(python3 - "$SEL" "$EX_PORT" "$CLIENT_PORT" "$TARGET_PORT" <<'PY'
import json, sys
sel = json.loads(sys.argv[1])
print(json.dumps({
    "ex_node": "ex-gf", "ir_node": "ir-gf", "port": int(sys.argv[2]), "client_port": int(sys.argv[3]),
    "target": "127.0.0.1:" + sys.argv[4], "instance_id": "baft-gf-e2e",
    "route_listen": "127.0.0.1:1444", "ex_metrics_listen": "127.0.0.1:9202", "ir_metrics_listen": "127.0.0.1:9302",
    "auto_select_evidence": sel,
}, separators=(",", ":")))
PY
)"
PLAN_OK=False
for _ in $(seq 1 30); do
  PLAN="$(post /api/tunnels/plan "$REQ")"
  PLAN_OK="$(printf '%s' "$PLAN" | jget ok)"
  [[ "$PLAN_OK" == "True" ]] && break
  sleep 1
done
[[ "$PLAN_OK" == "True" ]] || fail "the reviewed plan never passed its gates: $PLAN"
HASH="$(printf '%s' "$PLAN" | jget hash)"

log "step 4 — deploy by plan hash and follow BCC's phases"
T="$(post /api/tunnels "${REQ%\}},\"plan_hash\":\"$HASH\"}" | jget id)"
[[ -n "$T" ]] || fail "deploy returned no tunnel id"
phase=""
for _ in $(seq 1 120); do
  phase="$(get "/api/tunnels?id=$T" | jget phase)"
  case "$phase" in active|rolled_back|rollback_failed|superseded) break ;; esac
  sleep 3
done
[[ "$phase" == active ]] || fail "tunnel ended as '$phase'"

log "step 5 — public ingress must be applied and independently verified (ingress.ready)"
ING=""
for _ in $(seq 1 120); do
  ING="$(get "/api/tunnels?id=$T" | jget ingress)"
  [[ "$(printf '%s' "${ING:-null}" | jget ready)" == True ]] && break
  case "$(printf '%s' "${ING:-null}" | jget state)" in failed|blocked) break ;; esac
  sleep 3
done
log "ingress: ${ING:-none}"
[[ "$(printf '%s' "${ING:-null}" | jget ready)" == True ]] || fail "operational public ingress never became ready: ${ING:-no ingress state}"
nft list table inet baft >"$WORK/nft-applied.txt" 2>&1 || fail "the owned nft table is missing after apply"
grep -q "$CLIENT_PORT" "$WORK/nft-applied.txt" || fail "no owned firewall rule for client port $CLIENT_PORT"

log "step 6 — a real client through the public port, the tunnel and the EX service"
python3 - "$HOST_IP" "$CLIENT_PORT" <<'PY' || fail "client traffic through public ingress failed"
import os, socket, sys, time
host, port = sys.argv[1], int(sys.argv[2])
for attempt in range(15):
    try:
        data = os.urandom(512 * 1024)
        with socket.create_connection((host, port), timeout=10) as s:
            s.sendall(data); s.shutdown(socket.SHUT_WR)
            got = bytearray()
            while True:
                b = s.recv(65536)
                if not b: break
                got += b
        if bytes(got) == data:
            print("echoed", len(got), "bytes"); sys.exit(0)
        print("mismatch", len(got))
    except OSError as e:
        print("attempt", attempt, e)
    time.sleep(2)
sys.exit(1)
PY

if get /api/jobs | grep -qE 'BAFTPAIR1:|BAFTREPLY1:'; then fail "a pairing code is listed by /api/jobs"; fi

log "step 7 — decommission; the public port must close and the owned rule go away"
DPLAN="$(post /api/tunnels/decommission/plan "{\"id\":\"$T\"}")"
DHASH="$(printf '%s' "$DPLAN" | jget hash)"
[[ -n "$DHASH" ]] || fail "no decommission plan: $DPLAN"
post /api/tunnels/decommission "{\"id\":\"$T\",\"plan_hash\":\"$DHASH\"}" >/dev/null
for _ in $(seq 1 120); do
  phase="$(get "/api/tunnels?id=$T" | jget phase)"
  [[ "$phase" == decommissioned ]] && break
  sleep 3
done
[[ "$phase" == decommissioned ]] || fail "decommission ended as '$phase': $(get "/api/tunnels?id=$T" | head -c 2000)"
closed=0
for _ in $(seq 1 30); do
  if ! python3 -c 'import socket,sys;socket.create_connection((sys.argv[1],int(sys.argv[2])),timeout=2)' "$HOST_IP" "$CLIENT_PORT" 2>/dev/null; then closed=1; break; fi
  sleep 2
done
[[ "$closed" == 1 ]] || fail "public port $CLIENT_PORT still accepts connections after decommission: ingress=$(get "/api/tunnels?id=$T" | jget ingress)"
if nft list table inet baft 2>/dev/null | grep -q "dport $CLIENT_PORT"; then fail "owned firewall rule for $CLIENT_PORT survived decommission"; fi

log "PASS: connection test -> auto-select -> plan -> deploy -> verified public ingress -> client traffic -> clean decommission"
