#!/usr/bin/env bash
# Guided tunnel flow, end to end, on one exact commit (v0.5.0 product path).
#
# Runs the same BCC API sequence the Quick Tunnel wizard (web/bcc-action-
# wizard.js) sends, against a real baft-bcc and two agents installed with
# install.sh --agent-only from a signed release of this commit, on one
# throwaway systemd host that plays both servers:
#
#   install with --provision-ingress-policy -> truthful capability (a host
#   agent in the init network namespace must NOT advertise
#   operational-ingress-v1: public ingress Apply supports only the isolated
#   LAB namespace) -> connection test (signed path probe, IR -> EX, must be
#   FULL_DATA) -> evidence-checked auto-select -> reviewed plan WITHOUT a
#   public client port (what the wizard sends when the IR lacks the
#   capability) -> deploy by plan hash -> tunnel active and "ready" with no
#   pending ingress -> real client traffic through the tunnel entry on the IR,
#   the tunnel and the EX service -> no pairing secret listed ->
#   decommission -> tunnel entry closed again.
#
# GF_MODE=host runs the same product path with ADR-003 safe host mode on the
# IR (install.sh --enable-host-ingress): the agent must advertise
# operational-ingress-host-v1 (never the LAB token); a plan with a public
# client port FAILs until the per-port acknowledgement is given; a port that
# an Xray inbound is configured for is refused by the agent before any effect;
# a free port is opened (owned listener + owned rule), BCC's independent probe
# makes it Ready, real client traffic flows through HOST_IP:port, and
# decommission retires exactly the owned listener and rule.
#
# Nothing here is a multi-host or physical LAB result: both "servers" share
# one kernel.
#
#   sudo [GF_MODE=host] tests/e2e/guided_flow.sh <release-dir from local_release.sh>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
REL="$(cd "${1:?usage: guided_flow.sh <release-dir>}" && pwd)"
VERSION="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$REL/dist/manifest.json")"
COMMIT="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["commit"])' "$REL/dist/manifest.json")"
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; *) echo "unsupported arch"; exit 1 ;; esac
WORK="$(mktemp -d)"
PIDS=()
# BCC is served over real HTTPS (throwaway CA in the system trust store),
# exactly like production: agents send authenticated ingress receipts only to
# a direct https:// BCC origin.
BCC=https://127.0.0.1:18300
E2E_CA=/usr/local/share/ca-certificates/baft-guided-e2e.crt
EX_PORT=18543        # BAFT inter-server port on EX (tested, then auto-selected)
TARGET_PORT=2543     # EX service destination (echo)
UNITS=(baft-agent-gfex baft-agent-gfir)
GF_MODE="${GF_MODE:-manual}"        # manual | host
case "$GF_MODE" in manual|host) ;; *) echo "GF_MODE must be manual or host"; exit 2 ;; esac
CLIENT_PORT=18610    # host mode: the public port users connect to on the IR
XRAY_PORT=18620      # host mode: a port an Xray inbound is configured for
XRAY_CFG=/usr/local/etc/xray/config.json
LAST_STEP="start"
log() { LAST_STEP="$*"; printf '[guided] %s\n' "$*"; }
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
    # Always leave one located annotation, whatever path ended the run.
    [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '::error title=exit::rc=%s after step: %s\n' "$rc" "$(printf '%s' "$LAST_STEP" | head -c 300 | sed 's/%/%25/g')" || true
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
  [[ -f "$WORK/xray-created" ]] && rm -f "$XRAY_CFG" && rmdir /usr/local/etc/xray 2>/dev/null
  [[ -f "$WORK/ca-installed" ]] && rm -f "$E2E_CA" && update-ca-certificates --fresh >/dev/null 2>&1
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

log "BCC TLS: throwaway CA in the system trust store, certificate for 127.0.0.1"
command -v openssl >/dev/null && command -v update-ca-certificates >/dev/null || fail "openssl and update-ca-certificates are required"
[[ -e "$E2E_CA" ]] && fail "refusing to overwrite an existing $E2E_CA"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj "/CN=BAFT guided e2e CA" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign" \
  -keyout "$WORK/ca.key" -out "$WORK/ca.crt" >/dev/null 2>&1 || fail "CA generation failed"
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=127.0.0.1" \
  -keyout "$WORK/bcc.key" -out "$WORK/bcc.csr" >/dev/null 2>&1 || fail "BCC key generation failed"
printf 'subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\nbasicConstraints=critical,CA:FALSE\n' >"$WORK/bcc.ext"
openssl x509 -req -in "$WORK/bcc.csr" -CA "$WORK/ca.crt" -CAkey "$WORK/ca.key" -CAcreateserial -days 1 \
  -extfile "$WORK/bcc.ext" -out "$WORK/bcc.crt" >/dev/null 2>&1 || fail "BCC certificate signing failed"
chmod 600 "$WORK/bcc.key"
install -m 0644 "$WORK/ca.crt" "$E2E_CA" && touch "$WORK/ca-installed"
update-ca-certificates >/dev/null 2>&1 || fail "update-ca-certificates failed"

log "BCC (the release's own baft-bcc binary) on https://127.0.0.1:18300"
BCC_BIN="$WORK/baft-bcc"; install -m 0755 "$REL/dist/baft-bcc-linux-$ARCH" "$BCC_BIN"
echo admintok >"$WORK/admin.token"; chmod 600 "$WORK/admin.token"
"$BCC_BIN" access init --access-file "$WORK/s.json.access.json" >/dev/null
TOK_EX="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"; TOK_IR="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
TOK_EX="$TOK_EX" TOK_IR="$TOK_IR" "$BCC_BIN" --admin-token-file "$WORK/admin.token" --state-file "$WORK/s.json" \
  --listen 127.0.0.1:18300 --tls-cert "$WORK/bcc.crt" --tls-key "$WORK/bcc.key" >"$WORK/bcc.log" 2>&1 & PIDS+=($!)
for _ in $(seq 1 30); do api -o /dev/null "$BCC/api/nodes" 2>/dev/null && break; sleep 1; done
api -o /dev/null "$BCC/api/nodes" || fail "BCC is not reachable over verified HTTPS: $(tail -n 5 "$WORK/bcc.log" | tr '\n' ' ')"
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
if [[ "$GF_MODE" == host ]]; then
  # An Xray install that already serves XRAY_PORT (read-only to BAFT).
  [[ -e "$XRAY_CFG" ]] && fail "refusing to overwrite an existing $XRAY_CFG on this host"
  install -d -m 0755 /usr/local/etc/xray
  printf '{"inbounds":[{"tag":"vless-e2e","listen":"0.0.0.0","port":%s,"protocol":"vless"}],"outbounds":[{"protocol":"freedom"}]}\n' "$XRAY_PORT" >"$XRAY_CFG"
  touch "$WORK/xray-created"
  log "install.sh --agent-only: EX plain, IR with the ingress chain and --enable-host-ingress (safe host mode)"
  enroll ex "$TOK_EX" 9391 --no-ingress-chain
  enroll ir "$TOK_IR" 9392 --enable-host-ingress
else
  log "install.sh --agent-only: EX plain, IR with the ingress chain and --provision-ingress-policy"
  enroll ex "$TOK_EX" 9391 --no-ingress-chain
  enroll ir "$TOK_IR" 9392 --provision-ingress-policy
fi

caps() { get /api/nodes | python3 -c 'import json,sys
for n in json.load(sys.stdin):
    if n.get("ID",n.get("id"))==sys.argv[1]: print(",".join(n.get("capabilities") or []))' "$1"; }
log "both agents report fresh capabilities; a host IR agent must NOT claim operational-ingress-v1"
for _ in $(seq 1 60); do
  IRC="$(caps ir-gf)"; EXC="$(caps ex-gf)"
  [[ -n "$IRC" && -n "$EXC" ]] && { [[ "$GF_MODE" != host || ",$IRC," == *",operational-ingress-host-v1,"* ]] && break; }
  sleep 3
done
log "IR capabilities: ${IRC:-none}; EX capabilities: ${EXC:-none}"
[[ -n "$IRC" && -n "$EXC" ]] || fail "agents never reported capabilities"
[[ ",$IRC," != *",operational-ingress-v1,"* ]] || fail "host IR agent advertises operational-ingress-v1 although every Apply refuses the init network namespace"
if [[ "$GF_MODE" == host ]]; then
  [[ ",$IRC," == *",operational-ingress-host-v1,"* ]] || fail "IR in safe host mode does not advertise operational-ingress-host-v1: $IRC; $(journalctl -u baft-agent-gfir -n 30 --no-pager -o cat | grep -i ingress | tail -n 3 | tr '\n' ' ')"
  python3 -c 'import json,sys;p=json.load(open("/etc/baft/ingress/managed-node.json"));sys.exit(0 if p.get("schema_version")==2 and p.get("execution_profile")=="host-v1" else 1)' \
    || fail "installed policy is not the host-v1 profile: $(head -c 600 /etc/baft/ingress/managed-node.json)"
else
journalctl -u baft-agent-gfir --no-pager -o cat | grep -q 'unsupported execution scope' \
  || fail "IR agent did not log why operational ingress is off: $(journalctl -u baft-agent-gfir -n 20 --no-pager -o cat | grep -i ingress | tail -n 3 | tr '\n' ' ')"
# The policy was provisioned BEFORE the agent started (install.sh order fix):
# the reason must be the execution scope, not a missing policy file.
if journalctl -u baft-agent-gfir --no-pager -o cat | grep 'operational ingress disabled' | grep -q 'no such file'; then
  fail "agent started before install.sh provisioned the ingress policy"
fi
fi

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

# make_req client_port ack instance route ex_metrics ir_metrics with_evidence
make_req() {
  python3 - "$SEL" "$EX_PORT" "$TARGET_PORT" "$@" <<'PY'
import json, sys
sel, ex_port, target, client, ack, inst, route, exm, irm, ev = sys.argv[1:11]
r = {"ex_node": "ex-gf", "ir_node": "ir-gf", "port": int(ex_port),
     "target": "127.0.0.1:" + target, "instance_id": inst,
     "route_listen": route, "ex_metrics_listen": exm, "ir_metrics_listen": irm}
if client:
    r["client_port"] = int(client)
    if ack == "1":
        r["client_port_ack"] = True
if ev == "1":
    r["auto_select_evidence"] = json.loads(sel)
print(json.dumps(r, separators=(",", ":")))
PY
}
plan_ok() { # req -> plan json once it passes its gates
  local plan ok i
  for i in $(seq 1 30); do
    plan="$(post /api/tunnels/plan "$1")"; ok="$(printf '%s' "$plan" | jget ok)"
    [[ "$ok" == True ]] && { printf '%s' "$plan"; return 0; }
    sleep 1
  done
  fail "the reviewed plan never passed its gates: $plan"
}
deploy_active() { # req hash -> tunnel id, once active
  local t phase i
  t="$(post /api/tunnels "${1%\}},\"plan_hash\":\"$2\"}" | jget id)"
  [[ -n "$t" ]] || fail "deploy returned no tunnel id"
  for i in $(seq 1 120); do
    phase="$(get "/api/tunnels?id=$t" | jget phase)"
    case "$phase" in active|rolled_back|rollback_failed|superseded) break ;; esac
    sleep 3
  done
  [[ "$phase" == active ]] || fail "tunnel ended as '$phase'"
  printf '%s' "$t"
}
echo_through() { # host port label
  python3 - "$1" "$2" <<'PY' || fail "client traffic through $3 failed"
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
            print("echoed", len(got), "bytes via", host, port); sys.exit(0)
        print("mismatch", len(got))
    except OSError as e:
        print("attempt", attempt, e)
    time.sleep(2)
sys.exit(1)
PY
}
closed_after() { # host port label
  local i
  for i in $(seq 1 30); do
    if ! python3 -c 'import socket,sys;socket.create_connection((sys.argv[1],int(sys.argv[2])),timeout=2)' "$1" "$2" 2>/dev/null; then return 0; fi
    sleep 2
  done
  fail "$3 $1:$2 still accepts connections after decommission"
}
decommission() { # tunnel id
  local t="$1" pending=1 code drift="" out dplan dhash phase i
  # Like launch1.sh for scoped instances: the reviewed decommission needs the
  # initial automatic drift inspection finished and a fresh IN_SYNC baseline.
  for i in $(seq 1 40); do
    pending="$(get "/api/tunnels?id=$t" | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("drift_jobs") or []))')"
    [[ "$pending" == 0 ]] && break
    sleep 3
  done
  [[ "$pending" == 0 ]] || fail "initial drift inspection never finished"
  for i in $(seq 1 10); do
    code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer admintok' "$BCC/api/tunnels/drift?id=$t")"
    [[ "$code" == 429 ]] && { sleep 3; continue; }
    [[ "$code" =~ ^2 ]] || fail "drift check request -> HTTP $code"
    break
  done
  for i in $(seq 1 40); do
    out="$(get "/api/tunnels?id=$t" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(len(d.get("drift_jobs") or []),(d.get("drift") or {}).get("state",""))')"
    [[ "${out%% *}" == 0 && -n "${out#* }" ]] && { drift="${out#* }"; break; }
    sleep 3
  done
  [[ "$drift" == IN_SYNC ]] || fail "tunnel is not IN_SYNC before decommission: '$drift'"
  for i in $(seq 1 20); do
    dplan="$(post /api/tunnels/decommission/plan "{\"id\":\"$t\"}")"
    [[ "$(printf '%s' "$dplan" | jget ok)" == True ]] && break
    sleep 3
  done
  [[ "$(printf '%s' "$dplan" | jget ok)" == True ]] || fail "decommission plan stays blocked: $(printf '%s' "$dplan" | head -c 1500)"
  dhash="$(printf '%s' "$dplan" | jget hash)"
  post /api/tunnels/decommission "{\"id\":\"$t\",\"plan_hash\":\"$dhash\"}" >/dev/null
  for i in $(seq 1 120); do
    phase="$(get "/api/tunnels?id=$t" | jget phase)"
    [[ "$phase" == decommissioned || "$phase" == decommission_failed ]] && break
    sleep 3
  done
  [[ "$phase" == decommissioned ]] || fail "decommission ended as '$phase': $(get "/api/tunnels?id=$t" | head -c 2000)"
}
ingress_state() { get "/api/tunnels?id=$1" | python3 -c 'import json,sys;i=json.load(sys.stdin).get("ingress") or {};print(i.get("state",""),"|",i.get("ready",False),"|",i.get("reason",""))'; }
nft_has_port() { # any rule in BAFT's own chain that matches this numeric port
  nft -j list chain inet baft ingress 2>/dev/null | python3 -c 'import json,sys
port=int(sys.argv[1])
def walk(x):
    if isinstance(x,dict):
        if x.get("right")==port: return True
        return any(walk(v) for v in x.values())
    if isinstance(x,list): return any(walk(v) for v in x)
    return False
rules=[o["rule"] for o in json.load(sys.stdin).get("nftables",[]) if "rule" in o]
sys.exit(0 if any(walk(r.get("expr")) for r in rules) else 1)' "$1"
}

