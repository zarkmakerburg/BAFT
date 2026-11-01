#!/usr/bin/env bash
# Real-hosts LAB: the v0.5.0 product path between two REAL servers the owner
# provides (one in Iran, one abroad), driven over SSH from a GitHub runner.
#
# Unlike two_host_lab.sh (two KVM guests on one runner, emulated network),
# this crosses the real Iran<->abroad Internet path. BCC runs on the foreign
# server (verified HTTPS, throwaway CA); both agents are installed from a
# signed release of this commit; the Iran server enables safe host mode.
#
#   MODE=preflight  read-only: reachability (direct or through the foreign
#                   server), OS/kernel/systemd, nft, existing BAFT, listeners
#                   on the planned ports, Xray/3X-UI/nginx/HAProxy/Docker.
#                   Changes nothing on either server.
#   MODE=full       preflight, then: install -> connection test (FULL_DATA)
#                   -> auto-select -> reviewed plan with the acknowledged
#                   public port -> deploy -> owned listener + rule -> Ready by
#                   BCC's independent probe -> real client traffic -> agent
#                   restarts -> firewall drop (Apply is not Ready) -> tunnel
#                   path cut and recovery -> [optional reboot persistence] ->
#                   exact decommission -> full uninstall and cleanup.
#
# The servers must be TEST servers without real users: full mode installs and
# removes BAFT, restarts agents, drops traffic on the planned ports and can
# reboot the Iran server (REBOOT=1).
#
# Required environment:
#   IR_HOST EX_HOST     public IPv4 addresses (never printed; GitHub masks them)
#   SSH_KEY_FILE        private key accepted by both servers
# Optional:
#   SSH_USER=root SSH_PORT=22 SSH_KNOWN_HOSTS_FILE (strict host keys when set)
#   MODE=preflight|full  REL=<release dir from local_release.sh> (full only)
#   CANDIDATE_PORTS=443,8443,2053  CLIENT_PORT=18610  BCC_PORT=18300
#   MIRROR_PORT=18301  REBOOT=0|1  SUMMARY_OUT=<file>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."

