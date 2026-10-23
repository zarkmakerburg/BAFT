#!/usr/bin/env bash
# Gamma P0 LAB: two isolated Linux network namespaces; NOT physical hosts.
# Never run on production. Needs root, iproute2, nftables and temporary binaries.
set -Eeuo pipefail
if [ "$#" -ne 1 ] || [ "$(id -u)" -ne 0 ]; then
  echo "usage: sudo bash gamma_two_node_namespace.sh /absolute/path/to/bin (LAB ONLY)" >&2
  exit 2
fi
BIN_DIR="$1"
BAFT="$BIN_DIR/baft"
PAIR="$BIN_DIR/baft-pair"
# Optional genuine older binaries for v0.3.7 -> candidate transport recovery.
# Only the first pair and live processes use the older binaries; the second
# start uses the exact candidate binaries, without changing any state file.
INITIAL_BIN_DIR="${GAMMA_INITIAL_BIN_DIR:-$BIN_DIR}"
INITIAL_BAFT="$INITIAL_BIN_DIR/baft"
INITIAL_PAIR="$INITIAL_BIN_DIR/baft-pair"
test -x "$BAFT" && test -x "$PAIR"
test -x "$INITIAL_BAFT" && test -x "$INITIAL_PAIR"
for tool in ip nft python3 curl sha256sum; do command -v "$tool" >/dev/null; done
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
chmod 700 "$WORK"
NS_EX="gamma-ex-$$"
NS_IR="gamma-ir-$$"
IF_EX="gex$$"
IF_IR="gir$$"
PIDS=""
cleanup() {
  local rc=$?
  trap - EXIT HUP INT TERM
  for pid in $PIDS; do kill "$pid" 2>/dev/null || true; done
  for pid in $PIDS; do wait "$pid" 2>/dev/null || true; done
  ip netns del "$NS_EX" 2>/dev/null || true
  ip netns del "$NS_IR" 2>/dev/null || true
  ip link del "$IF_EX" 2>/dev/null || true
  rm -rf -- "$WORK"
  exit "$rc"
}
trap cleanup EXIT HUP INT TERM
ip netns add "$NS_EX"
ip netns add "$NS_IR"
ip link add "$IF_EX" type veth peer name "$IF_IR"
ip link set "$IF_EX" netns "$NS_EX"
ip link set "$IF_IR" netns "$NS_IR"
for ns in "$NS_EX" "$NS_IR"; do ip -n "$ns" link set lo up; done
ip -n "$NS_EX" addr add 10.221.52.1/30 dev "$IF_EX"
ip -n "$NS_IR" addr add 10.221.52.2/30 dev "$IF_IR"
ip -n "$NS_EX" link set "$IF_EX" up
ip -n "$NS_IR" link set "$IF_IR" up
EX="$WORK/ex"
IR="$WORK/ir"
mkdir -m 700 -p "$EX" "$IR/state"
EX_PORT=18443
IR_PORT=15443
TARGET_PORT=15619

wait_port() {
  local ns="$1" host="$2" port="$3"
  local i
  for i in $(seq 1 80); do
    if ip netns exec "$ns" python3 -c \
       'import socket,sys;s=socket.create_connection((sys.argv[1],int(sys.argv[2])),0.2);s.close()' \
       "$host" "$port" 2>/dev/null; then return 0; fi
    sleep 0.1
  done
  echo "FAIL: listener not ready in namespace" >&2
  return 1
}

# A pre-existing IR service stays live while Pairing/BAFT is created/stopped.
ip netns exec "$NS_IR" python3 -m http.server 2053 --bind 10.221.52.2 \
  >"$WORK/unrelated2053.log" 2>&1 &
PIDS="$PIDS $!"
wait_port "$NS_IR" 10.221.52.2 2053
ip netns exec "$NS_EX" curl -fsS --max-time 3 -o /dev/null \
  http://10.221.52.2:2053/

# Model an IR SSH listener and reject NEW EX->IR TCP/22 using a scoped
# disposable nft rule. This models one-way reachability, not sshd/auth.
ip netns exec "$NS_IR" python3 -m http.server 22 --bind 10.221.52.2 \
  >"$WORK/ssh22-placeholder.log" 2>&1 &
PIDS="$PIDS $!"
wait_port "$NS_IR" 10.221.52.2 22
ip netns exec "$NS_IR" nft -f - <<EOF
table inet gamma_p0 {
  chain input {
    type filter hook input priority 0; policy accept;
    iifname "$IF_IR" tcp dport 22 drop
  }
}
EOF
if ip netns exec "$NS_EX" python3 -c \
    'import socket;socket.create_connection(("10.221.52.2",22),0.5)' \
    >/dev/null 2>&1; then
  echo "FAIL: blocked EX->IR port 22 unexpectedly reachable" >&2
  exit 1
fi
# A reachable unrelated port proves the negative SSH test is not route loss.
ip netns exec "$NS_EX" curl -fsS --max-time 3 -o /dev/null \
  http://10.221.52.2:2053/

"$INITIAL_PAIR" keygen --file "$EX/noise-key.json" >/dev/null
"$INITIAL_PAIR" pki --dir "$EX/pki" --host 10.221.52.1 >/dev/null
CODE=$("$INITIAL_PAIR" ex-code --key "$EX/noise-key.json" \
  --address 10.221.52.1:$EX_PORT --server-name 10.221.52.1 \
  --identity urn:baft:node:ex-gamma-lab --ca-file "$EX/pki/ca.pem" \
  --psk-out "$EX/pair.psk" --pending-out "$EX/pair.pending" --ttl 5m)
