#!/usr/bin/env bash
# Two-host LAB: the v0.5.0 product path between TWO virtual machines.
#
# Two QEMU/KVM guests (separate kernels, systemd, firewalls and IP addresses)
# sit on a private bridge on this runner; the runner hosts BCC (verified
# HTTPS, throwaway CA) and plays the user's client. Each guest installs its
# agent with install.sh --agent-only from a signed release of this commit;
# the Iran guest enables safe host mode (--enable-host-ingress).
#
#   connection test (signed probe IR VM -> EX VM, FULL_DATA) -> auto-select
#   -> reviewed plan with the acknowledged public port -> deploy -> owned
#   listener + rule on the IR VM -> Ready by BCC's independent probe ->
#   client traffic from the runner through IR_IP:public port, the tunnel
#   across the bridge, and the service on the EX VM -> measured baseline
#   -> latency/jitter/loss (netem) -> link cut and recovery -> decommission
#   -> public port closed, owned rule gone.
#
# Scope: two real guests on one physical runner over an emulated network.
# It is not an Iran<->abroad Internet path.
#
#   sudo tests/e2e/two_host_lab.sh <release-dir from local_release.sh> <ubuntu cloud image (qcow2)>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
REL="$(cd "${1:?usage: two_host_lab.sh <release-dir> <cloud-image>}" && pwd)"
IMG="$(readlink -f "${2:?usage: two_host_lab.sh <release-dir> <cloud-image>}")"
for t in qemu-system-x86_64 qemu-img cloud-localds ssh scp ip tc iptables openssl update-ca-certificates python3 curl; do
  command -v "$t" >/dev/null || { echo "missing tool: $t"; exit 1; }
done
[[ -r /dev/kvm && -w /dev/kvm ]] || { echo "KVM is not available"; exit 1; }

WORK="$(mktemp -d)"; chmod 700 "$WORK"
BR=brbaft; HOST_IP=10.77.0.1; IR_IP=10.77.0.10; EX_IP=10.77.0.20
BCC="https://$HOST_IP:18300"
EX_NODE=ex-lab; IR_NODE=ir-lab
EX_PORT=18543; TARGET_PORT=2543; CLIENT_PORT=18610
E2E_CA=/usr/local/share/ca-certificates/baft-two-host-lab.crt
SSHO=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5 -i "$WORK/id")
LAST_STEP=start
SUMMARY="$WORK/summary.json"; echo '{}' >"$SUMMARY"