if [[ "$GF_MODE" == manual ]]; then
  log "step 3 — reviewed plan, exactly as the wizard builds it for an IR without automatic public ingress"
  REQ="$(make_req "" 0 baft-gf-e2e 127.0.0.1:1444 127.0.0.1:9202 127.0.0.1:9302 1)"
  HASH="$(plan_ok "$REQ" | jget hash)"

  log "step 4 — deploy by plan hash and follow BCC's phases"
  T="$(deploy_active "$REQ" "$HASH")"

  log "step 5 — truthful readiness: nothing pending, no public ingress staged"
  TJSON="$(get "/api/tunnels?id=$T")"
  [[ -z "$(printf '%s' "$TJSON" | jget client_port)" ]] || fail "a public client port was planned although none was requested: $TJSON"
  [[ -z "$(printf '%s' "$TJSON" | jget ingress)" ]] || fail "public ingress was staged for a tunnel without a client port: $TJSON"
  ROUTE="$(printf '%s' "$TJSON" | jget route_listen)"
  [[ "$ROUTE" == 127.0.0.1:1444 ]] || fail "unexpected tunnel entry on IR: '$ROUTE'"

  log "step 6 — a real client through the tunnel entry on the IR, the tunnel and the EX service"
  echo_through "${ROUTE%:*}" "${ROUTE##*:}" "the tunnel entry"
  if get /api/jobs | grep -qE 'BAFTPAIR1:|BAFTREPLY1:'; then fail "a pairing code is listed by /api/jobs"; fi

  log "step 7 — decommission; the tunnel entry must close"
  decommission "$T"
  closed_after "${ROUTE%:*}" "${ROUTE##*:}" "tunnel entry"
  log "PASS: truthful capability -> connection test (FULL_DATA) -> auto-select -> plan -> deploy -> ready -> client traffic -> clean decommission"
  exit 0