"$INITIAL_PAIR" keygen --file "$IR/noise-key.json" >/dev/null
REPLY=$("$INITIAL_PAIR" ir-apply --code "$CODE" --key "$IR/noise-key.json" \
  --state-dir "$IR/state" --config-out "$IR/baft.yaml" \
  --route-listen "127.0.0.1:$IR_PORT" --metrics-listen 127.0.0.1:18113)
"$INITIAL_PAIR" ex-accept --reply "$REPLY" --pending "$EX/pair.pending" \
  --psk-file "$EX/pair.psk" --key "$EX/noise-key.json" \
  --listen "10.221.52.1:$EX_PORT" --ca-file "$EX/pki/ca.pem" \
  --cert-file "$EX/pki/server.pem" --cert-key-file "$EX/pki/server.key" \
  --target "127.0.0.1:$TARGET_PORT" --metrics-listen 127.0.0.1:18112 \
  --unix-socket "$EX/admin.sock" --config-out "$EX/baft.yaml" >/dev/null
test ! -e "$EX/pair.psk" && test ! -e "$EX/pair.pending"
"$INITIAL_BAFT" config validate --file "$EX/baft.yaml" >/dev/null
"$INITIAL_BAFT" config validate --file "$IR/baft.yaml" >/dev/null
# Catch unsupported legacy config before stopping a working old tunnel.
"$BAFT" config validate --file "$EX/baft.yaml" >/dev/null
"$BAFT" config validate --file "$IR/baft.yaml" >/dev/null
BEFORE=$(sha256sum "$EX/baft.yaml" "$IR/baft.yaml" \
  "$EX/noise-key.json" "$IR/noise-key.json" | sha256sum)

ip netns exec "$NS_EX" python3 "$ROOT/tests/e2e/echo.py" serve "$TARGET_PORT" \
  >"$WORK/target.log" 2>&1 &
PIDS="$PIDS $!"
ip netns exec "$NS_EX" "$INITIAL_BAFT" run --file "$EX/baft.yaml" \
  >"$WORK/ex.log" 2>&1 &
EX_PID=$!
PIDS="$PIDS $EX_PID"
wait_port "$NS_EX" 10.221.52.1 "$EX_PORT"
ip netns exec "$NS_IR" "$INITIAL_BAFT" run --file "$IR/baft.yaml" \
  >"$WORK/ir.log" 2>&1 &
IR_PID=$!
PIDS="$PIDS $IR_PID"
wait_port "$NS_IR" 127.0.0.1 "$IR_PORT"

ip netns exec "$NS_IR" python3 "$ROOT/tests/e2e/echo.py" send "$IR_PORT" 1 2
kill -0 "$EX_PID"
kill -0 "$IR_PID"
AFTER=$(sha256sum "$EX/baft.yaml" "$IR/baft.yaml" \
  "$EX/noise-key.json" "$IR/noise-key.json" | sha256sum)
test "$BEFORE" = "$AFTER" || { echo "FAIL: config/key drift" >&2; exit 1; }
# Cold process restart: no new PairCode or regenerated key is permitted.
# This checks re-establishing NEW connections with the original pair, not
# continuity of a socket during a crash or systemd's restart policy.
kill "$IR_PID" "$EX_PID"
wait "$IR_PID" 2>/dev/null || true
wait "$EX_PID" 2>/dev/null || true
ip netns exec "$NS_EX" curl -fsS --max-time 3 -o /dev/null \
  http://10.221.52.2:2053/

ip netns exec "$NS_EX" "$BAFT" run --file "$EX/baft.yaml" \
  >"$WORK/ex-restart.log" 2>&1 &
EX_RESTART_PID=$!
PIDS="$PIDS $EX_RESTART_PID"
wait_port "$NS_EX" 10.221.52.1 "$EX_PORT"
ip netns exec "$NS_IR" "$BAFT" run --file "$IR/baft.yaml" \
  >"$WORK/ir-restart.log" 2>&1 &
IR_RESTART_PID=$!
PIDS="$PIDS $IR_RESTART_PID"
wait_port "$NS_IR" 127.0.0.1 "$IR_PORT"
ip netns exec "$NS_IR" python3 "$ROOT/tests/e2e/echo.py" send "$IR_PORT" 1 2
kill -0 "$EX_RESTART_PID"
kill -0 "$IR_RESTART_PID"
RESTART_HASH=$(sha256sum "$EX/baft.yaml" "$IR/baft.yaml" \
  "$EX/noise-key.json" "$IR/noise-key.json" | sha256sum)
test "$BEFORE" = "$RESTART_HASH" || { echo "FAIL: restart changed config/key" >&2; exit 1; }
if ip netns exec "$NS_EX" python3 -c \
    'import socket;socket.create_connection(("10.221.52.2",22),0.5)' \
    >/dev/null 2>&1; then
  echo "FAIL: EX->IR blocked port22 opened after restart" >&2
  exit 1
fi
ip netns exec "$NS_EX" curl -fsS --max-time 3 -o /dev/null \
  http://10.221.52.2:2053/
kill "$IR_RESTART_PID" "$EX_RESTART_PID"
wait "$IR_RESTART_PID" 2>/dev/null || true
wait "$EX_RESTART_PID" 2>/dev/null || true
ip netns exec "$NS_EX" curl -fsS --max-time 3 -o /dev/null \
  http://10.221.52.2:2053/
if [[ "$INITIAL_BIN_DIR" != "$BIN_DIR" ]]; then
  echo "PASS: genuine older EX/IR binaries upgraded to candidate with both configs/keys unchanged, 2x2x1MiB SHA transport, one-way SSH filter, IR:2053 preserved"
else
  echo "PASS: isolated Linux one-way pairing, 2x2x1MiB SHA transport including cold process restart, blocked EX->IR:22, unchanged configs/keys and preserved IR:2053"
fi