log() { LAST_STEP="$*"; printf '[two-host] %s\n' "$*"; }
fail() {
  echo "FAIL: $*" >&2
  [[ -n "${WORK:-}" ]] && printf '%s\n' "$*" >>"$WORK/fail-reason" 2>/dev/null || true
  [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '%s\n' "$*" | head -c 3000 | tr '\n' ' ' | sed 's/%/%25/g; s/^/::error title=FAIL::/; s/$/\n/'
  exit 1
}
record() { python3 - "$SUMMARY" "$1" "$2" <<'PY'
import json, sys
p, k, v = sys.argv[1:4]
d = json.load(open(p))
try: d[k] = json.loads(v)
except ValueError: d[k] = v
json.dump(d, open(p, "w"), indent=1)
PY
}
vm() { local ip="$1"; shift; ssh "${SSHO[@]}" "ubuntu@$ip" "sudo bash -c $(printf '%q' "$*")"; }
cleanup() {
  local rc=$?
  set +e
  if [[ $rc -ne 0 ]]; then
    [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '::error title=exit::rc=%s after step: %s\n' "$rc" "$(printf '%s' "$LAST_STEP" | head -c 300 | sed 's/%/%25/g')"
    for f in "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f"; tail -n 40 "$f"; }; done
    for ip in "$IR_IP" "$EX_IP"; do
      echo "== agent journal $ip"; vm "$ip" "journalctl -u baft-agent -n 60 --no-pager -o cat" 2>/dev/null
      echo "== baft journal $ip"; vm "$ip" "journalctl -u 'baft*' -n 30 --no-pager -o cat" 2>/dev/null | tail -n 30
    done
    echo "== tunnels"; curl -fsS -H 'Authorization: Bearer admintok' "$BCC/api/tunnels" 2>/dev/null | head -c 12000
    echo; echo "== nodes"; curl -fsS -H 'Authorization: Bearer admintok' "$BCC/api/nodes" 2>/dev/null | head -c 6000; echo
  fi
  [[ -f "$SUMMARY" ]] && { echo "== summary"; cat "$SUMMARY"; }
  for n in ir ex; do [[ -f "$WORK/$n.pid" ]] && kill "$(cat "$WORK/$n.pid")" 2>/dev/null; done
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done
  for n in ir ex; do ip link del "tap-$n" 2>/dev/null; done
  ip link del "$BR" 2>/dev/null
  iptables -D FORWARD -i "$BR" -j ACCEPT 2>/dev/null; iptables -D FORWARD -o "$BR" -j ACCEPT 2>/dev/null
  iptables -t nat -D POSTROUTING -s 10.77.0.0/24 ! -o "$BR" -j MASQUERADE 2>/dev/null
  [[ -f "$WORK/ca-installed" ]] && rm -f "$E2E_CA" && update-ca-certificates --fresh >/dev/null 2>&1
  exit "$rc"
}
PIDS=()
trap cleanup EXIT
trap 'fail "line $LINENO: unexpected failure of: $BASH_COMMAND. $(tail -n 3 "$WORK/fail-reason" 2>/dev/null | tr "\n" " ")"' ERR
# shellcheck source=tests/e2e/lib/flow.sh
source tests/e2e/lib/flow.sh

VERSION="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$REL/dist/manifest.json")"
COMMIT="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["commit"])' "$REL/dist/manifest.json")"
log "release $VERSION built from $COMMIT"
[[ "$COMMIT" == "$(git rev-parse HEAD)" ]] || fail "the release was not built from the checked-out commit"
record commit "\"$COMMIT\""

# ---------------------------------------------------------------- network --
log "private bridge $BR $HOST_IP/24 with one tap per guest (NAT only for package installs)"
ip link add "$BR" type bridge && ip addr add "$HOST_IP/24" dev "$BR" && ip link set "$BR" up
for n in ir ex; do ip tuntap add dev "tap-$n" mode tap && ip link set "tap-$n" master "$BR" && ip link set "tap-$n" up; done
sysctl -qw net.ipv4.ip_forward=1
# The runner's Docker sets FORWARD to DROP and may filter bridged traffic.
iptables -I FORWARD -i "$BR" -j ACCEPT; iptables -I FORWARD -o "$BR" -j ACCEPT
iptables -t nat -A POSTROUTING -s 10.77.0.0/24 ! -o "$BR" -j MASQUERADE

# -------------------------------------------------------------------- TLS --
log "BCC TLS: throwaway CA, certificate for $HOST_IP"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj "/CN=BAFT two-host LAB CA" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign" \
  -keyout "$WORK/ca.key" -out "$WORK/ca.crt" >/dev/null 2>&1
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=$HOST_IP" -keyout "$WORK/bcc.key" -out "$WORK/bcc.csr" >/dev/null 2>&1
printf 'subjectAltName=IP:%s\nextendedKeyUsage=serverAuth\nbasicConstraints=critical,CA:FALSE\n' "$HOST_IP" >"$WORK/bcc.ext"
openssl x509 -req -in "$WORK/bcc.csr" -CA "$WORK/ca.crt" -CAkey "$WORK/ca.key" -CAcreateserial -days 1 -extfile "$WORK/bcc.ext" -out "$WORK/bcc.crt" >/dev/null 2>&1
chmod 600 "$WORK/bcc.key"
[[ -e "$E2E_CA" ]] && fail "refusing to overwrite $E2E_CA"
install -m 0644 "$WORK/ca.crt" "$E2E_CA" && touch "$WORK/ca-installed" && update-ca-certificates >/dev/null 2>&1