MODE="${MODE:-preflight}"
: "${IR_HOST:?IR_HOST is required}" "${EX_HOST:?EX_HOST is required}" "${SSH_KEY_FILE:?SSH_KEY_FILE is required}"
SSH_USER="${SSH_USER:-root}"; SSH_PORT="${SSH_PORT:-22}"
CANDIDATE_PORTS="${CANDIDATE_PORTS:-443,8443,2053}"; CLIENT_PORT="${CLIENT_PORT:-18610}"
BCC_PORT="${BCC_PORT:-18300}"; MIRROR_PORT="${MIRROR_PORT:-18301}"; REBOOT="${REBOOT:-0}"
TARGET_PORT=2543
EX_NODE=ex-real; IR_NODE=ir-real
LAB=/opt/baft-real-hosts
LAB_CA=/usr/local/share/ca-certificates/baft-real-hosts.crt
case "$MODE" in preflight|full) ;; *) echo "MODE must be preflight or full"; exit 2 ;; esac
for p in $CLIENT_PORT $BCC_PORT $MIRROR_PORT ${CANDIDATE_PORTS//,/ }; do
  [[ "$p" =~ ^[0-9]+$ && "$p" -ge 1 && "$p" -le 65535 ]] || { echo "invalid port: $p"; exit 2; }
  [[ "$p" != "$SSH_PORT" && "$p" != 22 ]] || { echo "port $p is reserved for SSH management"; exit 2; }
done
for t in ssh scp python3 curl openssl; do command -v "$t" >/dev/null || { echo "missing tool: $t"; exit 1; }; done

WORK="$(mktemp -d)"; chmod 700 "$WORK"
LAST_STEP=start; SUMMARY="$WORK/summary.json"; echo '{}' >"$SUMMARY"
INSTALLED_EX=0; INSTALLED_IR=0; TUNNEL=""; DECOMMISSIONED=0; FWLAB_IR=0; FWLAB_CUT=0
PIDS=()

log() { LAST_STEP="$*"; printf '[real-hosts] %s\n' "$*"; }
fail() {
  echo "FAIL: $*" >&2
  printf '%s\n' "$*" >>"$WORK/fail-reason" 2>/dev/null || true
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

# ------------------------------------------------------------------- SSH --
install -m 0600 "$SSH_KEY_FILE" "$WORK/id"
if [[ -n "${SSH_KNOWN_HOSTS_FILE:-}" && -s "${SSH_KNOWN_HOSTS_FILE}" ]]; then
  install -m 0600 "$SSH_KNOWN_HOSTS_FILE" "$WORK/known_hosts"; HOSTKEY=yes
  record host_keys '"pinned"'
else
  : >"$WORK/known_hosts"; HOSTKEY=accept-new
  log "WARNING: no pinned host keys (SSH_KNOWN_HOSTS); first-use keys are accepted and their fingerprints recorded"
  record host_keys '"first-use"'
fi
COMMON=(-i "$WORK/id" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking="$HOSTKEY"
        -o UserKnownHostsFile="$WORK/known_hosts" -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o LogLevel=ERROR)
SSHO=("${COMMON[@]}" -p "$SSH_PORT"); SCPO=("${COMMON[@]}" -P "$SSH_PORT")
JUMP=("-o" "ProxyCommand=ssh -i $WORK/id -p $SSH_PORT -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=$HOSTKEY -o UserKnownHostsFile=$WORK/known_hosts -o ConnectTimeout=15 -o LogLevel=ERROR -W %h:%p $SSH_USER@$EX_HOST")
IR_VIA=()   # set to JUMP when the runner cannot reach the Iran server directly
SUDO=""; [[ "$SSH_USER" == root ]] || SUDO="sudo -n"
on() { # ex|ir command... (runs as root)
  local h="$1"; shift
  if [[ "$h" == ex ]]; then ssh "${SSHO[@]}" "$SSH_USER@$EX_HOST" "$SUDO bash -c $(printf '%q' "$*")"
  else ssh "${SSHO[@]}" "${IR_VIA[@]}" "$SSH_USER@$IR_HOST" "$SUDO bash -c $(printf '%q' "$*")"; fi
}
STAGE="/tmp/baft-real-hosts-stage-$$"
put() { # ex|ir remote-dir files...  (the login user may not be root: copy to
  # a private staging directory it owns, then move into place as root)
  local h="$1" dir="$2" host="$EX_HOST" via=(); shift 2
  [[ "$h" == ir ]] && { host="$IR_HOST"; via=("${IR_VIA[@]}"); }
  ssh "${SSHO[@]}" "${via[@]}" "$SSH_USER@$host" "rm -rf $STAGE && mkdir -m 700 $STAGE"
  scp -q "${SCPO[@]}" "${via[@]}" "$@" "$SSH_USER@$host:$STAGE/"
  on "$h" "mv -f $STAGE/* $dir/ && rm -rf $STAGE"
}

cleanup() {
  local rc=$?
  set +e
  trap - ERR
  if [[ $rc -ne 0 ]]; then
    [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '::error title=exit::rc=%s after step: %s\n' "$rc" "$(printf '%s' "$LAST_STEP" | head -c 300 | sed 's/%/%25/g')"
    for h in ir ex; do
      [[ "$h" == ir && "$INSTALLED_IR" == 0 ]] && continue
      [[ "$h" == ex && "$INSTALLED_EX" == 0 ]] && continue
      echo "== agent journal $h"; on "$h" "journalctl -u baft-agent -n 60 --no-pager -o cat" 2>/dev/null | tail -n 60
    done
    [[ "$INSTALLED_EX" == 1 ]] && { echo "== BCC journal"; on ex "journalctl -u baft-realhosts-bcc -n 40 --no-pager -o cat" 2>/dev/null; }
    [[ -n "$TUNNEL" ]] && { echo "== tunnel"; curl -sS -H 'Authorization: Bearer admintok' "$BCC/api/tunnels?id=$TUNNEL" 2>/dev/null | head -c 6000; echo; }
  fi
  # Remove every LAB effect, even after a failure, in reverse order. The
  # servers are test servers, but nothing this run created is left behind.
  if [[ "$FWLAB_IR" == 1 ]]; then on ir "nft delete table inet baftlab_drop" 2>/dev/null; fi
  if [[ "$FWLAB_CUT" == 1 ]]; then on ir "nft delete table inet baftlab_cut" 2>/dev/null; fi
  if [[ -n "$TUNNEL" && "$DECOMMISSIONED" == 0 && $rc -ne 0 ]]; then
    log "cleanup: best-effort decommission of $TUNNEL"
    ( decommission "$TUNNEL" ) >/dev/null 2>&1
  fi
  for h in ir ex; do
    [[ "$h" == ir && "$INSTALLED_IR" == 0 ]] && continue
    [[ "$h" == ex && "$INSTALLED_EX" == 0 ]] && continue
    log "cleanup: full uninstall of BAFT on $h"
    on "$h" "systemctl stop lab-echo baft-realhosts-bcc baft-realhosts-mirror 2>/dev/null
      if command -v baft >/dev/null; then baft uninstall --full --delete-tunnel-configs --stop-active-tunnels --yes >$LAB/uninstall.log 2>&1 || tail -n 20 $LAB/uninstall.log; fi
      systemctl disable --now baft-agent 2>/dev/null; true" 2>/dev/null
    on "$h" "rm -f $LAB_CA; update-ca-certificates --fresh >/dev/null 2>&1; rm -rf $LAB; systemctl reset-failed 'baft-realhosts-*' lab-echo 2>/dev/null; true" 2>/dev/null
    left="$(on "$h" "systemctl list-units --all --no-legend 'baft*' 2>/dev/null | grep -vc baft-uninstall; nft list tables 2>/dev/null | grep -c baftlab; nft list tables 2>/dev/null | grep -c 'inet baft\$'; true" 2>/dev/null | tr '\n' ' ')"
    log "cleanup on $h: remaining baft units / lab tables / inet baft table: $left"
  done
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done
  [[ -f "$SUMMARY" ]] && { echo "== summary"; cat "$SUMMARY"; }
  [[ -n "${SUMMARY_OUT:-}" ]] && cp "$SUMMARY" "$SUMMARY_OUT" 2>/dev/null
  rm -rf "$WORK"
  exit "$rc"
}
# shellcheck source=tests/e2e/lib/flow.sh
source tests/e2e/lib/flow.sh
trap cleanup EXIT
trap 'fail "line $LINENO: unexpected failure of: $BASH_COMMAND. $(tail -n 3 "$WORK/fail-reason" 2>/dev/null | tr "\n" " ")"' ERR

# -------------------------------------------------------------- preflight --
log "preflight — SSH to the foreign server"
on ex true || fail "cannot SSH to the foreign server as $SSH_USER (key, port $SSH_PORT, firewall, or sudo -n)"
log "preflight — SSH to the Iran server (direct, else through the foreign server)"
if on ir true 2>/dev/null; then record ir_ssh '"direct"'
else
  IR_VIA=("${JUMP[@]}")
  on ir true || fail "cannot SSH to the Iran server, neither directly nor through the foreign server"
  record ir_ssh '"via foreign server"'
fi
if [[ "$HOSTKEY" == accept-new ]]; then
  record host_key_fingerprints "$(ssh-keygen -lf "$WORK/known_hosts" 2>/dev/null | awk '{print $2}' | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read().split()))')"
fi

facts() { # host -> JSON of read-only facts
  on "$1" "PORTS='$CANDIDATE_PORTS,$CLIENT_PORT,$BCC_PORT,$MIRROR_PORT' python3 - <<'PY'
import json, os, re, shutil, subprocess
def sh(c):
    try: return subprocess.run(c, shell=True, capture_output=True, text=True, timeout=20).stdout.strip()
    except Exception as e: return 'error: %s' % e
ports = sorted({int(p) for p in os.environ['PORTS'].split(',')})
listen = sh('ss -Htlnp')
busy = {}
for p in ports:
    rows = [l for l in listen.splitlines() if re.search(r':%d\s' % p, l)]
    if rows: busy[p] = [re.sub(r'\s+', ' ', r)[:160] for r in rows]
osr = dict(l.split('=', 1) for l in open('/etc/os-release').read().splitlines() if '=' in l)
print(json.dumps({
  'os': osr.get('PRETTY_NAME', '').strip('\"'), 'kernel': os.uname().release, 'arch': os.uname().machine,
  'pid1': os.path.basename(os.readlink('/proc/1/exe')) if os.path.exists('/proc/1/exe') else '',
  'virt': sh('systemd-detect-virt 2>/dev/null || true'), 'cpus': os.cpu_count(),
  'mem_mib': int(open('/proc/meminfo').read().split()[1]) // 1024,
  'nft': bool(shutil.which('nft')), 'python3': True,
  'baft_installed': bool(sh('systemctl list-unit-files --no-legend \"baft*\" 2>/dev/null')) or os.path.exists('/etc/baft'),
  'planned_ports_busy': busy,
  'services': {k: bool(sh(c)) for k, c in {
      'xray': 'pgrep -x xray || ls /usr/local/etc/xray/config.json 2>/dev/null',
      'x-ui': 'pgrep -x x-ui || ls /etc/x-ui/x-ui.db 2>/dev/null',
      'nginx': 'pgrep -x nginx', 'haproxy': 'pgrep -x haproxy', 'docker': 'pgrep -x dockerd'}.items()},
}))
PY"
}
FEX="$(facts ex)" || fail "could not read the foreign server's facts"
FIR="$(facts ir)" || fail "could not read the Iran server's facts"
record ex_facts "$FEX"; record ir_facts "$FIR"
log "foreign: $(printf '%s' "$FEX" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["os"],d["kernel"],d["arch"],"busy:",list(d["planned_ports_busy"]),"services:",[k for k,v in d["services"].items() if v])')"
log "iran:    $(printf '%s' "$FIR" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["os"],d["kernel"],d["arch"],"busy:",list(d["planned_ports_busy"]),"services:",[k for k,v in d["services"].items() if v])')"

log "preflight — the Iran server reaches the foreign server on the BCC and candidate ports (TCP connect)"
REACH="$(on ir "python3 - '$EX_HOST' '$BCC_PORT,$CANDIDATE_PORTS' <<'PY'
import json, socket, sys, time
host, ports = sys.argv[1], [int(p) for p in sys.argv[2].split(',')]
out = {}
for p in ports:
    t = time.monotonic()
    try:
        socket.create_connection((host, p), timeout=5).close(); out[p] = 'open %.0f ms' % ((time.monotonic() - t) * 1000)
    except ConnectionRefusedError: out[p] = 'refused (reachable, nothing listening)'
    except OSError as e: out[p] = 'blocked/timeout: %s' % type(e).__name__
print(json.dumps(out))
PY")" || fail "reachability check from the Iran server failed to run"
record ir_to_ex_tcp "$REACH"; log "iran -> foreign TCP: $REACH"

if [[ "$MODE" == preflight ]]; then
  record result '"PREFLIGHT_OK"'
  [[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && { echo "### Real-hosts preflight"; echo; echo '```json'; cat "$SUMMARY"; echo '```'; } >>"$GITHUB_STEP_SUMMARY"
  log "preflight complete; nothing was changed on either server"
  exit 0
fi

# ------------------------------------------------------------------- full --
[[ -n "${REL:-}" && -f "$REL/dist/manifest.json" ]] || fail "MODE=full needs REL=<release dir from local_release.sh>"
for f in "$FEX" "$FIR"; do
  [[ "$(printf '%s' "$f" | jget baft_installed)" == False ]] || fail "BAFT is already installed on a server; this LAB only runs on clean test servers"
  [[ "$(printf '%s' "$f" | jget planned_ports_busy)" == "{}" ]] || fail "a planned port is already in use: $(printf '%s' "$f" | jget planned_ports_busy)"
  [[ "$(printf '%s' "$f" | jget arch)" == x86_64 ]] || fail "only x86_64 servers are supported by this LAB"
done
VERSION="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$REL/dist/manifest.json")"
COMMIT="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["commit"])' "$REL/dist/manifest.json")"
[[ "$COMMIT" == "$(git rev-parse HEAD)" ]] || fail "the release was not built from the checked-out commit"
record commit "\"$COMMIT\""; log "release $VERSION built from $COMMIT"

# BCC listens on the foreign server's own address for its public IP (cloud
# hosts often NAT the public IP); its certificate names the public IP.
BIND="$(on ex "ip -4 -o addr show | awk '{print \$4}' | cut -d/ -f1 | grep -Fx '$EX_HOST' || ip -4 route get 1.1.1.1 | awk '{for(i=1;i<NF;i++) if(\$i==\"src\") print \$(i+1)}'")"
[[ "$BIND" =~ ^[0-9.]+$ ]] || fail "could not determine the foreign server's bind address"
BCC="https://$EX_HOST:$BCC_PORT"
log "BCC TLS: throwaway CA and a certificate for the foreign server's public IP"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj "/CN=BAFT real-hosts LAB CA" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign" -keyout "$WORK/ca.key" -out "$WORK/ca.crt" >/dev/null 2>&1
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=baft-real-hosts" -keyout "$WORK/bcc.key" -out "$WORK/bcc.csr" >/dev/null 2>&1
printf 'subjectAltName=IP:%s\nextendedKeyUsage=serverAuth\nbasicConstraints=critical,CA:FALSE\n' "$EX_HOST" >"$WORK/bcc.ext"
openssl x509 -req -in "$WORK/bcc.csr" -CA "$WORK/ca.crt" -CAkey "$WORK/ca.key" -CAcreateserial -days 1 -extfile "$WORK/bcc.ext" -out "$WORK/bcc.crt" >/dev/null 2>&1
rm -f "$WORK/ca.key"
# The runner reaches BCC through an SSH forward; curl still verifies the
# certificate for the public IP against the throwaway CA.
ssh "${SSHO[@]}" -N -o ExitOnForwardFailure=yes -L "127.0.0.1:$BCC_PORT:$BIND:$BCC_PORT" "$SSH_USER@$EX_HOST" & PIDS+=($!)
export CURL_HOME="$WORK"
printf 'cacert = "%s"\nconnect-to = "%s:%s:127.0.0.1:%s"\n' "$WORK/ca.crt" "$EX_HOST" "$BCC_PORT" "$BCC_PORT" >"$WORK/.curlrc"

tar -C "$REL" -czf "$WORK/rel.tgz" dist root.pub revocations.json
TOK_EX="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"; TOK_IR="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
stage() { # host: copy release, CA, helpers; trust the CA; nft present
  on "$1" "mkdir -p $LAB && chmod 700 $LAB"
  put "$1" "$LAB" "$WORK/rel.tgz" "$WORK/ca.crt" install.sh tests/e2e/echo.py
  on "$1" "set -e; tar -C $LAB -xzf $LAB/rel.tgz; install -m 0644 $LAB/ca.crt $LAB_CA; update-ca-certificates >/dev/null 2>&1
    command -v nft >/dev/null || { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables >/dev/null; }"
}
log "stage the release on the foreign server; start BCC and the release mirror"
INSTALLED_EX=1; stage ex
put ex "$LAB" "$WORK/bcc.crt" "$WORK/bcc.key"
printf '%s\n%s\n' "$TOK_EX" "$TOK_IR" | on ex "umask 077; cat >$LAB/tokens"
printf '%s\n' "$TOK_EX" | on ex "umask 077; cat >$LAB/agent.token"
on ex "set -e; cd $LAB; install -m 0755 dist/baft-bcc-linux-amd64 $LAB/baft-bcc; echo admintok >admin.token; chmod 600 admin.token bcc.key
  ./baft-bcc access init --access-file $LAB/s.json.access.json >/dev/null
  mkdir -p www/$VERSION; cp dist/* www/$VERSION/; cp revocations.json www/
  systemd-run --unit=baft-realhosts-mirror --collect -p WorkingDirectory=$LAB/www python3 -m http.server $MIRROR_PORT --bind $BIND >/dev/null
  systemd-run --unit=baft-realhosts-bcc --collect -p WorkingDirectory=$LAB \
    --setenv=BAFT_BCC_ALLOWED_LISTEN_IPS=$BIND --setenv=TOK_EX=\$(sed -n 1p tokens) --setenv=TOK_IR=\$(sed -n 2p tokens) \
    $LAB/baft-bcc --admin-token-file $LAB/admin.token --state-file $LAB/s.json --listen $BIND:$BCC_PORT \
    --tls-cert $LAB/bcc.crt --tls-key $LAB/bcc.key >/dev/null
  rm -f tokens"
for _ in $(seq 1 30); do api -o /dev/null "$BCC/api/nodes" 2>/dev/null && break; sleep 1; done
api -o /dev/null "$BCC/api/nodes" || fail "BCC on the foreign server is not reachable over verified HTTPS"
JOB_KEY="$(on ex "$LAB/baft-bcc jobkey show --job-key-file $LAB/s.json.job-key")"
post /api/nodes "{\"ID\":\"$EX_NODE\",\"Alias\":\"foreign\",\"Address\":\"$EX_HOST:$SSH_PORT\",\"Role\":\"foreign\",\"AgentTokenEnv\":\"TOK_EX\"}" >/dev/null
post /api/nodes "{\"ID\":\"$IR_NODE\",\"Alias\":\"iran\",\"Address\":\"$IR_HOST:$SSH_PORT\",\"Role\":\"worker\",\"AgentTokenEnv\":\"TOK_IR\"}" >/dev/null

install_agent() { # host node flags... ($LAB/agent.token is already in place)
  local h="$1" node="$2"; shift 2
  on "$h" "set -e; cd $LAB; chmod 600 agent.token
    env BAFT_INSTALL_FROM=release BAFT_RELEASE_URL=file://$LAB/dist BAFT_ROOT_PUB=\"\$(cat $LAB/root.pub)\" \
      BAFT_REVOCATIONS_URL=file://$LAB/revocations.json BAFT_BCC_JOB_KEY='$JOB_KEY' BAFT_AGENT_TOKEN_FILE=$LAB/agent.token \
      BAFT_AGENT_RELEASE_BASE_URL=http://$EX_HOST:$MIRROR_PORT BAFT_AGENT_REVOCATIONS_URL=http://$EX_HOST:$MIRROR_PORT/revocations.json \
      BAFT_AGENT_INTERVAL=2s bash $LAB/install.sh --agent-only --bcc-url '$BCC' --node-id '$node' $* >$LAB/install.log 2>&1 \
      || { tail -n 15 $LAB/install.log; exit 1; }
    systemctl is-active baft-agent >/dev/null" </dev/null || fail "install.sh failed on the $h server"
}
log "install.sh --agent-only on the foreign server"
install_agent ex "$EX_NODE" --no-ingress-chain
log "stage and install on the Iran server (--enable-host-ingress)"
INSTALLED_IR=1; stage ir
printf '%s\n' "$TOK_IR" | on ir "umask 077; cat >$LAB/agent.token"
install_agent ir "$IR_NODE" --enable-host-ingress
for _ in $(seq 1 60); do
  IRC="$(caps "$IR_NODE")"; EXC="$(caps "$EX_NODE")"
  [[ ",$IRC," == *",operational-ingress-host-v1,"* && -n "$EXC" ]] && break
  sleep 3
done
log "Iran capabilities: ${IRC:-none}; foreign capabilities: ${EXC:-none}"
[[ ",$IRC," == *",operational-ingress-host-v1,"* ]] || fail "the Iran server does not advertise safe host mode: $IRC"

on ex "systemd-run --unit=lab-echo --collect python3 $LAB/echo.py serve $TARGET_PORT >/dev/null"

log "step 1 — connection test: signed probe Iran -> foreign on $CANDIDATE_PORTS"
PROBE="$(post /api/path-probes "{\"source_node\":\"$IR_NODE\",\"destination_node\":\"$EX_NODE\",\"family\":\"4\",\"candidate_ports\":[$CANDIDATE_PORTS]}")"
PID="$(printf '%s' "$PROBE" | jget id)"
for _ in $(seq 1 120); do
  PROBE="$(get "/api/path-probes?id=$PID")"
  case "$(printf '%s' "$PROBE" | jget phase)" in complete|failed|cancelled) break ;; esac
  sleep 3
done
record probe "$(printf '%s' "$PROBE" | python3 -c 'import json,sys
d=json.load(sys.stdin)
print(json.dumps({"phase":d.get("phase"),"candidates":[{k:c.get(k) for k in ("port","class","connect_rtt_ms","connect_spread_ms","connect_successes","attempts","throughput_kbps","throughput_at_least")} for c in d.get("candidates") or []]}))')"
[[ "$(printf '%s' "$PROBE" | jget phase)" == complete ]] || fail "the connection test did not complete: $(printf '%s' "$PROBE" | head -c 1500)"
printf '%s' "$PROBE" | python3 -c 'import json,sys;sys.exit(0 if any(c.get("class")=="FULL_DATA" for c in json.load(sys.stdin).get("candidates") or []) else 1)' \
  || fail "no candidate port proved FULL_DATA across the real path: $(python3 -c 'import json,sys;print(json.dumps(json.load(open(sys.argv[1]))["probe"]))' "$SUMMARY")"

log "step 2 — evidence-checked auto-select"
SEL=""
for _ in $(seq 1 15); do
  SEL="$(post /api/tunnels/auto-select "{\"ex_node\":\"$EX_NODE\",\"ir_node\":\"$IR_NODE\",\"family\":\"4\",\"carrier\":\"h2\"}" | jget selected)"
  [[ -n "$SEL" ]] && break; sleep 2
done
[[ -n "$SEL" ]] || fail "auto-select refused fresh evidence"
EX_PORT="$(printf '%s' "$SEL" | jget port)"; record selected_port "$EX_PORT"
log "auto-select chose foreign port $EX_PORT"

log "step 3 — reviewed plan with the acknowledged public port $CLIENT_PORT; deploy"
REQ="$(make_req "$CLIENT_PORT" 1 baft-real-1 127.0.0.1:1444 127.0.0.1:9202 127.0.0.1:9302 1)"
HASH="$(plan_ok "$REQ" | jget hash)"
TUNNEL="$(deploy_active "$REQ" "$HASH")"; T="$TUNNEL"

ready_within() { # seconds -> 0 when BCC's own probe reports Ready
  local end=$(( $(date +%s) + $1 )) rd
  while (( $(date +%s) < end )); do
    rd="$(get "/api/ingress/readiness?tunnel_id=$T")"
    [[ "$(printf '%s' "$rd" | jget ready)" == True ]] && return 0
    sleep 3
  done
  printf '%s' "$rd" | head -c 1200 >>"$WORK/fail-reason"; return 1
}
not_ready_within() { # seconds -> 0 once BCC's own probe stops reporting Ready
  local end=$(( $(date +%s) + $1 ))
  while (( $(date +%s) < end )); do
    [[ "$(get "/api/ingress/readiness?tunnel_id=$T" | jget ready)" != True ]] && return 0
    sleep 3
  done
  return 1
}
log "step 4 — owned listener + rule on the Iran server; Ready by BCC's independent probe"
st=""
for _ in $(seq 1 60); do st="$(ingress_state "$T")"; case "${st%% *}" in applied|failed|blocked) break ;; esac; sleep 3; done
[[ "${st%% *}" == applied ]] || fail "public ingress did not apply on the Iran server: $st"
ready_within 90 || fail "BCC did not verify the Iran server's public port $CLIENT_PORT"
on ir "nft list chain inet baft ingress" | grep -Eq "dport $CLIENT_PORT( |$)" || fail "no owned rule for $CLIENT_PORT in the Iran server's inet baft ingress chain"

cat >"$WORK/measure.py" <<'PY'
import json, os, socket, statistics, sys, time, threading
host, port, label, mib, limit = sys.argv[1], int(sys.argv[2]), sys.argv[3], int(sys.argv[4]), float(sys.argv[5])
data = os.urandom(mib * 1024 * 1024); got = bytearray(); t0 = time.monotonic()
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
        msg = b"%08d" % i; t = time.monotonic(); s.sendall(msg); r = b""
        while len(r) < len(msg): r += s.recv(64)
        rtts.append((time.monotonic() - t) * 1000)
rtts.sort()
print(json.dumps({"label": label, "integrity": bytes(got) == data, "mib": mib, "seconds": round(secs, 3),
    "mbps_echo": round(len(data) * 8 / secs / 1e6, 2), "rtt_ms_p50": round(rtts[len(rtts)//2], 1),
    "rtt_ms_p95": round(rtts[int(len(rtts)*0.95)-1], 1), "rtt_ms_max": round(rtts[-1], 1)}))
PY
measure_from() { # runner|ex label MiB timeout
  if [[ "$1" == runner ]]; then python3 "$WORK/measure.py" "$IR_HOST" "$CLIENT_PORT" "$2" "$3" "$4"
  else on ex "python3 - '$IR_HOST' '$CLIENT_PORT' '$2' '$3' '$4'" <"$WORK/measure.py"; fi
}
log "step 5 — real client traffic through the Iran server's public port (from abroad: the foreign server and the runner)"
M="$(measure_from ex from_foreign_server 8 300)" || fail "client traffic from the foreign server through the Iran public port failed"
log "from the foreign server: $M"; record traffic_from_foreign_server "$M"
[[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "data mismatch through the real path"
if M="$(measure_from runner from_runner 8 300 2>/dev/null)"; then record traffic_from_runner "$M"; log "from the runner: $M"
else record traffic_from_runner '"unreachable from the runner (not required)"'; fi

log "step 6 — agent restarts must not disturb the live tunnel or its public port"
ingress_job() { get "/api/tunnels?id=$T" | python3 -c 'import json,sys;print((json.load(sys.stdin).get("ingress") or {}).get("job_id",""))'; }
J0="$(ingress_job)"; t0=$(date +%s)
on ir "systemctl restart baft-agent"; on ex "systemctl restart baft-agent"
# The listener lives in the Agent process, so a restart closes the public
# port; the Agent rolls it back with evidence and BCC re-issues a fresh
# signed Apply (ADR-003 §4.4). Wait for that new Apply (BCC's readiness
# cache could otherwise still answer for the old listener), then for Ready,
# and record how long the public port was not Ready.
restaged=false
for _ in $(seq 1 40); do [[ "$(ingress_job)" != "$J0" ]] && { restaged=true; break; }; sleep 3; done
[[ "$restaged" == true ]] || sleep 12
ready_within 120 || fail "not Ready again after agent restarts"
record agent_restart "{\"restaged\":$restaged,\"ready_again_after_s\":$(( $(date +%s) - t0 ))}"
M="$(measure_from ex after_agent_restart 2 300)" && [[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "traffic failed after agent restarts"
record after_agent_restart "$M"

log "step 7 — firewall drop: an operator rule drops the public port; BCC must not report Ready"
FWLAB_IR=1
on ir "nft add table inet baftlab_drop && nft add chain inet baftlab_drop input '{ type filter hook input priority -300; policy accept; }' && nft add rule inet baftlab_drop input tcp dport $CLIENT_PORT drop"
t0=$(date +%s)
not_ready_within 90 || fail "BCC still reports Ready while the public port is dropped (false Ready)"
record firewall_drop "{\"not_ready_after_s\":$(( $(date +%s) - t0 ))}"
on ir "nft delete table inet baftlab_drop"; FWLAB_IR=0
t0=$(date +%s); ready_within 90 || fail "not Ready again after the drop rule was removed"
record firewall_restore "{\"ready_again_after_s\":$(( $(date +%s) - t0 ))}"

log "step 8 — tunnel path cut: the Iran server drops its own tunnel traffic to the foreign port for 20 s"
FWLAB_CUT=1
on ir "nft add table inet baftlab_cut && nft add chain inet baftlab_cut output '{ type filter hook output priority -300; policy accept; }' && nft add rule inet baftlab_cut output ip daddr $EX_HOST tcp dport $EX_PORT drop"
sleep 5
# The cut must really break the path, otherwise "recovery" proves nothing.
if M="$(measure_from ex during_cut 1 10 2>/dev/null)" && [[ "$(printf '%s' "$M" | jget integrity)" == True ]]; then
  fail "traffic still flows during the path cut; the cut is not effective, so recovery cannot be measured"
fi
sleep 15
on ir "nft delete table inet baftlab_cut"; FWLAB_CUT=0
t0=$(date +%s); recovered=""
for _ in $(seq 1 90); do
  if M="$(measure_from ex recovery_probe 1 30 2>/dev/null)" && [[ "$(printf '%s' "$M" | jget integrity)" == True ]]; then
    recovered=$(( $(date +%s) - t0 )); break
  fi
  sleep 2
done
[[ -n "$recovered" ]] || fail "the tunnel did not recover within ~4 min after the path came back"
log "recovered ${recovered}s after the path came back"; record path_cut "{\"cut_s\":20,\"recovered_after_s\":$recovered}"

if [[ "$REBOOT" == 1 ]]; then
  log "step 9 — reboot persistence: reboot the Iran server; the public port must come back by itself"
  BOOT0="$(on ir 'cat /proc/sys/kernel/random/boot_id')"
  on ir "systemctl reboot" || true
  sleep 20; back=""
  for _ in $(seq 1 60); do
    b="$(on ir 'cat /proc/sys/kernel/random/boot_id' 2>/dev/null)" && [[ -n "$b" && "$b" != "$BOOT0" ]] && { back=1; break; }
    sleep 5
  done
  [[ -n "$back" ]] || fail "the Iran server did not come back after reboot"
  t0=$(date +%s)
  ready_within 300 || fail "not Ready after reboot"
  M="$(measure_from ex after_reboot 2 300)" && [[ "$(printf '%s' "$M" | jget integrity)" == True ]] || fail "traffic failed after reboot"
  record reboot "{\"ready_after_boot_s\":$(( $(date +%s) - t0 )),\"traffic\":$M}"
else
  record reboot '"not requested"'
fi

log "step 10 — decommission: public port closed, owned rule removed, BAFT's chain kept"
decommission "$T"; DECOMMISSIONED=1
on ex "python3 - '$IR_HOST' '$CLIENT_PORT' <<'PY'
import socket, sys, time
for _ in range(30):
    try: socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=3).close()
    except OSError: sys.exit(0)
    time.sleep(2)
sys.exit(1)
PY" || fail "the Iran public port still accepts connections after decommission"
if on ir "nft list chain inet baft ingress" | grep -Eq "dport $CLIENT_PORT( |$)"; then fail "owned rule survived decommission"; fi
record result '"PASS"'
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  { echo "### Real-hosts LAB ($COMMIT) — real Iran <-> abroad path"; echo; echo '```json'; cat "$SUMMARY"; echo '```'; } >>"$GITHUB_STEP_SUMMARY"
fi
log "PASS: real servers -> FULL_DATA -> plan -> deploy -> Ready -> real traffic -> restarts -> firewall drop -> path cut + recovery -> decommission"
