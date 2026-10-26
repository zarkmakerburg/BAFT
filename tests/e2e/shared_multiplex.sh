#!/usr/bin/env bash
# Real-binary check of shared transport multiplexing: two tunnels (two IR
# identities, two targets, the SAME route id) share ONE EX listener/port/process.
# The second IR is paired with `baft-pair ex-accept --join` and admitted with
# SIGHUP while the first tunnel is streaming; the EX is never restarted.
#
#   tests/e2e/shared_multiplex.sh        # builds into a temp dir
#   BIN_DIR=/path tests/e2e/shared_multiplex.sh
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
WORK="$(mktemp -d)"
PIDS=()
cleanup() {
  local rc=$?
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  if [[ $rc -ne 0 ]]; then
    for f in "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f"; tail -n 50 "$f"; }; done
  fi
  rm -rf "$WORK"
  exit $rc
}
trap cleanup EXIT
log() { printf '[e2e] %s\n' "$*"; }

if [[ -z "${BIN_DIR:-}" ]]; then
  BIN_DIR="$WORK/bin"
  go build -o "$BIN_DIR/" ./cmd/baft ./cmd/baft-pair
fi
BAFT="$BIN_DIR/baft"; PAIR="$BIN_DIR/baft-pair"
umask 077

# Bind every port at once so one run never hands the same port out twice
# (sequential bind/close lets the kernel reuse a port it just freed).
free_ports() { python3 -c 'import socket,sys
ss=[socket.socket() for _ in range(int(sys.argv[1]))]
for s in ss: s.bind(("127.0.0.1",0))
print(" ".join(str(s.getsockname()[1]) for s in ss))' "$1"; }
read -r EX_PORT T1 T2 IR1_PORT IR2_PORT EX_M IR1_M IR2_M <<<"$(free_ports 8)"
EX="$WORK/ex"; IR1="$WORK/ir1"; IR2="$WORK/ir2"; mkdir -p "$EX" "$IR1/state" "$IR2/state"

wait_port() {
  for _ in $(seq 1 100); do
    python3 -c "import socket;socket.create_connection(('127.0.0.1',$1),0.2)" 2>/dev/null && return 0
    sleep 0.1
  done
  echo "port $1 never opened"; return 1
}

pair_ir() { # name dir identity route-listen metrics psk-prefix -> prints reply
  local dir="$2" ident="$3" listen="$4" metrics="$5" tag="$6"
  "$PAIR" keygen --file "$dir/noise-key.json" >/dev/null
  CODE=$("$PAIR" ex-code --key "$EX/noise-key.json" --address "127.0.0.1:$EX_PORT" \
    --server-name 127.0.0.1 --identity urn:baft:node:ex-shared --ca-file "$EX/pki/ca.pem" \
    --psk-out "$EX/$tag.psk" --pending-out "$EX/$tag.ex.json" --ttl 5m)
  "$PAIR" ir-apply --code "$CODE" --key "$dir/noise-key.json" --state-dir "$dir/state" \
    --identity "$ident" --config-out "$dir/baft.yaml" --route-listen "127.0.0.1:$listen" \
    --metrics-listen "127.0.0.1:$metrics"
}

log "EX: keys, outer PKI; tenant 1 pairs normally"
"$PAIR" keygen --file "$EX/noise-key.json" >/dev/null
"$PAIR" pki --dir "$EX/pki" --host 127.0.0.1
REPLY1=$(pair_ir ir1 "$IR1" urn:baft:node:ir-one "$IR1_PORT" "$IR1_M" t1)
"$PAIR" ex-accept --reply "$REPLY1" --pending "$EX/t1.ex.json" --psk-file "$EX/t1.psk" \
  --key "$EX/noise-key.json" --listen "127.0.0.1:$EX_PORT" --ca-file "$EX/pki/ca.pem" \
  --cert-file "$EX/pki/server.pem" --cert-key-file "$EX/pki/server.key" \
  --target "127.0.0.1:$T1" --metrics-listen "127.0.0.1:$EX_M" \
  --unix-socket "$EX/admin.sock" --config-out "$EX/baft.yaml" >/dev/null
"$BAFT" config validate --file "$EX/baft.yaml"

log "start target 1, the EX and tenant 1"
python3 "$ROOT/tests/e2e/echo.py" serve "$T1" >"$WORK/t1.log" 2>&1 & PIDS+=($!)
python3 "$ROOT/tests/e2e/echo.py" serve "$T2" >"$WORK/t2.log" 2>&1 & PIDS+=($!)
"$BAFT" run --file "$EX/baft.yaml" >"$WORK/ex.log" 2>&1 & EX_PID=$!; PIDS+=($EX_PID)
wait_port "$EX_PORT"
"$BAFT" run --file "$IR1/baft.yaml" >"$WORK/ir1.log" 2>&1 & PIDS+=($!)
wait_port "$IR1_PORT"
python3 "$ROOT/tests/e2e/echo.py" send "$IR1_PORT" 2 2

log "tenant 1 streams (soak) while tenant 2 is added"
python3 "$ROOT/tests/e2e/echo.py" soak "$IR1_PORT" 8 >"$WORK/soak1.log" 2>&1 & SOAK=$!

log "pair tenant 2 into the SAME EX config (--join): no new listener, port or unit"
REPLY2=$(pair_ir ir2 "$IR2" urn:baft:node:ir-two "$IR2_PORT" "$IR2_M" t2)
cp "$EX/baft.yaml" "$WORK/ex.before.yaml"
"$PAIR" ex-accept --reply "$REPLY2" --pending "$EX/t2.ex.json" --psk-file "$EX/t2.psk" \
  --key "$EX/noise-key.json" --join "$EX/baft.yaml" --target "127.0.0.1:$T2" \
  --config-out "$EX/baft.yaml" >/dev/null
"$BAFT" config validate --file "$EX/baft.yaml"
python3 - "$EX/baft.yaml" "$WORK/ex.before.yaml" "$EX_PORT" <<'PY'
import json, sys
new, old = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
assert new["server"]["listen"] == old["server"]["listen"] and new["server"]["listen"].endswith(":" + sys.argv[3]), "listener changed"
assert len(new["server"]["allowed_peer_identities"]) == 2, new["server"]
ids = [r["id"] for r in new["routes"]]
assert len(ids) == 2 and ids[0] == ids[1], ids  # same route id, disjoint tenants
assert new["routes"][0] == old["routes"][0], "tenant 1 route changed"
PY

log "SIGHUP: EX admits tenant 2 without restarting"
kill -HUP "$EX_PID"
for _ in $(seq 1 50); do grep -q "tenants applied" "$WORK/ex.log" && break; sleep 0.1; done
grep -q "tenants applied" "$WORK/ex.log" || { echo "EX did not apply the tenant change"; exit 1; }
"$BAFT" run --file "$IR2/baft.yaml" >"$WORK/ir2.log" 2>&1 & PIDS+=($!)
wait_port "$IR2_PORT"

log "both tunnels carry traffic concurrently, checksums compared"
python3 "$ROOT/tests/e2e/echo.py" send "$IR2_PORT" 4 3 &
S2=$!
python3 "$ROOT/tests/e2e/echo.py" send "$IR1_PORT" 4 3
wait "$S2"
wait "$SOAK" || { echo "tenant 1's stream was interrupted"; cat "$WORK/soak1.log"; exit 1; }
cat "$WORK/soak1.log"
kill -0 "$EX_PID" || { echo "EX died"; exit 1; }
[[ "$(grep -c 'starting BAFT node' "$WORK/ex.log")" -eq 1 ]] || { echo "EX restarted"; exit 1; }

log "removing tenant 2's IR leaves tenant 1 up"
kill "${PIDS[-1]}"; sleep 0.5
python3 "$ROOT/tests/e2e/echo.py" send "$IR1_PORT" 2 2
log "PASS"