# ----------------------------------------------------------------- guests --
log "boot two guests (KVM) from $(basename "$IMG")"
ssh-keygen -q -t ed25519 -N '' -f "$WORK/id"
boot() { # name ip mac
  local n="$1" ip="$2" mac="$3"
  qemu-img create -q -f qcow2 -F qcow2 -b "$IMG" "$WORK/$n.qcow2" 12G
  cat >"$WORK/$n-user-data" <<EOF
#cloud-config
hostname: $n-lab
ssh_authorized_keys:
  - $(cat "$WORK/id.pub")
write_files:
  - path: /usr/local/share/ca-certificates/baft-two-host-lab.crt
    permissions: '0644'
    encoding: b64
    content: $(base64 -w0 "$WORK/ca.crt")
runcmd:
  - [update-ca-certificates]
EOF
  cat >"$WORK/$n-network" <<EOF
version: 2
ethernets:
  lab0:
    match: {macaddress: "$mac"}
    set-name: lab0
    addresses: [$ip/24]
    routes: [{to: default, via: $HOST_IP}]
    nameservers: {addresses: [1.1.1.1, 8.8.8.8]}
EOF
  printf 'instance-id: %s-lab\nlocal-hostname: %s-lab\n' "$n" "$n" >"$WORK/$n-meta"
  cloud-localds -N "$WORK/$n-network" "$WORK/$n-seed.img" "$WORK/$n-user-data" "$WORK/$n-meta"
  qemu-system-x86_64 -enable-kvm -cpu host -m 2048 -smp 2 -display none -daemonize -pidfile "$WORK/$n.pid" \
    -serial "file:$WORK/$n-console.log" \
    -drive "file=$WORK/$n.qcow2,if=virtio" -drive "file=$WORK/$n-seed.img,if=virtio,format=raw" \
    -netdev "tap,id=n0,ifname=tap-$n,script=no,downscript=no" -device "virtio-net-pci,netdev=n0,mac=$mac"
}
boot ir "$IR_IP" 52:54:00:77:00:10
boot ex "$EX_IP" 52:54:00:77:00:20
for ip in "$IR_IP" "$EX_IP"; do
  ok=0
  for _ in $(seq 1 90); do ssh "${SSHO[@]}" "ubuntu@$ip" true 2>/dev/null && { ok=1; break; }; sleep 3; done
  [[ "$ok" == 1 ]] || fail "guest $ip never accepted SSH: $(tail -n 20 "$WORK"/*-console.log | tr '\n' ' ' | tail -c 1500)"
  vm "$ip" "cloud-init status --wait >/dev/null; true"
done
record guests "{\"ir\":\"$(vm "$IR_IP" 'uname -r; cat /proc/sys/kernel/random/boot_id' | tr '\n' ' ')\",\"ex\":\"$(vm "$EX_IP" 'uname -r; cat /proc/sys/kernel/random/boot_id' | tr '\n' ' ')\"}"
[[ "$(vm "$IR_IP" 'cat /proc/sys/kernel/random/boot_id')" != "$(vm "$EX_IP" 'cat /proc/sys/kernel/random/boot_id')" ]] || fail "guests share a kernel"
for ip in "$IR_IP" "$EX_IP"; do
  vm "$ip" "command -v nft >/dev/null || { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables >/dev/null; }"
done