fi

log "step 3 — host mode: a public client port needs the per-port acknowledgement"
NOACK="$(make_req "$CLIENT_PORT" 0 baft-gf-e2e 127.0.0.1:1444 127.0.0.1:9202 127.0.0.1:9302 1)"
P0="$(post /api/tunnels/plan "$NOACK")"
python3 - "$P0" <<'PY' || fail "plan without the acknowledgement must FAIL its acknowledgement gate: $(printf '%s' "$P0" | head -c 1500)"
import json, sys
p = json.loads(sys.argv[1])
g = [x for x in p.get("gates") or [] if x.get("name") == "public port acknowledgement"]
sys.exit(0 if not p.get("ok") and g and g[0].get("status") == "FAIL" and "nginx" in g[0].get("detail", "") else 1)
PY
REQ="$(make_req "$CLIENT_PORT" 1 baft-gf-e2e 127.0.0.1:1444 127.0.0.1:9202 127.0.0.1:9302 1)"
HASH="$(plan_ok "$REQ" | jget hash)"

log "step 4 — deploy by plan hash; the acknowledged public port $CLIENT_PORT is queued for the IR"
T="$(deploy_active "$REQ" "$HASH")"
[[ "$(get "/api/tunnels?id=$T" | jget client_port_ack)" == True ]] || fail "the tunnel lost the reviewed acknowledgement"

