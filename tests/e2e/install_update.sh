#!/usr/bin/env bash
# One update for everything BAFT installed on a server (install.sh --update).
#
# On a systemd runner: install the panel (BCC) and an enrolled-style agent from
# signed release A, then update the whole server to signed release B with one
# command, and check that
# - every installed program (baft-bcc, baft-agent, baft, baft-pair) is B's,
#   and the release state records B;
# - no secret, unit, listener or setting changed, and no question was asked;
# - --update --plan changes nothing; a plain rerun on the updated server finds
#   it current and restarts nothing;
# - --update refuses role/pairing options and a server with nothing installed;
# - an agent installed without BAFT's firewall chain stays without it.
# Releases are signed with throwaway keys (scripts/release/local_release.sh).
#
#   BAFT_LOCAL_RELEASE_ROOT=$T/root scripts/release/local_release.sh $T/a v0.0.1-upd
#   BAFT_LOCAL_RELEASE_ROOT=$T/root scripts/release/local_release.sh $T/b v0.0.2-upd
#   sudo tests/e2e/install_update.sh $T/a $T/b
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; *) echo "unsupported arch" >&2; exit 1 ;; esac

W="$(mktemp -d)"
STEP="setup"
log() { echo "[install-update] $*"; }
fail() { echo "[install-update] FAIL ($STEP): $*" >&2; exit 1; }
cleanup() {
  local rc=$?
  trap - EXIT
  if [[ $rc -ne 0 ]]; then
    for f in "$W"/*.out; do [[ -f "$f" ]] && { echo "== $f" >&2; tail -n 60 "$f" >&2; }; done
    for u in baft-bcc baft-agent; do echo "== journal $u" >&2; journalctl -u "$u" -n 30 --no-pager >&2 2>/dev/null || true; done
    [[ "${GITHUB_ACTIONS:-}" == true ]] && echo "::error title=install-update::failed at step: $STEP"
  fi
  for u in baft-agent baft-bcc; do
    systemctl stop "$u.service" >/dev/null 2>&1 || true
    systemctl disable "$u.service" >/dev/null 2>&1 || true
    rm -f "/etc/systemd/system/$u.service"
  done
  systemctl daemon-reload >/dev/null 2>&1 || true
  rm -rf /etc/baft-bcc /var/lib/baft-bcc /etc/baft-agent /var/lib/baft-agent /opt/baft /etc/baft /var/lib/baft \
    /usr/local/bin/baft /usr/local/bin/baft-pair /usr/local/bin/baft-agent /usr/local/bin/baft-bcc /usr/local/bin/baft-panel
  rm -rf "$W"
  exit $rc
}
trap cleanup EXIT

STEP="releases"
RELA="${1:?usage: install_update.sh <release-A-dir> <release-B-dir> (local_release.sh, one BAFT_LOCAL_RELEASE_ROOT)}"
RELB="${2:?usage: install_update.sh <release-A-dir> <release-B-dir>}"
mkdir -p "$W/relA" "$W/relB"
cp -r "$RELA/." "$W/relA/"; cp -r "$RELB/." "$W/relB/"
for b in baft baft-agent baft-bcc; do
  cmp -s "$W/relA/dist/$b-linux-$ARCH" "$W/relB/dist/$b-linux-$ARCH" && fail "releases A and B ship the same $b; the update would prove nothing"
done

renv() { # release-dir: the environment an install from that signed release needs
  printf '%s\n' BAFT_INSTALL_FROM=release "BAFT_RELEASE_URL=file://$1/dist" "BAFT_ROOT_PUB=$(cat "$1/root.pub")" \
    "BAFT_REVOCATIONS_URL=file://$1/revocations.json" BAFT_NONINTERACTIVE=1 BAFT_TUNE_NETWORK=0
}
inst() { # release-dir out-name args...: run install.sh from that release
  local rel="$1" out="$2"; shift 2
  local e=(); mapfile -t e < <(renv "$rel")
  env "${e[@]}" bash install.sh "$@" >"$W/$out.out" 2>&1
}
sum() { sha256sum "$1" | awk '{print $1}'; }
started() { systemctl show -p ActiveEnterTimestampMonotonic --value "$1"; }
state_version() { python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' /opt/baft/release-state.json; }

STEP="nothing installed"
if inst "$W/relB" none --update --yes; then fail "--update succeeded on a server with nothing installed"; fi
grep -q "nothing to update" "$W/none.out" || fail "no clear message on an empty server: $(tail -n 3 "$W/none.out")"

STEP="install A"
log "installing the panel and an agent from release A"
inst "$W/relA" panel-a --bcc-only --bcc-listen 127.0.0.1:18090 || fail "panel install from A failed"
JOB_KEY="$(/usr/local/bin/baft-bcc jobkey show --job-key-file /etc/baft-bcc/job-key)"
head -c 24 /dev/urandom | base64 | tr -d '/+=' >"$W/token"; chmod 600 "$W/token"
BAFT_BCC_JOB_KEY="$JOB_KEY" BAFT_AGENT_TOKEN_FILE="$W/token" BAFT_AGENT_ALLOW_HTTP=1 BAFT_AGENT_INTERVAL=7s \
  inst "$W/relA" agent-a --agent-only --bcc-url http://127.0.0.1:18090 --node-id upd-e2e --no-ingress-chain ||
  fail "agent install from A failed"
for u in baft-bcc baft-agent; do systemctl is-active --quiet "$u" || fail "$u is not running after the install from A"; done
for b in baft baft-pair baft-agent baft-bcc; do
  [[ "$(sum /usr/local/bin/$b)" == "$(sum "$W/relA/dist/$b-linux-$ARCH")" ]] || fail "$b is not release A's"
done
[[ "$(state_version)" == v0.0.1-upd ]] || fail "release state is $(state_version), want v0.0.1-upd"

KEEP=(/etc/baft-bcc/admin-token /etc/baft-bcc/access.json /etc/baft-bcc/job-key /etc/baft-agent/token
  /etc/baft-agent/bcc-job.pub /etc/systemd/system/baft-bcc.service /etc/systemd/system/baft-agent.service)
for f in "${KEEP[@]}"; do sum "$f"; done >"$W/keep.before"

STEP="update plan"
log "install.sh --update --plan: shows every component, changes nothing"
inst "$W/relB" plan --update --plan || fail "--update --plan failed"
grep -q "found: panel" "$W/plan.out" && grep -q "found: agent" "$W/plan.out" || fail "the plan did not list both components"
grep -qi "nothing was changed" "$W/plan.out" || fail "the plan did not say that nothing changed"
[[ "$(sum /usr/local/bin/baft-bcc)" == "$(sum "$W/relA/dist/baft-bcc-linux-$ARCH")" ]] || fail "--plan changed baft-bcc"

STEP="refusals"
if inst "$W/relB" refuse --update --role ex --yes; then fail "--update accepted --role"; fi
grep -q "takes only" "$W/refuse.out" || fail "--update --role was not refused with a reason"

STEP="update"
log "install.sh --update --yes: panel, then agent, to release B"
inst "$W/relB" update --update --yes || fail "the update to B failed"
grep -E "updating (panel|agent)" "$W/update.out" | head -n 1 | grep -q "updating panel" || fail "the panel was not updated first"
for b in baft baft-pair baft-agent baft-bcc; do
  [[ "$(sum /usr/local/bin/$b)" == "$(sum "$W/relB/dist/$b-linux-$ARCH")" ]] || fail "$b was not updated to release B"
done
[[ "$(state_version)" == v0.0.2-upd ]] || fail "release state is $(state_version), want v0.0.2-upd"
for f in "${KEEP[@]}"; do sum "$f"; done >"$W/keep.after"
diff -u "$W/keep.before" "$W/keep.after" || fail "the update changed a secret or a unit"
grep -q -- "--interval 7s" /etc/systemd/system/baft-agent.service || fail "the agent's custom interval was lost"
[[ ! -f /etc/systemd/system/baft-ingress-chain.service ]] || fail "the update added BAFT's firewall chain to an agent installed without it"
for u in baft-bcc baft-agent; do systemctl is-active --quiet "$u" || fail "$u is not running after the update"; done
[[ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18090/ || true)" =~ ^[2345] ]] ||
  fail "the panel does not answer on its address after the update"

STEP="plain rerun"
log "a plain rerun on the updated server: current, nothing restarted"
t_bcc="$(started baft-bcc)"; t_agent="$(started baft-agent)"
inst "$W/relB" rerun --yes || fail "the plain rerun failed"
grep -q "found: panel" "$W/rerun.out" || fail "a plain rerun did not take the update path"
[[ "$(started baft-bcc)" == "$t_bcc" && "$(started baft-agent)" == "$t_agent" ]] || fail "a rerun on a current server restarted a service"

log "PASS: one command updated the panel and the agent to the signed release, kept every secret and setting, and a rerun changed nothing"