# Self-test of the real-servers driver: run tests/e2e/real_hosts.sh against
# the two guests exactly as it runs against the owner's servers (SSH, sudo,
# BCC on the foreign host, reboot persistence), so the driver is proven before
# anyone spends real servers on it. The guests stand in for the servers only;
# this is still not an Iran<->abroad path.
if [[ "${REAL_HOSTS_SELFTEST:-0}" == 1 ]]; then
  # The owner's foreign server already runs a production BAFT (and 3X-UI).
  # Simulate that on the EX guest: the standard BAFT binary, configuration
  # and a running baft.service holding port 8443. The driver must coexist,
  # skip the busy port, and leave all of it byte-for-byte unchanged.
  log "self-test: simulate a production BAFT on the EX guest (baft.service on 8443, standard paths)"
  scp "${SSHO[@]}" -q "$REL/dist/baft-linux-amd64" tests/e2e/echo.py "ubuntu@$EX_IP:/tmp/"
  vm "$EX_IP" "set -e; install -m 0755 /tmp/baft-linux-amd64 /usr/local/bin/baft; install -d -m 0755 /etc/baft /usr/local/lib/baft-prodsim
    printf 'prodsim: true\\n' >/etc/baft/baft.yaml; install -m 0644 /tmp/echo.py /usr/local/lib/baft-prodsim/echo.py
    install -m 0755 \"\$(readlink -f /usr/bin/python3)\" /usr/local/lib/baft-prodsim/baft
    printf '[Unit]\\nDescription=simulated production BAFT\\n[Service]\\nExecStart=/usr/local/lib/baft-prodsim/baft /usr/local/lib/baft-prodsim/echo.py serve 8443\\n[Install]\\nWantedBy=multi-user.target\\n' >/etc/systemd/system/baft.service
    systemctl daemon-reload; systemctl enable --now baft.service >/dev/null 2>&1
    for i in \$(seq 1 20); do ss -Htln | grep -q ':8443 ' && break; sleep 0.5; done; ss -Htlnp | grep ':8443 '" || fail "could not simulate the production BAFT"
  PROD_SIM_BEFORE="$(vm "$EX_IP" "sha256sum /usr/local/bin/baft /etc/baft/baft.yaml /etc/systemd/system/baft.service; systemctl is-active baft.service")"
  log "self-test: tests/e2e/real_hosts.sh MODE=full REBOOT=1 (Iran guest clean, EX guest coexisting)"
  MODE=full REL="$REL" IR_HOST="$IR_IP" EX_HOST="$EX_IP" SSH_KEY_FILE="$WORK/id" SSH_USER=ubuntu REBOOT=1 \
    SUMMARY_OUT="$WORK/real-hosts-summary.json" GITHUB_STEP_SUMMARY="${GITHUB_STEP_SUMMARY:-}" \
    tests/e2e/real_hosts.sh || fail "real_hosts.sh self-test failed"
  python3 - "$WORK/real-hosts-summary.json" <<'PY' || fail "the self-test did not prove coexistence with the simulated production BAFT"
import json, sys
d = json.load(open(sys.argv[1]))
assert d.get("host_modes", {}).get("ex") == "coexist", d.get("host_modes")
assert d.get("production_unchanged") is True, d.get("production_diff")
assert "8443" in d.get("candidate_ports", {}).get("dropped_busy_on_foreign", ""), d.get("candidate_ports")
assert not d.get("lab_leftovers_foreign"), d.get("lab_leftovers_foreign")
PY
  PROD_SIM_AFTER="$(vm "$EX_IP" "sha256sum /usr/local/bin/baft /etc/baft/baft.yaml /etc/systemd/system/baft.service; systemctl is-active baft.service")"
  [[ "$PROD_SIM_BEFORE" == "$PROD_SIM_AFTER" ]] || fail "the simulated production BAFT changed: before=[$PROD_SIM_BEFORE] after=[$PROD_SIM_AFTER]"
  vm "$EX_IP" "ss -Htlnp | grep -q ':8443 .*\\\"baft\\\"'" || fail "the simulated production listener on 8443 is gone"
  log "self-test: simulated production BAFT unchanged and still serving 8443"
  cp "$WORK/real-hosts-summary.json" "$SUMMARY"
  cp "$SUMMARY" "${TWO_HOST_SUMMARY_OUT:-/dev/null}" 2>/dev/null || true
  log "PASS: real_hosts.sh drove the full product path on two guests, including reboot persistence"
  exit 0