log "step 5 — the IR applies an owned listener + owned rule; BCC's independent probe makes it Ready"
st=""
for _ in $(seq 1 60); do
  st="$(ingress_state "$T")"
  case "${st%% *}" in applied|failed|blocked) break ;; esac
  sleep 3
done
[[ "${st%% *}" == applied ]] || fail "public ingress did not apply: $st; $(journalctl -u baft-agent-gfir -n 40 --no-pager -o cat | grep -iE 'ingress|coverage|refus' | tail -n 5 | tr '\n' ' ')"
nft_has_port "$CLIENT_PORT" || fail "no owned rule for $CLIENT_PORT in inet baft ingress: $(nft list chain inet baft ingress 2>&1 | head -c 800)"
READY=""
for _ in $(seq 1 20); do
  RD="$(get "/api/ingress/readiness?tunnel_id=$T")"
  READY="$(printf '%s' "$RD" | jget ready)"
  [[ "$READY" == True ]] && break
  sleep 3
done
[[ "$READY" == True ]] || fail "BCC's independent probe did not verify $HOST_IP:$CLIENT_PORT: $(printf '%s' "$RD" | head -c 1200)"
st="$(ingress_state "$T")"; [[ "$st" == "applied | True |"* ]] || fail "tunnel view not Ready after verification: $st"

