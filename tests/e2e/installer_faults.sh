#!/usr/bin/env bash
# Failure-injection and "nothing changes before confirmation" checks for
# install.sh on a throwaway fake tree: every path lives under one temp
# directory, systemctl is a stateful stub (active/enabled files) and apt-get
# only records its calls, so no systemd is needed. Needs root (install.sh
# checks) and Go (a signed release is built with throwaway keys).
#
#   sudo tests/e2e/installer_faults.sh
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
W="$(mktemp -d)"
trap '[[ -z "${SRV_PID:-}" ]] || kill "$SRV_PID" 2>/dev/null || true; rm -rf "$W"' EXIT
log() { printf '[faults] %s\n' "$*"; }
# Diagnostics for CI (job logs are not always retrievable; annotations are).
on_err() { local rc=$? line=$1 cmd=$2
  { echo "line $line (rc $rc): $cmd"; for f in "$W"/e "$W"/o "$W"/ex.err "$W"/ir.err; do [[ -f "$f" ]] && { echo "== $f"; tail -n 12 "$f"; }; done; } >&2
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    printf '::error title=installer-faults::line %s (rc %s): %s\n' "$line" "$rc" "${cmd//%/%25}"
  fi
}
trap 'on_err "$LINENO" "$BASH_COMMAND"' ERR
fail() { printf '[faults] FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p "$W/stubs" "$W/sysd" "$W/state"
cat >"$W/stubs/systemctl" <<'STUB'
#!/bin/sh
S="$STUB_DIR/state"
echo "$@" >>"$STUB_DIR/calls"
case "$1" in
  is-active) [ -f "$S/active.$2" ] && echo active || echo inactive ;;
  is-enabled) [ -f "$S/enabled.$2" ] && echo enabled || echo disabled ;;
  enable) touch "$S/enabled.$2" ;;
  disable) rm -f "$S/enabled.$2" ;;
  start) touch "$S/active.$2" ;;
  restart)
    if [ "${STUB_FAIL_RESTART:-0}" = 1 ]; then
      rm -f "$S/active.$2"
      exit 1
    fi
    touch "$S/active.$2" ;;
  stop) rm -f "$S/active.$2" ;;
  show) echo 4242 ;;
esac
exit 0
STUB
cat >"$W/stubs/apt-get" <<'STUB'
#!/bin/sh
echo "$@" >>"$STUB_DIR/apt.log"
exit 0
STUB
chmod +x "$W/stubs/"*