fi

# -------------------------------------------------------------------- BCC --
log "BCC (the release's own baft-bcc) on $BCC; release mirror for agents"
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; esac
mkdir -p "$WORK/www/$VERSION"; cp "$REL"/dist/* "$WORK/www/$VERSION/"; cp "$REL/revocations.json" "$WORK/www/"
(cd "$WORK/www" && exec python3 -m http.server 18301 --bind "$HOST_IP") >"$WORK/www.log" 2>&1 & PIDS+=($!)
BCC_BIN="$WORK/baft-bcc"; install -m 0755 "$REL/dist/baft-bcc-linux-$ARCH" "$BCC_BIN"
echo admintok >"$WORK/admin.token"; chmod 600 "$WORK/admin.token"
"$BCC_BIN" access init --access-file "$WORK/s.json.access.json" >/dev/null
TOK_EX="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"; TOK_IR="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
# A non-loopback BCC listener must be explicitly allowlisted (production rule).
BAFT_BCC_ALLOWED_LISTEN_IPS="$HOST_IP" TOK_EX="$TOK_EX" TOK_IR="$TOK_IR" "$BCC_BIN" --admin-token-file "$WORK/admin.token" --state-file "$WORK/s.json" \
  --listen "$HOST_IP:18300" --tls-cert "$WORK/bcc.crt" --tls-key "$WORK/bcc.key" >"$WORK/bcc.log" 2>&1 & PIDS+=($!)
for _ in $(seq 1 30); do api -o /dev/null "$BCC/api/nodes" 2>/dev/null && break; sleep 1; done
api -o /dev/null "$BCC/api/nodes" || fail "BCC is not reachable over verified HTTPS"
JOB_KEY="$("$BCC_BIN" jobkey show --job-key-file "$WORK/s.json.job-key")"
post /api/nodes "{\"ID\":\"$EX_NODE\",\"Alias\":\"EX VM\",\"Address\":\"$EX_IP:22\",\"Role\":\"foreign\",\"AgentTokenEnv\":\"TOK_EX\"}" >/dev/null
post /api/nodes "{\"ID\":\"$IR_NODE\",\"Alias\":\"IR VM\",\"Address\":\"$IR_IP:22\",\"Role\":\"worker\",\"AgentTokenEnv\":\"TOK_IR\"}" >/dev/null

log "install.sh --agent-only inside each guest (IR: --enable-host-ingress)"
tar -C "$REL" -czf "$WORK/rel.tgz" dist root.pub revocations.json
install_guest() { # ip node token flags...
  local ip="$1" node="$2" tok="$3"; shift 3
  scp "${SSHO[@]}" -q "$WORK/rel.tgz" install.sh tests/e2e/echo.py "ubuntu@$ip:/tmp/"
  printf '%s\n' "$tok" | ssh "${SSHO[@]}" "ubuntu@$ip" "umask 077; cat >/tmp/agent.token"
  vm "$ip" "set -e; rm -rf /opt/lab; mkdir -p /opt/lab; tar -C /opt/lab -xzf /tmp/rel.tgz; install -m 0600 /tmp/agent.token /opt/lab/agent.token; rm -f /tmp/agent.token
    env BAFT_INSTALL_FROM=release BAFT_RELEASE_URL=file:///opt/lab/dist BAFT_ROOT_PUB=\"\$(cat /opt/lab/root.pub)\" \
      BAFT_REVOCATIONS_URL=file:///opt/lab/revocations.json BAFT_BCC_JOB_KEY='$JOB_KEY' BAFT_AGENT_TOKEN_FILE=/opt/lab/agent.token \
      BAFT_AGENT_RELEASE_BASE_URL=http://$HOST_IP:18301 BAFT_AGENT_REVOCATIONS_URL=http://$HOST_IP:18301/revocations.json \
      BAFT_AGENT_INTERVAL=2s bash /tmp/install.sh --agent-only --bcc-url '$BCC' --node-id '$node' $* >/opt/lab/install.log 2>&1 \
      || { tail -n 15 /opt/lab/install.log; exit 1; }
    systemctl is-active baft-agent >/dev/null" || fail "install.sh failed on $ip"
}
install_guest "$EX_IP" "$EX_NODE" "$TOK_EX" --no-ingress-chain
install_guest "$IR_IP" "$IR_NODE" "$TOK_IR" --enable-host-ingress
for _ in $(seq 1 60); do
  IRC="$(caps "$IR_NODE")"; EXC="$(caps "$EX_NODE")"
  [[ ",$IRC," == *",operational-ingress-host-v1,"* && -n "$EXC" ]] && break
  sleep 3
done
log "IR capabilities: ${IRC:-none}; EX capabilities: ${EXC:-none}"
[[ ",$IRC," == *",operational-ingress-host-v1,"* ]] || fail "IR guest does not advertise safe host mode: $IRC; $(vm "$IR_IP" 'journalctl -u baft-agent -n 30 --no-pager -o cat' | grep -i ingress | tail -n 3 | tr '\n' ' ')"
[[ ",$IRC," != *",operational-ingress-v1,"* ]] || fail "IR guest claims the LAB capability"

vm "$EX_IP" "systemd-run --unit=lab-echo --collect python3 /tmp/echo.py serve $TARGET_PORT >/dev/null"

# ------------------------------------------------------------ product path --
log "step 1 — connection test: signed probe IR guest -> EX guest on $EX_PORT"
PROBE="$(post /api/path-probes "{\"source_node\":\"$IR_NODE\",\"destination_node\":\"$EX_NODE\",\"family\":\"4\",\"candidate_ports\":[$EX_PORT]}")"
PID="$(printf '%s' "$PROBE" | jget id)"
for _ in $(seq 1 90); do
  PROBE="$(get "/api/path-probes?id=$PID")"
  case "$(printf '%s' "$PROBE" | jget phase)" in complete|failed|cancelled) break ;; esac
  sleep 3
done
[[ "$(printf '%s' "$PROBE" | jget phase)" == complete && "$(printf '%s' "$PROBE" | jget candidates 0 class)" == FULL_DATA ]] \
  || fail "connection test across the guests did not prove FULL_DATA: $(printf '%s' "$PROBE" | head -c 1500)"
record probe "$(printf '%s' "$PROBE" | python3 -c 'import json,sys;c=json.load(sys.stdin)["candidates"][0];print(json.dumps({k:c.get(k) for k in ("port","class","connect_rtt_ms","connect_spread_ms","connect_successes","attempts","ack_successes","throughput_kbps","throughput_at_least")}))')"

# The agent's own speed evidence for the probe. /api/jobs deliberately wipes
# job output, so read the run job read-only from BCC's state database.
RUNJOB="$(printf '%s' "$PROBE" | jget candidates 0 run_job_id)"
record probe_speed "$(python3 - "$WORK/s.json" "$RUNJOB" <<'PY'
import json, sqlite3, sys
db = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
row = db.execute("SELECT doc FROM jobs WHERE id = ?", (sys.argv[2],)).fetchone()
if not row:
    print(json.dumps({"run_job": sys.argv[2], "found": False})); sys.exit()
j = json.loads(row[0])
out = {"requested": "speed_seconds" in (j.get("params") or {}), "status": j.get("status")}
try:
    ev = json.loads(j.get("output") or "{}")
    out["speed"] = ev.get("speed")
    out["bulk_connect_ms"] = [f.get("connect_ms") for f in ev.get("bulk") or []]
except ValueError:
    out["output_head"] = (j.get("output") or "")[:300]
print(json.dumps(out))
PY
)"
log "probe speed evidence: $(python3 -c 'import json,sys;print(json.dumps(json.load(open(sys.argv[1])).get("probe_speed")))' "$SUMMARY")"
# Both agents advertise path-probe-speed-v1, so a healthy path must report a
# speed (an estimate, or a floor when the flow hit its byte cap).
[[ "$(printf '%s' "$PROBE" | jget candidates 0 throughput_kbps)" =~ ^[1-9][0-9]*$ ]] \
  || fail "the connection test reported no speed although both agents support it: $(printf '%s' "$PROBE" | head -c 800)"

log "step 2 — evidence-checked auto-select"
SEL=""
for _ in $(seq 1 15); do
  SEL="$(post /api/tunnels/auto-select "{\"ex_node\":\"$EX_NODE\",\"ir_node\":\"$IR_NODE\",\"family\":\"4\",\"carrier\":\"h2\"}" | jget selected)"
  [[ -n "$SEL" ]] && break; sleep 2
done
[[ -n "$SEL" ]] || fail "auto-select refused fresh evidence"
EX_PORT="$(printf '%s' "$SEL" | jget port)"
log "auto-select chose EX port $EX_PORT"

log "step 3 — reviewed plan with the acknowledged public port $CLIENT_PORT; deploy"
REQ="$(make_req "$CLIENT_PORT" 1 baft-lab-1 127.0.0.1:1444 127.0.0.1:9202 127.0.0.1:9302 1)"
HASH="$(plan_ok "$REQ" | jget hash)"
T="$(deploy_active "$REQ" "$HASH")"
record tunnel "\"$T\""

log "step 4 — owned listener + rule on the IR guest; Ready by BCC's independent probe"
st=""
for _ in $(seq 1 60); do st="$(ingress_state "$T")"; case "${st%% *}" in applied|failed|blocked) break ;; esac; sleep 3; done
[[ "${st%% *}" == applied ]] || fail "public ingress did not apply on the IR guest: $st"
READY=""
for _ in $(seq 1 20); do RD="$(get "/api/ingress/readiness?tunnel_id=$T")"; READY="$(printf '%s' "$RD" | jget ready)"; [[ "$READY" == True ]] && break; sleep 3; done
[[ "$READY" == True ]] || fail "BCC did not verify $IR_IP:$CLIENT_PORT: $(printf '%s' "$RD" | head -c 1200)"
vm "$IR_IP" "nft -j list chain inet baft ingress" | python3 -c 'import json,sys
port=int(sys.argv[1])
def walk(x):
    if isinstance(x,dict): return x.get("right")==port or any(walk(v) for v in x.values())
    if isinstance(x,list): return any(walk(v) for v in x)
    return False
sys.exit(0 if any(walk(o["rule"].get("expr")) for o in json.load(sys.stdin)["nftables"] if "rule" in o) else 1)' "$CLIENT_PORT" \
  || fail "no owned rule for $CLIENT_PORT in the IR guest's inet baft ingress chain"

measure() { # label MiB timeout_s -> JSON {integrity, mib, seconds, mbps_echo, rtt_ms_*}
  python3 - "$IR_IP" "$CLIENT_PORT" "$1" "${2:-16}" "${3:-120}" <<'PY'
import json, os, socket, statistics, sys, time, threading
host, port, label = sys.argv[1], int(sys.argv[2]), sys.argv[3]
size, limit = int(sys.argv[4]) * 1024 * 1024, float(sys.argv[5])
data = os.urandom(size)
got = bytearray()
t0 = time.monotonic()
with socket.create_connection((host, port), timeout=60) as s:
    s.settimeout(limit)
    def send():
        s.sendall(data); s.shutdown(socket.SHUT_WR)
    th = threading.Thread(target=send); th.start()
    while True:
        b = s.recv(1 << 16)
        if not b: break
        got += b
    th.join()
secs = time.monotonic() - t0
rtts = []
with socket.create_connection((host, port), timeout=30) as s:
    s.settimeout(30)
    for i in range(40):
        msg = b"%08d" % i
        t = time.monotonic(); s.sendall(msg)
        r = b""
        while len(r) < len(msg):
            r += s.recv(64)
        rtts.append((time.monotonic() - t) * 1000)
rtts.sort()
print(json.dumps({"label": label, "integrity": bytes(got) == data, "mib": size >> 20, "seconds": round(secs, 3),
    "mbps_echo": round(size * 8 / secs / 1e6, 2), "rtt_ms_avg": round(statistics.mean(rtts), 2),
    "rtt_ms_p50": round(rtts[len(rtts)//2], 2), "rtt_ms_p95": round(rtts[int(len(rtts)*0.95)-1], 2)}))
PY
}
log "step 5 — real client from the runner through $IR_IP:$CLIENT_PORT -> tunnel -> EX guest service (baseline)"
M="$(measure baseline 16 120)" || fail "client traffic through the public port failed"
log "baseline: $M"; record baseline "$M"
[[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "baseline data mismatch"

# Each guest link adds its delay once per direction it carries: the tunnel
# (IR<->EX) crosses both links, the client leg crosses the IR link, so the
# end-to-end RTT grows by ~3x the per-link delay. Jitter reorders packets
# (worst case for TCP inside TCP); the volume is sized for that.
log "step 6 — impaired network: netem 40±10 ms delay and 1% loss on both guest links"
for n in ir ex; do tc qdisc add dev "tap-$n" root netem delay 40ms 10ms distribution normal loss 1%; done
M="$(measure netem_40ms_10ms_1pct 2 300)" || fail "client traffic under delay/loss failed"
log "netem: $M"; record netem "$M"
[[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "data mismatch under delay/loss"
for n in ir ex; do tc qdisc del dev "tap-$n" root; done

log "step 7 — link cut: EX guest offline for 20 s, then the tunnel must recover"
ip link set tap-ex down; sleep 20; ip link set tap-ex up
t0=$(date +%s); recovered=""
for _ in $(seq 1 90); do
  if python3 - "$IR_IP" "$CLIENT_PORT" <<'PY' 2>/dev/null
import os, socket, sys
d = os.urandom(65536)
with socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=5) as s:
    s.settimeout(10); s.sendall(d); s.shutdown(socket.SHUT_WR)
    g = bytearray()
    while True:
        b = s.recv(65536)
        if not b: break
        g += b
sys.exit(0 if bytes(g) == d else 1)
PY
  then recovered=$(( $(date +%s) - t0 )); break; fi
  sleep 2
done
[[ -n "$recovered" ]] || fail "the tunnel did not recover within 180 s after the EX link came back"
log "recovered ${recovered}s after the link came back"; record link_cut "{\"offline_s\":20,\"recovered_after_s\":$recovered}"
M="$(measure after_recovery 16 120)" || fail "traffic after recovery failed"
record after_recovery "$M"; [[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "data mismatch after recovery"

log "step 8 — decommission: public port closed, owned rule removed, BAFT's chain kept"
decommission "$T"
closed_after "$IR_IP" "$CLIENT_PORT" "public port"
vm "$IR_IP" "nft list chain inet baft ingress >/dev/null" || fail "BAFT's own chain was removed on the IR guest"
if vm "$IR_IP" "nft -j list chain inet baft ingress" | grep -q "\"right\": *$CLIENT_PORT[,}]"; then fail "owned rule survived decommission"; fi
record result '"PASS"'

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  { echo "### Two-host LAB ($COMMIT)"; echo; echo '```json'; cat "$SUMMARY"; echo '```'; } >>"$GITHUB_STEP_SUMMARY"
fi
cp "$SUMMARY" "${TWO_HOST_SUMMARY_OUT:-/dev/null}" 2>/dev/null || true
log "PASS: two guests -> FULL_DATA -> plan -> deploy -> owned public port Ready -> client traffic (baseline, netem, link cut + recovery) -> exact decommission"
