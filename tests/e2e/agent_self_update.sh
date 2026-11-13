#!/usr/bin/env bash
# BCC Update Center path: a signed update_baft job replaces the agent itself.
#
# On a systemd runner: a real BCC, an agent installed with install.sh
# --agent-only from signed release A, and release B served over HTTP the way
# the agent fetches releases. Then:
# 1. rollback: BCC stops right after it records the job's result, so the new
#    agent can never reconnect; the external guard (systemd-run timer) must put
#    agent A back and restart it, and agent A must reconnect when BCC returns;
# 2. success: the same job again with BCC up; the agent must restart into B,
#    reconnect, disarm the guard, and still be B after the guard time.
# Releases are signed with throwaway keys (scripts/release/local_release.sh).
#
#   sudo tests/e2e/agent_self_update.sh <release-A-dir> <release-B-dir>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; *) echo "unsupported arch" >&2; exit 1 ;; esac
RELA="$(cd "${1:?usage: agent_self_update.sh <release-A> <release-B>}" && pwd)"
RELB="$(cd "${2:?usage: agent_self_update.sh <release-A> <release-B>}" && pwd)"
version_of() { python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$1/dist/manifest.json"; }
VA="$(version_of "$RELA")"; VB="$(version_of "$RELB")"
W="$(mktemp -d)"
PIDS=() BCC_PID=""
STEP="setup"
GUARD=25s
log() { printf '[agent-self-update] %s\n' "$*"; }
fail() { echo "[agent-self-update] FAIL ($STEP): $*" >&2; exit 1; }
cleanup() {
  local rc=$?
  trap - EXIT
  if [[ $rc -ne 0 ]]; then
    for f in "$W"/*.log; do [[ -f "$f" ]] && { echo "== $f" >&2; tail -n 40 "$f" >&2; }; done
    echo "== journal baft-agent" >&2; journalctl -u baft-agent -n 80 --no-pager >&2 2>/dev/null || true
    echo "== guard units" >&2; systemctl list-units --all 'baft-agent-update-guard-*' --no-pager >&2 2>/dev/null || true
    [[ "${GITHUB_ACTIONS:-}" == true ]] && echo "::error title=agent-self-update::failed at step: $STEP"
  fi
  systemctl stop baft-agent 2>/dev/null || true
  systemctl disable baft-agent 2>/dev/null || true
  rm -f /etc/systemd/system/baft-agent.service; systemctl daemon-reload 2>/dev/null || true
  [[ -n "$BCC_PID" ]] && kill "$BCC_PID" 2>/dev/null || true
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  rm -rf /etc/baft-agent /var/lib/baft-agent /opt/baft /usr/local/bin/baft /usr/local/bin/baft-pair /usr/local/bin/baft-agent \
    /usr/local/bin/baft-agent.prev /usr/local/bin/baft.prev /usr/local/bin/baft-pair.prev "$W"
  exit $rc
}
trap cleanup EXIT

sum() { sha256sum "$1" | awk '{print $1}'; }
agent_is() { [[ "$(sum /usr/local/bin/baft-agent)" == "$(sum "$1/dist/baft-agent-linux-$ARCH")" ]]; }
mainpid() { systemctl show -p MainPID --value baft-agent; }
api() { curl -fsS -H 'Authorization: Bearer admintok' "$@"; }
start_bcc() {
  AGENT_TOK="$AGENT_TOKEN" "$W/baft-bcc" --admin-token-file "$W/admin.token" --state-file "$W/s.json" \
    --listen 127.0.0.1:18400 >>"$W/bcc.log" 2>&1 & BCC_PID=$!
  for _ in $(seq 1 30); do api -o /dev/null http://127.0.0.1:18400/api/nodes 2>/dev/null && return 0; sleep 1; done
  fail "BCC did not start"
}
stop_bcc() { kill "$BCC_PID"; wait "$BCC_PID" 2>/dev/null || true; BCC_PID=""; }
last_job() { api http://127.0.0.1:18400/api/jobs | python3 -c 'import json,sys;j=sorted(json.load(sys.stdin),key=lambda x:x["id"]);print(j[-1]["status"]+"|"+j[-1].get("message",""))'; }
deploy_and_wait() { # -> prints final status|message
  api -d "{\"node_ids\":[\"upd-e2e\"],\"version\":\"$VB\"}" http://127.0.0.1:18400/api/deploy >/dev/null
  local st=""
  for _ in $(seq 1 90); do
    st="$(last_job)"
    case "$st" in succeeded*|failed*) printf '%s' "$st"; return 0 ;; esac
    sleep 1
  done
  fail "the update job never finished: $st"
}

STEP="releases"
for b in baft baft-agent; do
  cmp -s "$RELA/dist/$b-linux-$ARCH" "$RELB/dist/$b-linux-$ARCH" && fail "A and B ship the same $b"
done
mkdir -p "$W/www/$VB"; cp "$RELB"/dist/* "$W/www/$VB/"; cp "$RELB/revocations.json" "$W/www/"
(cd "$W/www" && exec python3 -m http.server 18401 --bind 127.0.0.1) >"$W/www.log" 2>&1 & PIDS+=($!)

STEP="BCC"
go build -o "$W/baft-bcc" ./cmd/baft-bcc
echo admintok >"$W/admin.token"; chmod 600 "$W/admin.token"
"$W/baft-bcc" access init --access-file "$W/s.json.access.json" >/dev/null
AGENT_TOKEN="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
start_bcc
JOB_KEY="$("$W/baft-bcc" jobkey show --job-key-file "$W/s.json.job-key")"
api -d '{"ID":"upd-e2e","Alias":"update e2e","Address":"127.0.0.1:1","Role":"foreign","AgentTokenEnv":"AGENT_TOK"}' http://127.0.0.1:18400/api/nodes >/dev/null

STEP="install agent A"
printf '%s\n' "$AGENT_TOKEN" >"$W/token"; chmod 600 "$W/token"
env BAFT_INSTALL_FROM=release BAFT_RELEASE_URL="file://$RELA/dist" BAFT_ROOT_PUB="$(cat "$RELA/root.pub")" \
  BAFT_REVOCATIONS_URL="file://$RELA/revocations.json" BAFT_BCC_JOB_KEY="$JOB_KEY" BAFT_AGENT_TOKEN_FILE="$W/token" \
  BAFT_AGENT_ALLOW_HTTP=1 BAFT_AGENT_INTERVAL=2s BAFT_AGENT_RELEASE_BASE_URL=http://127.0.0.1:18401 \
  BAFT_AGENT_REVOCATIONS_URL=http://127.0.0.1:18401/revocations.json BAFT_NONINTERACTIVE=1 BAFT_TUNE_NETWORK=0 \
  bash install.sh --agent-only --bcc-url http://127.0.0.1:18400 --node-id upd-e2e --no-ingress-chain >"$W/install.log" 2>&1 ||
  fail "agent install from A failed"
# A short guard keeps the test quick (production: 3 minutes). Each agent start
# waits 3 s, so the rollback step can stop BCC before the new agent's first poll.
sed -i "s|^ExecStart=\(.*\)$|ExecStartPre=/bin/sleep 3\nExecStart=\1 --self-update-guard $GUARD|" /etc/systemd/system/baft-agent.service
grep -q -- "--self-update-guard $GUARD" /etc/systemd/system/baft-agent.service || fail "could not set the test guard time"
systemctl daemon-reload; systemctl restart baft-agent
agent_is "$RELA" || fail "the installed agent is not A's"
for _ in $(seq 1 30); do journalctl -u baft-agent --no-pager | grep -q "polling http://127.0.0.1:18400" && break; sleep 1; done

STEP="rollback"
log "update to $VB while BCC disappears right after recording the result: the guard must restore agent A"
api -d "{\"node_ids\":[\"upd-e2e\"],\"version\":\"$VB\"}" http://127.0.0.1:18400/api/deploy >/dev/null
st=""
for _ in $(seq 1 600); do
  st="$(last_job)"
  case "$st" in succeeded*|failed*) break ;; esac
  sleep 0.1
done
stop_bcc
log "job: ${st%%|*}"
[[ "$st" == succeeded*"baft-agent $VB installed"* ]] || fail "the job did not install agent $VB: $st"
restored=""
for _ in $(seq 1 90); do
  if agent_is "$RELA" && [[ ! -f /var/lib/baft-agent/agent-update.json ]]; then restored=1; break; fi
  sleep 1
done
[[ -n "$restored" ]] || fail "the guard did not restore agent A after $GUARD (agent is $(agent_is "$RELB" && echo B || echo other))"
systemctl is-active --quiet baft-agent || sleep 12
systemctl is-active --quiet baft-agent || fail "the restored agent A is not running"
start_bcc
sleep 5
agent_is "$RELA" || fail "agent A was replaced again without a job"
log "rolled back to agent A and reconnected"

STEP="success"
log "the same update with BCC up: the agent must restart into $VB and keep it"
pid_a="$(mainpid)"
st="$(deploy_and_wait)"
[[ "$st" == succeeded*"baft-agent $VB installed"* ]] || fail "the job did not install agent $VB: $st"
ok=""
for _ in $(seq 1 60); do
  if agent_is "$RELB" && [[ ! -f /var/lib/baft-agent/agent-update.json ]] && [[ "$(mainpid)" != "$pid_a" ]]; then ok=1; break; fi
  sleep 1
done
[[ -n "$ok" ]] || fail "agent B did not confirm itself (marker left: $([[ -f /var/lib/baft-agent/agent-update.json ]] && echo yes || echo no))"
journalctl -u baft-agent --no-pager | grep -q "now running baft-agent $VB" || fail "agent B did not log its confirmation"
sleep "${GUARD%s}"; sleep 5
agent_is "$RELB" || fail "the guard rolled back a healthy agent B"
systemctl is-active --quiet baft-agent || fail "agent B is not running after the guard time"
[[ "$(sum /usr/local/bin/baft-agent.prev)" == "$(sum "$RELA/dist/baft-agent-linux-$ARCH")" ]] || fail "agent A was not kept as .prev"
log "PASS: the Update Center job replaced the agent, the external guard rolled a dead update back, and a healthy update stayed"
