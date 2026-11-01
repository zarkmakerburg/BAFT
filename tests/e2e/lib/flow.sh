# Shared BCC product-API helpers for the end-to-end LABs (guided_flow.sh,
# two_host_lab.sh). The caller defines: BCC (https origin), WORK, log, fail,
# and for make_req: SEL (auto-select evidence JSON), EX_PORT, TARGET_PORT.
# EX_NODE / IR_NODE default to the guided-flow node ids.
EX_NODE="${EX_NODE:-ex-gf}"
IR_NODE="${IR_NODE:-ir-gf}"

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

caps() { get /api/nodes | python3 -c 'import json,sys
for n in json.load(sys.stdin):
    if n.get("ID",n.get("id"))==sys.argv[1]: print(",".join(n.get("capabilities") or []))' "$1"; }

# make_req client_port ack instance route ex_metrics ir_metrics with_evidence
make_req() {
  EX_NODE="$EX_NODE" IR_NODE="$IR_NODE" python3 - "$SEL" "$EX_PORT" "$TARGET_PORT" "$@" <<'PY'
import json, os, sys
sel, ex_port, target, client, ack, inst, route, exm, irm, ev = sys.argv[1:11]
r = {"ex_node": os.environ["EX_NODE"], "ir_node": os.environ["IR_NODE"], "port": int(ex_port),
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
  # A check whose inspect result was lost (the agent restarted or the server
  # rebooted between running the job and acknowledging it) is only settled
  # by BCC's drift timeout (10 min), so wait past it, not just 2 minutes.
  for i in $(seq 1 230); do
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