# A PATH that has everything except curl (a missing prerequisite).
mkdir -p "$W/nocurl"
for d in /usr/bin /bin /usr/sbin /sbin; do
  for f in "$d"/*; do
    n="$(basename "$f")"
    [[ "$n" == curl || -e "$W/nocurl/$n" ]] && continue
    ln -s "$f" "$W/nocurl/$n" 2>/dev/null || true
  done
done

log "build a signed release (v1), a newer one (v2) with the same root"
export BAFT_LOCAL_RELEASE_ROOT="$W/root"
scripts/release/local_release.sh "$W/rel1" v0.0.1-f >/dev/null
scripts/release/local_release.sh "$W/rel2" v0.0.2-f >/dev/null
unset BAFT_LOCAL_RELEASE_ROOT

BASEPATH="$W/stubs:$PATH"
common=(STUB_DIR="$W" BAFT_USER=root BAFT_INSTALL_FROM=release BAFT_ROOT_PUB="$(cat "$W/rel1/root.pub")"
  BAFT_BIN="$W/bin/baft" BAFT_PAIR_BIN="$W/bin/baft-pair" BAFT_AGENT_BIN="$W/bin/baft-agent"
  BAFT_SYSTEMD_DIR="$W/sysd" BAFT_AGENT_STATE_DIR="$W/ags" BAFT_METRICS_LISTEN=127.0.0.1:9191)
rel() { # release dir -> env
  echo "BAFT_RELEASE_URL=file://$1/dist" "BAFT_REVOCATIONS_URL=file://$1/revocations.json"; }
# role env for an instance named $1 (ex/ir)
role_env() { echo "BAFT_SERVICE=baft-$1" "BAFT_PREFIX=$W/opt-$1" "BAFT_CONFIG_DIR=$W/etc-$1" "BAFT_STATE_DIR=$W/var-$1" "BAFT_RELEASE_STATE=$W/opt-$1/release-state.json"; }
run() { # PATH-prefix env... -- args
  local p="$BASEPATH"; if [[ "$1" == nocurl ]]; then p="$W/stubs:$W/nocurl"; shift; fi
  env PATH="$p" "${common[@]}" "$@" <"${RUN_STDIN:-/dev/null}"
}
snapshot() { # everything an install could touch, minus stub bookkeeping
  { { find "$W/bin" "$W/sysd" "$W"/etc-* "$W"/var-* "$W"/opt-* "$W/ag" "$W/ags" "$W/src" -xdev 2>/dev/null || true; } | sort |
      while read -r f; do [[ -f "$f" ]] && sha256sum "$f" && stat -c '%n %U:%G %a' "$f" || echo "$f"; done
    ls "$W/state" | sort; { ls /usr/local/go 2>/dev/null || true; } | head -1; } | sha256sum
}
active() { [[ -f "$W/state/active.$1.service" ]]; }
enabled() { [[ -f "$W/state/enabled.$1.service" ]]; }

log "0. service-user path preflight is non-mutating and names the first blocked parent"
if id -u nobody >/dev/null 2>&1; then
  chmod 0755 "$W"
  mkdir -p "$W/preflight-block/etc"
  chmod 0700 "$W/preflight-block"
  PF0="$(snapshot)"
  rc=0
  run $(rel "$W/rel1") BAFT_USER=nobody BAFT_SERVICE=baft-preflight \
    BAFT_PREFIX="$W/preflight-opt" BAFT_CONFIG_DIR="$W/preflight-block/etc/baft" \
    BAFT_STATE_DIR="$W/preflight-state" BAFT_RELEASE_STATE="$W/preflight-opt/release-state.json" \
    BAFT_NONINTERACTIVE=1 bash install.sh --role ex --public-address 127.0.0.1 --yes \
    >"$W/o" 2>"$W/e" || rc=$?
  [[ "$rc" != 0 ]] || fail "blocked service-user parent unexpectedly passed preflight"
  grep -q "preflight: service user 'nobody' cannot traverse '$W/preflight-block'" "$W/e" ||
    { cat "$W/e"; fail "preflight did not name the first blocked parent"; }
  grep -Eq 'mode=700 uid=[0-9]+ gid=[0-9]+' "$W/e" ||
    { cat "$W/e"; fail "preflight diagnostic omitted mode/uid/gid"; }
  [[ "$(snapshot)" == "$PF0" ]] || fail "path preflight mutated the host"

  chmod 0755 "$W/preflight-block"
else
  log "skip unprivileged preflight fixture: nobody user is unavailable"
fi

log "install a paired EX and IR from release v1 on the fake tree"
mkfifo "$W/ff"; exec 3<>"$W/ff"
RUN_STDIN="$W/ff" run $(rel "$W/rel1") $(role_env ex) BAFT_NONINTERACTIVE=0 BAFT_PORT=8443 BAFT_TARGET=127.0.0.1:2443 \
  bash install.sh --role ex --public-address 127.0.0.1 >"$W/ex.out" 2>"$W/ex.err" &
EXPID=$!
for _ in $(seq 100); do grep -q '^BAFTPAIR1:' "$W/ex.out" 2>/dev/null && break; sleep 0.2; done
CODE="$(grep -m1 '^BAFTPAIR1:' "$W/ex.out")"
run $(rel "$W/rel1") $(role_env ir) BAFT_NONINTERACTIVE=1 BAFT_PAIRING_CODE="$CODE" BAFT_METRICS_LISTEN=127.0.0.1:9192 \
  BAFT_ROUTE_LISTEN=127.0.0.1:1443 bash install.sh --role ir >"$W/ir.out" 2>"$W/ir.err"
grep -m1 '^BAFTREPLY1:' "$W/ir.out" >&3
wait "$EXPID"; exec 3>&-
active baft-ex && enabled baft-ex || { cat "$W/ex.err" "$W/ir.err" | tail -20; ls "$W/state"; fail "EX not installed"; }

# Leading NAME=value arguments are environment; the rest are installer arguments.
EXI() { local e=(); while [[ $# -gt 0 && "$1" == *=* ]]; do e+=("$1"); shift; done
  run $(rel "$W/rel1") $(role_env ex) BAFT_NONINTERACTIVE=1 BAFT_PORT=8443 BAFT_TARGET=127.0.0.1:2443 "${e[@]}" bash install.sh --role ex --public-address 127.0.0.1 "$@"; }

# ---------------------------------------------------------------------------
log "1. nothing on the host changes before the plan is confirmed"
rm -f "$W/state/active.baft-ex.service"            # the install now needs a (changing) repair
: >"$W/apt.log"
S0="$(snapshot)"
rc=0; EXI >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 4 ]] || { cat "$W/e"; fail "changing plan without --yes exited $rc, want 4"; }
[[ "$(snapshot)" == "$S0" ]] || fail "a refused plan changed the host"
[[ ! -e "$W/bin/baft.new" && ! -e "$W/bin/baft-pair.new" ]] || fail "*.new left in the bin directory"

log "1b. a missing prerequisite on an existing install: no apt before confirmation"
rc=0; run nocurl $(rel "$W/rel1") $(role_env ex) BAFT_NONINTERACTIVE=1 bash install.sh --role ex --public-address 127.0.0.1 >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 4 ]] || { cat "$W/e"; fail "missing curl without --yes exited $rc, want 4"; }
grep -q 'packages' "$W/e" || { cat "$W/e"; fail "the plan does not list the packages"; }
[[ ! -s "$W/apt.log" ]] || fail "apt ran before confirmation: $(cat "$W/apt.log")"
[[ "$(snapshot)" == "$S0" ]] || fail "missing-prerequisite refusal changed the host"

log "1c. --from-source on an existing install is refused the same way (no Go, no source tree)"
rc=0; run nocurl BAFT_INSTALL_FROM=source BAFT_REPO_URL="file://$ROOT" BAFT_REF=HEAD $(role_env ex) BAFT_NONINTERACTIVE=1 \
  bash install.sh --role ex --public-address 127.0.0.1 >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 4 ]] || { cat "$W/e"; fail "source rerun without --yes exited $rc, want 4"; }
[[ ! -s "$W/apt.log" && ! -e "$W/opt-ex/src" ]] || fail "apt or the source tree changed before confirmation"
[[ "$(snapshot)" == "$S0" ]] || fail "source refusal changed the host"

log "1d. a broken install is refused before any mutation, even with --yes and a missing prerequisite"
cp "$W/etc-ex/baft.yaml" "$W/baft.yaml.good"
printf 'not: [valid\n' >"$W/etc-ex/baft.yaml"
S1="$(snapshot)"
rc=0; run nocurl $(rel "$W/rel1") $(role_env ex) BAFT_NONINTERACTIVE=1 bash install.sh --role ex --public-address 127.0.0.1 --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 3 ]] || { cat "$W/e"; fail "broken install exited $rc, want 3"; }
[[ ! -s "$W/apt.log" ]] || fail "apt ran on a broken install"
[[ "$(snapshot)" == "$S1" ]] || fail "a refused broken install changed the host"
cp "$W/baft.yaml.good" "$W/etc-ex/baft.yaml"; chown root:root "$W/etc-ex/baft.yaml"; chmod 0640 "$W/etc-ex/baft.yaml"
touch "$W/state/active.baft-ex.service"

log "1e. valid config + Noise key + PKI + unit, but the installed baft binary is MISSING: binary only, config and pairing untouched"
cfgsum() { sha256sum "$W/etc-ex/baft.yaml" "$W/etc-ex/noise-key.json" "$W"/etc-ex/pki/* "$W/sysd/baft-ex.service"; stat -c '%n %U:%G %a' "$W/etc-ex/baft.yaml"; }
K0="$(cfgsum)"
rm -f "$W/bin/baft"
rc=0; EXI --plan --json >"$W/plan.json" 2>/dev/null || rc=$?
python3 - "$W/plan.json" <<'PY' || fail "plan for a missing binary re-pairs or overwrites the config"
import json, sys
p = {s["item"]: s["action"] for s in json.load(open(sys.argv[1]))["steps"]}
assert p["config"] == "keep" and p["pairing"] == "skip" and p["baft binary"] == "create", p
PY
rc=0; EXI >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 4 ]] || { cat "$W/e"; fail "repair without --yes exited $rc"; }
EXI --yes >"$W/o" 2>"$W/e" || { cat "$W/e"; fail "repair of a missing binary failed"; }
grep -q '^BAFTPAIR1:' "$W/o" && fail "a repair printed a pairing code"
grep -q 'config .*keep' "$W/e" && grep -q 'pairing .*skip' "$W/e" || { cat "$W/e"; fail "final plan does not keep the config and skip pairing"; }
[[ "$(cfgsum)" == "$K0" ]] || fail "config, key, PKI or unit changed while repairing a missing binary"
[[ -x "$W/bin/baft" ]] || fail "binary not restored"
active baft-ex && enabled baft-ex || fail "service not running after the repair"

log "1f. INVALID config + missing binary: refused, config unchanged, binary still missing"
printf 'not: [valid\n' >"$W/etc-ex/baft.yaml"
chown root:root "$W/etc-ex/baft.yaml"; chmod 0640 "$W/etc-ex/baft.yaml"
rm -f "$W/bin/baft"
K1="$(cfgsum)"; S1="$(snapshot)"
rc=0; EXI --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 3 ]] || { cat "$W/e"; fail "invalid config + missing binary exited $rc, want 3"; }
grep -q '^BAFTPAIR1:' "$W/o" && fail "paired over an invalid config"
[[ "$(cfgsum)" == "$K1" && ! -e "$W/bin/baft" ]] || fail "a refused install changed config or binary"
[[ "$(snapshot)" == "$S1" ]] || fail "a refused install changed the host"
cp "$W/baft.yaml.good" "$W/etc-ex/baft.yaml"; chown root:root "$W/etc-ex/baft.yaml"; chmod 0640 "$W/etc-ex/baft.yaml"
EXI --yes >"$W/o" 2>"$W/e" || { cat "$W/e"; fail "restoring the binary failed"; }

# ---------------------------------------------------------------------------
cfg_hash() { sha256sum "$W/etc-ex/baft.yaml" "$W/etc-ex/noise-key.json" "$W"/etc-ex/pki/* "$W/sysd/baft-ex.service" "$W/opt-ex/release-state.json" "$W/bin/baft"; }
log "2a. failure after restart/enable, service was ACTIVE + ENABLED: restored to active + enabled, old binary"
C0="$(cfg_hash)"
rc=0; EXI $(rel "$W/rel2") BAFT_TEST_FAIL_AT=pre-commit --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" != 0 ]] || fail "injected failure reported success"
grep -q 'rolling back' "$W/e" || { cat "$W/e"; fail "no rollback"; }
active baft-ex && enabled baft-ex || fail "service state not restored (active+enabled)"
[[ "$(cfg_hash)" == "$C0" ]] || fail "files not restored after a failed upgrade"

log "2b. failure after start/enable, service was INACTIVE + DISABLED: back to inactive + disabled"
rm -f "$W/state/active.baft-ex.service" "$W/state/enabled.baft-ex.service"
C0="$(cfg_hash)"
rc=0; EXI BAFT_TEST_FAIL_AT=pre-commit --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" != 0 ]] || fail "injected failure reported success"
active baft-ex && fail "a service that was stopped is left running after rollback"
enabled baft-ex && fail "a service that was disabled is left enabled after rollback"
[[ "$(cfg_hash)" == "$C0" ]] || fail "files changed by a rolled-back repair"
log "2b'. the same repair without a failure starts and enables it"
EXI --yes >"$W/o" 2>"$W/e" || { cat "$W/e"; fail "repair failed"; }
active baft-ex && enabled baft-ex || fail "repair did not start and enable the service"

log "2c. failure on a FRESH role install: unit, key, PKI removed and nothing left enabled or running"
run $(rel "$W/rel1") $(role_env fr) BAFT_SERVICE=baft-fr BAFT_NONINTERACTIVE=1 BAFT_PORT=8444 BAFT_TEST_FAIL_AT=pre-commit \
  bash install.sh --role ex --public-address 127.0.0.1 >"$W/o" 2>"$W/e" && fail "injected failure reported success"
[[ ! -e "$W/sysd/baft-fr.service" && ! -e "$W/etc-fr/noise-key.json" && ! -e "$W/etc-fr/pki/ca.key" && ! -e "$W/etc-fr/baft.yaml" ]] || fail "files of the failed fresh install were left"
enabled baft-fr && fail "enablement of a removed unit was left behind"
active baft-fr && fail "a failed fresh install left the service running"

# ---------------------------------------------------------------------------
AG=(BAFT_AGENT_DIR="$W/ag" BAFT_BCC_URL=https://bcc.example.com BAFT_NODE_ID=n1 BAFT_BCC_JOB_KEY=jobkeyabc BAFT_AGENT_TOKEN=tok1
    BAFT_PREFIX="$W/opt-ag" BAFT_CONFIG_DIR="$W/etc-ag" BAFT_STATE_DIR="$W/var-ag" BAFT_RELEASE_STATE="$W/opt-ag/release-state.json" BAFT_NONINTERACTIVE=1)
AGI() { local e=(); while [[ $# -gt 0 && "$1" == *=* ]]; do e+=("$1"); shift; done
  run $(rel "$W/rel1") "${AG[@]}" "${e[@]}" bash install.sh --agent-only "$@"; }
log "3a. agent: failure on a FRESH install leaves nothing behind"
AGI BAFT_TEST_FAIL_AT=pre-commit >"$W/o" 2>"$W/e" && fail "injected failure reported success"
[[ ! -e "$W/sysd/baft-agent.service" && ! -e "$W/ag/token" && ! -e "$W/ag/bcc-job.pub" ]] || fail "agent files left after a failed fresh install"
enabled baft-agent && fail "agent enablement left behind"
active baft-agent && fail "agent left running"
AGI >"$W/o" 2>"$W/e" || { cat "$W/e"; fail "agent install"; }
active baft-agent && enabled baft-agent || fail "agent not running and enabled"
AH() { sha256sum "$W/ag"/* "$W/sysd/baft-agent.service"; }
log "3b. agent: a different token fails after restart -> old token, still active + enabled"
A0="$(AH)"
AGI BAFT_AGENT_TOKEN=tok2 BAFT_TEST_FAIL_AT=pre-commit --yes >"$W/o" 2>"$W/e" && fail "injected failure reported success"
[[ "$(AH)" == "$A0" ]] || fail "agent files not restored"
active baft-agent && enabled baft-agent || fail "agent state not restored (active+enabled)"
log "3c. agent: inactive + disabled before, failure after start/enable -> inactive + disabled"
rm -f "$W/state/active.baft-agent.service" "$W/state/enabled.baft-agent.service"
AGI BAFT_TEST_FAIL_AT=pre-commit --yes >"$W/o" 2>"$W/e" && fail "injected failure reported success"
active baft-agent && fail "agent that was stopped is left running"
enabled baft-agent && fail "agent that was disabled is left enabled"
[[ "$(AH)" == "$A0" ]] || fail "agent files changed"

log "4. a failed upgrade with a failed rollback restart reports incomplete and leaves the service stopped"
C0="$(cfg_hash)"
rc=0
EXI $(rel "$W/rel2") STUB_FAIL_RESTART=1 --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" != 0 ]] || fail "restart failure reported success"
grep -q 'rollback incomplete' "$W/e" || { cat "$W/e"; fail "failed rollback restart was hidden"; }
! grep -q 'rolled back to the previous binaries' "$W/e" || fail "incomplete rollback claimed full success"
active baft-ex && fail "failed rollback restart left EX marked active"
[[ "$(cfg_hash)" == "$C0" ]] || fail "binary and configuration were not restored"
log "5. BCC failed binary rollback retains the verified candidate without replacing state"
BCC_STATE="$W/var-bcc/state.db"
mkdir -p "$(dirname "$BCC_STATE")"
printf 'existing BCC state; fake service does not open it\n' >"$BCC_STATE"
bcc_env=(BAFT_PREFIX="$W/opt-bcc" BAFT_RELEASE_STATE="$W/opt-bcc/release-state.json"
  BAFT_BCC_BIN="$W/bin/baft-bcc" BAFT_BCC_CONFIG_DIR="$W/etc-bcc"
  BAFT_BCC_STATE_DIR="$W/var-bcc" BAFT_BCC_STATE_FILE="$BCC_STATE"
  BAFT_BCC_BOOTSTRAP_SCRIPT="$W/opt-bcc/bcc-bootstrap/install.sh"
  BAFT_BCC_SERVICE=baft-bcc-fault BAFT_NONINTERACTIVE=1)
run $(rel "$W/rel1") "${bcc_env[@]}" bash install.sh --bcc-only --yes >"$W/o" 2>"$W/e" ||
  { cat "$W/e"; fail "BCC fake-systemd fixture install failed"; }
active baft-bcc-fault || fail "BCC fixture did not start"
printf '\n' >>"$W/bin/baft-bcc" # Still executable, but distinct from the staged candidate.
"$W/bin/baft-bcc" --help >/dev/null 2>&1 || fail "modified old BCC fixture does not run"
old_bcc="$(sha256sum "$W/bin/baft-bcc" | cut -d ' ' -f1)"
old_state="$(sha256sum "$BCC_STATE" | cut -d ' ' -f1)"
rc=0
run $(rel "$W/rel2") "${bcc_env[@]}" STUB_FAIL_RESTART=1 bash install.sh --bcc-only --yes >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" != 0 ]] || fail "failed BCC restart reported success"
grep -q 'rollback incomplete' "$W/e" || { cat "$W/e"; fail "BCC rollback failure was hidden"; }
active baft-bcc-fault && fail "failed BCC rollback left service active"
[[ "$(sha256sum "$BCC_STATE" | cut -d ' ' -f1)" == "$old_state" ]] || fail "BCC state changed during failed apply"
[[ "$(sha256sum "$W/bin/baft-bcc" | cut -d ' ' -f1)" == "$old_bcc" ]] || fail "older BCC binary was not restored"
candidate="$(find "$W/opt-bcc/backups" -name bcc-candidate-on-failed-apply -type f -print -quit)"
[[ -n "$candidate" && -f "$candidate" ]] || fail "compatible BCC recovery candidate was lost"
[[ "$(stat -c '%a %U' "$candidate")" == "700 root" ]] || fail "BCC recovery candidate is not root-only"
( cd "$(dirname "$candidate")" && sha256sum -c bcc-candidate.sha256 ) >/dev/null ||
  fail "BCC recovery candidate checksum mismatch"
case "$(uname -m)" in x86_64) bcc_arch=amd64 ;; aarch64) bcc_arch=arm64 ;; *) fail "unsupported BCC test architecture" ;; esac
cmp -s "$candidate" "$W/rel2/dist/baft-bcc-linux-$bcc_arch" ||
  fail "retained BCC binary differs from verified release candidate"

# Failures at each BCC apply point of an upgrade put back the binary, the
# bootstrap script, the unit, the recorded release, the keys/access files and
# the service state, and never touch the live state database.
bcc_hash() { sha256sum "$W/bin/baft-bcc" "$W/opt-bcc/bcc-bootstrap/install.sh" "$W/sysd/baft-bcc-fault.service" \
  "$W/opt-bcc/release-state.json" "$BCC_STATE" "$W"/etc-bcc/*; }
run $(rel "$W/rel1") "${bcc_env[@]}" bash install.sh --bcc-only --yes >"$W/o" 2>"$W/e" ||
  { cat "$W/e"; fail "BCC reinstall of v1 after the failed restart"; }
active baft-bcc-fault && enabled baft-bcc-fault || fail "BCC v1 not active + enabled"
for point in post-binaries post-bcc-bootstrap pre-commit; do
  log "6. BCC upgrade failure at $point: v1 files, release and service state restored; state DB untouched"
  # Same installer bytes in v1 and v2 keep the bootstrap script, so make the
  # installed copy differ to send the upgrade through the bootstrap step.
  [[ "$point" == post-bcc-bootstrap ]] && printf '\n' >>"$W/opt-bcc/bcc-bootstrap/install.sh"
  B0="$(bcc_hash)"
  rc=0
  run $(rel "$W/rel2") "${bcc_env[@]}" BAFT_TEST_FAIL_AT="$point" bash install.sh --bcc-only --yes >"$W/o" 2>"$W/e" || rc=$?
  [[ "$rc" != 0 ]] || fail "injected BCC failure at $point reported success"
  grep -q "injected failure at $point" "$W/e" || { cat "$W/e"; fail "BCC upgrade never reached $point"; }
  grep -q 'rolling back' "$W/e" || { cat "$W/e"; fail "no BCC rollback after $point"; }
  grep -q 'rollback incomplete' "$W/e" && { cat "$W/e"; fail "BCC rollback after $point reported incomplete"; }
  [[ "$(bcc_hash)" == "$B0" ]] || { diff <(echo "$B0") <(bcc_hash) || true; fail "BCC files changed by a rolled-back upgrade ($point)"; }
  active baft-bcc-fault && enabled baft-bcc-fault || fail "BCC service state not restored after $point"
done
log "6'. the same BCC upgrade without a failure records v2"
run $(rel "$W/rel2") "${bcc_env[@]}" bash install.sh --bcc-only --yes >"$W/o" 2>"$W/e" ||
  { cat "$W/e"; fail "BCC upgrade to v2 failed"; }
grep -q 'v0.0.2-f' "$W/opt-bcc/release-state.json" || fail "BCC release state does not record v2"
[[ "$(sha256sum "$BCC_STATE" | cut -d ' ' -f1)" == "$old_state" ]] || fail "BCC state DB changed by the upgrade"
active baft-bcc-fault && enabled baft-bcc-fault || fail "BCC v2 not active + enabled"

# ---------------------------------------------------------------------------
# 7. `baft update apply`: the real CLI runs the real signed installer. The CLI
#    is built from this tree; the release comes from a local https server and
#    is signed by the throwaway root (BAFT_ROOT_PUB overrides the pinned root
#    exactly as it does for install.sh). systemctl is the stateful stub with
#    its state directory baked in, because the CLI builds the installer's
#    environment from scratch (STUB_DIR is deliberately not passed on).
log "7. baft update apply: a transport service one release behind is updated by the signed installer"
mkdir -p "$W/cli" "$W/stubs2"
# The CLI runs the installer with a fixed PATH (never the caller's), so the lab's
# stub systemctl/apt-get are put on that PATH at build time, not through the environment.
go build -ldflags "-X main.trustedPath=$W/stubs2:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" -o "$W/cli/baft" ./cmd/baft
for f in systemctl apt-get; do sed "s#\$STUB_DIR#$W#g" "$W/stubs/$f" >"$W/stubs2/$f"; chmod +x "$W/stubs2/$f"; done
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$W/srv.key" -out "$W/srv.crt" -subj "/CN=127.0.0.1" \
  -addext "subjectAltName=IP:127.0.0.1" -days 1 >/dev/null 2>&1
SRV_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')"
cat >"$W/serve.py" <<PY
import http.server, ssl, functools, sys
h = functools.partial(http.server.SimpleHTTPRequestHandler, directory="$W")
s = http.server.ThreadingHTTPServer(("127.0.0.1", $SRV_PORT), h)
c = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); c.load_cert_chain("$W/srv.crt", "$W/srv.key")
s.socket = c.wrap_socket(s.socket, server_side=True); s.serve_forever()
PY
python3 "$W/serve.py" >/dev/null 2>&1 & SRV_PID=$!
for _ in $(seq 50); do curl -fsS --cacert "$W/srv.crt" "https://127.0.0.1:$SRV_PORT/rel2/revocations.json" >/dev/null 2>&1 && break; sleep 0.2; done
curl -fsS --cacert "$W/srv.crt" "https://127.0.0.1:$SRV_PORT/rel2/revocations.json" >/dev/null || fail "the local https release server did not start"

# A paired EX + IR from release v1 on their own instance names.
mkfifo "$W/ff2"; exec 4<>"$W/ff2"
RUN_STDIN="$W/ff2" run $(rel "$W/rel1") $(role_env ux) BAFT_NONINTERACTIVE=0 BAFT_PORT=8453 BAFT_TARGET=127.0.0.1:2443 BAFT_METRICS_LISTEN=127.0.0.1:9291 \
  bash install.sh --role ex --public-address 127.0.0.1 >"$W/ux.out" 2>"$W/ux.err" &
UXPID=$!
for _ in $(seq 100); do grep -q '^BAFTPAIR1:' "$W/ux.out" 2>/dev/null && break; sleep 0.2; done
UXCODE="$(grep -m1 '^BAFTPAIR1:' "$W/ux.out")"
run $(rel "$W/rel1") $(role_env ui) BAFT_NONINTERACTIVE=1 BAFT_PAIRING_CODE="$UXCODE" BAFT_METRICS_LISTEN=127.0.0.1:9292 \
  BAFT_ROUTE_LISTEN=127.0.0.1:1543 bash install.sh --role ir >"$W/ui.out" 2>"$W/ui.err"
grep -m1 '^BAFTREPLY1:' "$W/ui.out" >&4
wait "$UXPID"; exec 4>&-
active baft-ui && enabled baft-ui || { tail -20 "$W/ui.err" "$W/ux.err"; fail "the IR to update was not installed"; }
grep -q 'v0.0.1-f' "$W/opt-ui/release-state.json" || fail "the IR does not record v1"

UPENV() { # all flags explicit -> the CLI, the lab root and the https release
  env PATH="$W/stubs2:$PATH" BAFT_ROOT_PUB="$(cat "$W/rel1/root.pub")" SSL_CERT_FILE="$W/srv.crt" CURL_CA_BUNDLE="$W/srv.crt" \
    BAFT_RELEASE_URL="https://127.0.0.1:$SRV_PORT/${UP_REL:-rel2}/dist" BAFT_REVOCATIONS_URL="https://127.0.0.1:$SRV_PORT/${UP_REL:-rel2}/revocations.json" \
    "$W/cli/baft" update "$@" </dev/null; }
UP() { # args... -> the IR instance, with every service of the lab visible
  UPENV "$@" --unit-dir "$W/sysd" --service baft-ui --release-state "$W/opt-ui/release-state.json"; }
# The lab's services share one binary directory, so more than one of them runs
# the baft binary an update replaces: exactly what the CLI refuses (7b2). A run
# that is meant to succeed sees a unit directory in which only its own service
# runs that binary, as on a real host with a single transport service.
mkdir -p "$W/sysd-ui"; cp "$W/sysd/baft-ui.service" "$W/sysd-ui/"
UPI() { UPENV "$@" --unit-dir "$W/sysd-ui" --service baft-ui --release-state "$W/opt-ui/release-state.json"; }
AO=(--allow-root-override)   # the lab release is signed by a throwaway root taken from the environment
ui_creds() { { sha256sum "$W/etc-ui/baft.yaml" "$W/etc-ui/noise-key.json" "$W/sysd-ui/baft-ui.service" 2>/dev/null || true; } | sha256sum; }

log "7a. check: the IR's baft binary differs from the signed release"
rc=0; UP check >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 3 ]] || { cat "$W/o" "$W/e"; fail "check exited $rc, want 3 (update available)"; }
grep -q 'transport .*OUTDATED' "$W/o" || { cat "$W/o"; fail "the IR is not reported OUTDATED"; }
grep -q 'trust root    OVERRIDDEN by BAFT_ROOT_PUB' "$W/o" || { cat "$W/o"; fail "a lab root is not announced in the report"; }

log "7b. apply --plan changes nothing, and (as root) shows the installer's plan after the download"
U0="$(snapshot)"
UPI apply --plan --only transport >"$W/o" 2>"$W/e" || { cat "$W/o" "$W/e"; fail "apply --plan failed"; }
grep -q 'This was a preview. Nothing was changed.' "$W/o" || { cat "$W/o"; fail "no preview verdict"; }
grep -q 'baft binary .*update .*verified release' "$W/o" && ! grep -q 'decided by version before the download' "$W/o" ||
  { cat "$W/o"; fail "the preview is not the plan the installer makes after the download"; }
[[ "$(snapshot)" == "$U0" ]] || fail "apply --plan changed the host"
[[ "$(ls "$W/opt-ui/update-logs" 2>/dev/null | grep -c '\.log$' || true)" == 0 ]] || fail "a preview ran an installer for real"

log "7b2. several services of this host run the baft binary an update replaces: refused, each one named"
rc=0; UP apply --plan --only transport >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 1 ]] || { cat "$W/o" "$W/e"; fail "replacing a binary under services outside the run exited $rc, want 1"; }
grep -q 'which this update replaces' "$W/o" && grep -Eq 'baft-(ex|ux) runs ' "$W/o" || { cat "$W/o"; fail "the refusal does not name the other services"; }
[[ "$(snapshot)" == "$U0" ]] || fail "a refused plan changed the host"

log "7c. an agent that is not running is refused (real installer says REPAIR_REQUIRED), nothing changes"
U1="$(snapshot)"
rc=0
UPENV apply --yes "${AO[@]}" --only agent --unit-dir "$W/sysd" --service baft-ex --release-state "$W/opt-ag/release-state.json" \
  >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 1 ]] || { cat "$W/o" "$W/e"; fail "an agent that is not running was not refused (exit $rc)"; }
grep -q 'REPAIR_REQUIRED' "$W/o" || { cat "$W/o"; fail "the refusal does not name the installer's state"; }
[[ "$(snapshot)" == "$U1" ]] || fail "a refused apply changed the host"

log "7c2. --yes with a release root / source taken from the environment is refused unless allowed explicitly"
rc=0; UPI apply --yes --only transport >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 1 ]] || { cat "$W/o" "$W/e"; fail "--yes with BAFT_ROOT_PUB in the environment exited $rc, want 1"; }
grep -q -- '--allow-root-override' "$W/e" && grep -q 'BAFT_ROOT_PUB' "$W/o" || { cat "$W/o" "$W/e"; fail "the refusal does not name the variable and the flag"; }
[[ "$(snapshot)" == "$U1" ]] || fail "a refused apply changed the host"

cp "$W/opt-ui/release-state.json" "$W/rs-ui-v1.json"   # for 7i, which puts this host back on v1
log "7d. apply --yes: binary and release state move to v2; keys, config, PKI and unit are byte-identical"
C0="$(ui_creds)"
rc=0; UPI apply --yes "${AO[@]}" --only transport >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -20 "$W/opt-ui"/update-logs/*.log 2>/dev/null; fail "apply exited $rc"; }
grep -q 'Updated to v0.0.2-f: transport' "$W/o" || { cat "$W/o"; fail "no success line"; }
[[ "$("$W/bin/baft" version)" == "baft 0.0.2-f" ]] || fail "the baft binary was not updated"
grep -q 'v0.0.2-f' "$W/opt-ui/release-state.json" || fail "the release state was not advanced"
[[ "$(ui_creds)" == "$C0" ]] || fail "credentials, config or unit changed by the update"
active baft-ui && enabled baft-ui || fail "the IR is not running after the update"
ls "$W/opt-ui/update-logs/"*-transport.log >/dev/null 2>&1 || fail "no installer log was kept"
[[ "$(stat -c '%a' "$W"/opt-ui/update-logs/*-transport.log)" == 600 ]] || fail "the installer log is not private"

log "7e. a second apply has nothing to do"
UPI apply --yes "${AO[@]}" --only transport >"$W/o" 2>"$W/e" || { cat "$W/o" "$W/e"; fail "second apply failed"; }
grep -q 'Nothing to apply' "$W/o" || { cat "$W/o"; fail "the second apply did not say there is nothing to do"; }
# The IR is current now. The agent's binary is byte-identical in v1 and v2 (only
# baft and baft-bcc carry the version), so it is current as well: the inventory
# says what is true, not what is expected.
rc=0; UP check >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o"; fail "check after the update exited $rc, want 0"; }
grep -q 'transport .*CURRENT' "$W/o" && grep -q 'agent .*CURRENT' "$W/o" || { cat "$W/o"; fail "the updated IR and the identical agent are not both CURRENT"; }
# The production case: Core is current and the shared release state already
# says v2, but the panel still runs the old binary.
BCCF=(--unit-dir "$W/sysd" --bcc-service baft-bcc-fault --release-state "$W/opt-bcc/release-state.json")
cp "$W/rel1/dist/baft-bcc-linux-$bcc_arch" "$W/bin/baft-bcc"
rc=0; UPENV check "${BCCF[@]}" --service baft-ui >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 3 ]] || { cat "$W/o"; fail "check with a panel left on v1 exited $rc, want 3"; }
grep -q 'bcc .*OUTDATED' "$W/o" || { cat "$W/o"; fail "the panel left on v1 is not reported OUTDATED"; }
grep -q 'status        UPDATE AVAILABLE' "$W/o" && grep -q 'but these components run binaries that differ from the signed release: bcc' "$W/o" ||
  { cat "$W/o"; fail "a release state that already says v2 hid the panel that is behind"; }

log "7f. BCC: refused without a recent backup, then updated to v2 with credentials, access files and the state DB untouched"
bcc_files() { sha256sum "$W"/etc-bcc/* "$BCC_STATE" "$W/sysd/baft-bcc-fault.service" | sha256sum; }
B0="$(bcc_files)"; U3="$(snapshot)"
rc=0; UPENV apply --yes "${AO[@]}" --only bcc "${BCCF[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 1 ]] || { cat "$W/o" "$W/e"; fail "BCC was updated without a backup (exit $rc)"; }
grep -q 'no BCC backup found' "$W/o" || { cat "$W/o"; fail "the refusal does not name the missing backup"; }
[[ "$(snapshot)" == "$U3" ]] || fail "a refused BCC update changed the host"
mkdir -p "$W/var-bcc/backups"; : >"$W/var-bcc/backups/bcc-daily-test.baftbak"
rc=0; UPENV apply --yes "${AO[@]}" --only bcc "${BCCF[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -20 "$W/opt-bcc"/update-logs/*.log 2>/dev/null; fail "the BCC update exited $rc"; }
grep -q 'Updated to v0.0.2-f: bcc' "$W/o" || { cat "$W/o"; fail "no BCC success line"; }
cmp -s "$W/rel2/dist/baft-bcc-linux-$bcc_arch" "$W/bin/baft-bcc" || fail "the BCC binary is not the signed v2 one"
[[ "$(bcc_files)" == "$B0" ]] || fail "BCC credentials, access files, unit or state DB changed by the update"
active baft-bcc-fault && enabled baft-bcc-fault || fail "BCC is not running after the update"
rc=0; UPENV check "${BCCF[@]}" --service baft-ui >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o"; fail "check after the BCC update exited $rc, want 0"; }

log "7g. agent: a running agent whose binary differs is updated from the inputs recovered from its unit; token, job key and unit unchanged"
touch "$W/state/active.baft-agent.service" "$W/state/enabled.baft-agent.service"
printf 'local build\n' >>"$W/bin/baft-agent"   # no longer the signed release's bytes
ag_files() { sha256sum "$W/ag"/* "$W/sysd/baft-agent.service" | sha256sum; }
AGF=(--unit-dir "$W/sysd" --service baft-ex --release-state "$W/opt-ag/release-state.json")
rc=0; UPENV check "${AGF[@]}" >"$W/o" 2>"$W/e" || rc=$?
grep -q 'agent .*OUTDATED' "$W/o" || { cat "$W/o"; fail "the agent with a different binary is not reported OUTDATED"; }
A1="$(ag_files)"
rc=0; UPENV apply --yes "${AO[@]}" --only agent "${AGF[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -30 "$W/opt-ag"/update-logs/*-agent.log 2>/dev/null; fail "the agent update exited $rc"; }
grep -q 'Updated to v0.0.2-f: agent' "$W/o" || { cat "$W/o"; fail "no agent success line"; }
case "$(uname -m)" in x86_64) ag_arch=amd64 ;; aarch64) ag_arch=arm64 ;; esac
cmp -s "$W/rel2/dist/baft-agent-linux-$ag_arch" "$W/bin/baft-agent" || fail "the agent binary is not the signed v2 one"
grep -q 'v0.0.2-f' "$W/opt-ag/release-state.json" || fail "the agent's release state was not advanced"
[[ "$(ag_files)" == "$A1" ]] || fail "the agent's token, job key, root pin or unit changed by the update"
active baft-agent && enabled baft-agent || fail "the agent is not running after the update"

log "7i. transport and agent on one host: the transport goes first and an agent with nothing left is not run again"
# baft and baft-pair are run by the transport service and also installed by the
# agent installer, which restarts only the agent. The transport must replace them,
# or its process would keep the old code while the files look current.
# One healthy host: the running IR (baft-ui) with an agent installed next to it,
# in the same prefix and unit directory, then both put back on release 1.
mkdir -p "$W/sysd-h"; cp "$W/sysd/baft-ui.service" "$W/sysd-h/"
HA=(BAFT_SYSTEMD_DIR="$W/sysd-h" BAFT_AGENT_UNIT=baft-agent-h BAFT_AGENT_DIR="$W/ag-h" BAFT_AGENT_STATE_DIR="$W/ags-h"
    BAFT_BCC_URL=https://bcc.example.com BAFT_NODE_ID=nh BAFT_BCC_JOB_KEY=jobkeyh BAFT_AGENT_TOKEN=tokh BAFT_NONINTERACTIVE=1 BAFT_METRICS_LISTEN=127.0.0.1:9393
    BAFT_SERVICE=baft-ui BAFT_PREFIX="$W/opt-ui" BAFT_CONFIG_DIR="$W/etc-ui" BAFT_STATE_DIR="$W/var-ui" BAFT_RELEASE_STATE="$W/opt-ui/release-state.json")
v1_back() { # this host's binaries and release state back to release 1
  cp "$W/rel1/dist/baft-linux-$ag_arch" "$W/bin/baft"; cp "$W/rel1/dist/baft-pair-linux-$ag_arch" "$W/bin/baft-pair"
  cp "$W/rel1/dist/baft-agent-linux-$ag_arch" "$W/bin/baft-agent"; cp "$W/rs-ui-v1.json" "$W/opt-ui/release-state.json"
  rm -rf "$W/opt-ui/update-logs"; }
v1_back
run $(rel "$W/rel1") "${HA[@]}" bash install.sh --agent-only >"$W/o" 2>"$W/e" || { cat "$W/e"; fail "7i: the agent next to the IR was not installed"; }
active baft-ui && active baft-agent-h && enabled baft-agent-h || fail "7i: the host is not healthy before the update"
HC=(--unit-dir "$W/sysd-h" --service baft-ui --agent-unit baft-agent-h --release-state "$W/opt-ui/release-state.json")
host_files() { sha256sum "$W/etc-ui"/baft.yaml "$W/etc-ui"/noise-key.json "$W/ag-h"/* "$W/sysd-h"/*.service | sha256sum; }
G0="$(host_files)"
rc=0; UPENV check "${HC[@]}" >"$W/o" 2>"$W/e" || rc=$?
grep -q 'transport .*OUTDATED' "$W/o" && grep -q 'agent .*OUTDATED' "$W/o" || { cat "$W/o"; fail "7i: both are not reported behind (only the shared baft and baft-pair differ for the agent)"; }
rc=0; UPENV apply --yes "${AO[@]}" "${HC[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -30 "$W/opt-ui"/update-logs/*.log 2>/dev/null; fail "7i: the combined update exited $rc"; }
grep -q 'Updated to v0.0.2-f: transport, agent' "$W/o" || { cat "$W/o"; fail "7i: no success line naming transport then agent"; }
grep -q 'agent: already current\|agent: the installer has nothing left' "$W/o" || { cat "$W/o"; fail "7i: the agent was not skipped although the transport pass updated everything it owns"; }
ls "$W"/opt-ui/update-logs/*-transport.log >/dev/null 2>&1 || fail "7i: no transport installer ran"
! ls "$W"/opt-ui/update-logs/*-agent.log >/dev/null 2>&1 || fail "7i: the agent installer ran with nothing to do"
[[ "$("$W/bin/baft" version)" == "baft 0.0.2-f" ]] || fail "7i: baft was not updated"
[[ "$(host_files)" == "$G0" ]] || fail "7i: the IR's config, key, agent files or units changed"
active baft-ui && active baft-agent-h || fail "7i: a service is not running afterwards"

log "7i2. the same host with the agent's own binary behind too: transport first, then the agent, units untouched"
v1_back
printf 'local build\n' >>"$W/bin/baft-agent"
rc=0; UPENV apply --yes "${AO[@]}" "${HC[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -30 "$W/opt-ui"/update-logs/*.log 2>/dev/null; fail "7i2: the combined update exited $rc"; }
t_line="$(grep -n '^== transport: running' "$W/o" | cut -d: -f1)"; a_line="$(grep -n '^== agent: running' "$W/o" | cut -d: -f1)"
[[ -n "$t_line" && -n "$a_line" && "$t_line" -lt "$a_line" ]] || { cat "$W/o"; fail "7i2: the transport did not run before the agent"; }
cmp -s "$W/rel2/dist/baft-agent-linux-$ag_arch" "$W/bin/baft-agent" || fail "7i2: the agent binary is not the signed v2 one"
[[ "$(host_files)" == "$G0" ]] || fail "7i2: the IR's config, key, agent files or units changed"
grep -q 'Updated to v0.0.2-f: transport, agent' "$W/o" || { cat "$W/o"; fail "7i2: no success line"; }

log "7j. BCC: a release whose bootstrap script differs rewrites the unit's bootstrap-sha256 comment and nothing else"
export BAFT_LOCAL_RELEASE_ROOT="$W/root"
BAFT_LOCAL_RELEASE_INSTALLER_APPEND="bootstrap script changed in v3" scripts/release/local_release.sh "$W/rel3" v0.0.3-f >/dev/null
unset BAFT_LOCAL_RELEASE_ROOT
unit_bcc="$W/sysd/baft-bcc-fault.service"
bootstrap_path="$(sed -n 's/^# baft-bootstrap-script: //p' "$unit_bcc")"
cp "$unit_bcc" "$W/bcc-unit-before"
B3="$(sha256sum "$W"/etc-bcc/* "$BCC_STATE" | sha256sum)"
rc=0; UP_REL=rel3 UPENV apply --yes "${AO[@]}" --only bcc "${BCCF[@]}" >"$W/o" 2>"$W/e" || rc=$?
[[ "$rc" == 0 ]] || { cat "$W/o" "$W/e"; tail -30 "$W/opt-bcc"/update-logs/*.log 2>/dev/null; fail "7j: the BCC update exited $rc"; }
grep -q 'only that line changes' "$W/o" && ! grep -q WARNING "$W/o" || { cat "$W/o"; fail "7j: the unit comment was not announced, or a warning appeared"; }
cmp -s "$W/rel3/dist/baft-bcc-linux-$bcc_arch" "$W/bin/baft-bcc" || fail "7j: the BCC binary is not the signed v3 one"
cmp -s "$W/rel3/dist/baft-install-linux-$bcc_arch" "$bootstrap_path" || fail "7j: the bootstrap script is not the v3 installer"
diff <(grep -v '^# baft-bootstrap-sha256:' "$W/bcc-unit-before") <(grep -v '^# baft-bootstrap-sha256:' "$unit_bcc") >/dev/null ||
  { diff "$W/bcc-unit-before" "$unit_bcc" || true; fail "7j: the BCC unit changed beyond its bootstrap-sha256 comment"; }
[[ "$(sed -n 's/^# baft-bootstrap-sha256: //p' "$unit_bcc")" == "$(sha256sum "$bootstrap_path" | cut -d' ' -f1)" ]] || fail "7j: the unit does not record the new bootstrap script"
[[ "$(sha256sum "$W"/etc-bcc/* "$BCC_STATE" | sha256sum)" == "$B3" ]] || fail "7j: BCC credentials, access files or state DB changed"
active baft-bcc-fault || fail "7j: BCC is not running afterwards"

log "7h. without the lab root the pinned production root rejects the throwaway release"
U2="$(snapshot)"; rc=0
env PATH="$W/stubs2:$PATH" SSL_CERT_FILE="$W/srv.crt" CURL_CA_BUNDLE="$W/srv.crt" \
  BAFT_RELEASE_URL="https://127.0.0.1:$SRV_PORT/rel2/dist" BAFT_REVOCATIONS_URL="https://127.0.0.1:$SRV_PORT/rel2/revocations.json" \
  "$W/cli/baft" update apply --yes --only transport --unit-dir "$W/sysd" --service baft-ui --release-state "$W/opt-ui/release-state.json" \
  >"$W/o" 2>"$W/e" </dev/null || rc=$?
[[ "$rc" == 1 ]] || { cat "$W/o"; fail "a release not signed by the pinned root was applied (exit $rc)"; }
grep -q 'did not verify' "$W/o" || { cat "$W/o"; fail "the refusal does not say the release did not verify"; }
[[ "$(snapshot)" == "$U2" ]] || fail "a refused release changed the host"

log "PASS"