log "step 6 — a real client through the PUBLIC port $HOST_IP:$CLIENT_PORT, the tunnel and the EX service"
echo_through "$HOST_IP" "$CLIENT_PORT" "the public port"
if get /api/jobs | grep -qE 'BAFTPAIR1:|BAFTREPLY1:'; then fail "a pairing code is listed by /api/jobs"; fi

log "step 7 — decommission retires exactly the owned listener and rule"
decommission "$T"
closed_after "$HOST_IP" "$CLIENT_PORT" "public port"
closed_after 127.0.0.1 1444 "tunnel entry"
! nft_has_port "$CLIENT_PORT" || fail "the owned rule for $CLIENT_PORT survived decommission"
nft list chain inet baft ingress >/dev/null 2>&1 || fail "BAFT's own chain was removed (only the owned rule may go)"

log "step 8 — a port an Xray inbound is configured for ($XRAY_PORT) is refused before any effect"
REQX="$(make_req "$XRAY_PORT" 1 baft-gf-x 127.0.0.1:1445 127.0.0.1:9203 127.0.0.1:9303 0)"
HX="$(plan_ok "$REQX" | jget hash)"
TX="$(deploy_active "$REQX" "$HX")"
st=""
for _ in $(seq 1 60); do
  st="$(ingress_state "$TX")"
  case "${st%% *}" in applied|failed|blocked) break ;; esac
  sleep 3
done
[[ "${st%% *}" == failed && "$st" == *xray* ]] || fail "an Xray-claimed port was not refused with the detector's reason: $st"
! nft_has_port "$XRAY_PORT" || fail "a rule for the refused port $XRAY_PORT exists"
if python3 -c 'import socket,sys;socket.create_connection((sys.argv[1],int(sys.argv[2])),timeout=2)' "$HOST_IP" "$XRAY_PORT" 2>/dev/null; then
  fail "something listens on the refused port $XRAY_PORT"
fi
[[ -f "$XRAY_CFG" ]] && grep -q "\"port\":$XRAY_PORT" "$XRAY_CFG" || fail "the Xray config was modified"
decommission "$TX"
closed_after 127.0.0.1 1445 "tunnel entry"

log "PASS (host mode): host capability -> ack gate -> plan -> deploy -> owned public port Ready by independent probe -> client traffic via public port -> exact retire; Xray-claimed port refused"
