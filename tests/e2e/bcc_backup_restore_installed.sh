#!/usr/bin/env bash
# Backup and restore on an INSTALLED host, in an isolated throwaway LAB (a CI
# runner with systemd). Everything uses the signed release built by
# scripts/release/local_release.sh with throwaway keys and the production
# paths of install.sh:
#
#   1. install.sh --bcc-only (signed release) -> systemd BCC, enabled for the
#      encrypted backup loop with --backup-key-file (0600 key file)
#   2. two servers enrolled with install.sh --agent-only; BCC builds a tunnel
#      from a reviewed plan hash; real bytes pass through it
#   3. agents quiesced, an encrypted backup is taken through the admin API
#   4. the BCC host is WIPED (unit, binary, config, state, audit, backups)
#   5. a FRESH install.sh --bcc-only; the operator's escrow (job key, admin
#      token, access file, backup key) is put back, the backup is verified,
#      previewed, refused when wrong/tampered, and restored with the installed
#      baft-bcc while the service is stopped
#   6. the restored BCC must equal the pre-wipe BCC: nodes + enrollments, the
#      tunnel and its plan hash, jobs, counters, the audit hash chain; the
#      agents reconnect, the tunnel is IN_SYNC and still carries traffic
#
#   sudo tests/e2e/bcc_backup_restore_installed.sh <release-dir from local_release.sh>
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
[[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
REL="$(cd "${1:?usage: bcc_backup_restore_installed.sh <release-dir>}" && pwd)"
VERSION="$(python3 -c 'import json,base64,sys;e=json.load(open(sys.argv[1]));print(json.loads(base64.b64decode(e["payload"]))["version"])' "$REL/dist/manifest.json")"
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; *) echo "unsupported arch"; exit 1 ;; esac
WORK="$(mktemp -d)"
PIDS=()
SVC=baft-bcc-e2e-bk
BCC_ETC=/etc/$SVC; BCC_VAR=/var/lib/$SVC; BCC_OPT=/opt/$SVC; BCC_BIN=/usr/local/bin/$SVC
STATE=$BCC_VAR/bcc-state.json
KEYF=$BCC_ETC/backup.key
UNIT=/etc/systemd/system/$SVC.service
DROPIN=/etc/systemd/system/$SVC.service.d
PORT=18090
BCC=http://127.0.0.1:$PORT
AGENT_UNITS=(baft-agent-ex baft-agent-ir)
TUNNEL_UNITS=(baft-ex baft-ir)
log() { printf '[bcc-backup-restore] %s\n' "$*"; }
fail() {
  echo "FAIL: $*" >&2
  [[ "${GITHUB_ACTIONS:-}" == "true" ]] && printf '%s\n' "$*" | head -c 3000 | tr '\n' ' ' | sed 's/%/%25/g; s/^/::error title=FAIL::/; s/$/\n/'
  exit 1
}
cleanup() {
  local rc=$?
  trap - EXIT
  if [[ $rc -ne 0 ]]; then
    for f in "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f" >&2; tail -n 40 "$f" >&2; }; done
    for u in "$SVC" "${AGENT_UNITS[@]}" "${TUNNEL_UNITS[@]}"; do echo "== journal $u" >&2; journalctl -u "$u" -n 30 --no-pager >&2 2>/dev/null || true; done
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      for u in "$SVC" "${AGENT_UNITS[@]}"; do journalctl -u "$u" -n 12 --no-pager 2>/dev/null | sed "s/%/%25/g; s|^|::error title=$u::|" || true; done
    fi
  fi
  for u in "${AGENT_UNITS[@]}" "${TUNNEL_UNITS[@]}" "$SVC"; do
    systemctl stop "$u" 2>/dev/null || true; systemctl disable "$u" 2>/dev/null || true; rm -f "/etc/systemd/system/$u.service"
  done
  rm -rf "$DROPIN"
  systemctl daemon-reload 2>/dev/null || true
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  rm -rf "$BCC_ETC" "$BCC_VAR" "$BCC_OPT" "$BCC_BIN" /etc/baft-ex /etc/baft-ir /etc/baft-agent-ex /etc/baft-agent-ir \
    /var/lib/baft-ex /var/lib/baft-ir /var/lib/baft-agent-ex /var/lib/baft-agent-ir "$WORK"
  exit $rc
}
trap cleanup EXIT

ADMIN=""   # read from the installed admin-token file
api() { curl -fsS -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' "$@"; }
jfield() { python3 -c 'import json,sys;d=json.load(sys.stdin);print(d'"$1"')'; }
wait_bcc() {
  local i
  for i in $(seq 1 60); do
    api -o /dev/null "$BCC/api/nodes" 2>/dev/null && return 0
    sleep 1
  done
  journalctl -u "$SVC" -n 30 --no-pager >&2 || true
  fail "BCC did not answer on $BCC"
}

# ---- release served the way the agents fetch it ------------------------------
log "release $VERSION"
mkdir -p "$WORK/www/$VERSION"
cp "$REL"/dist/* "$WORK/www/$VERSION/"
cp "$REL/revocations.json" "$WORK/www/"
(cd "$WORK/www" && exec python3 -m http.server 18201 --bind 127.0.0.1) >"$WORK/www.log" 2>&1 & PIDS+=($!)
go build -o "$WORK/audit-verify" ./cmd/baft-bcc-audit-verify

inst_env=(
  BAFT_INSTALL_FROM=release
  BAFT_RELEASE_URL="file://$REL/dist"
  BAFT_ROOT_PUB="$(cat "$REL/root.pub")"
  BAFT_REVOCATIONS_URL="file://$REL/revocations.json"
  BAFT_PREFIX=$BCC_OPT
  BAFT_RELEASE_STATE=$BCC_OPT/release-state.json
  BAFT_BCC_BIN=$BCC_BIN
  BAFT_BCC_CONFIG_DIR=$BCC_ETC
  BAFT_BCC_STATE_DIR=$BCC_VAR
  BAFT_BCC_STATE_FILE=$STATE
  BAFT_BCC_ADMIN_TOKEN_FILE=$BCC_ETC/admin-token
  BAFT_BCC_ACCESS_FILE=$BCC_ETC/access.json
  BAFT_BCC_JOB_KEY_FILE=$BCC_ETC/job-key
  BAFT_BCC_BACKUP_DIR=$BCC_VAR/backups
  BAFT_BCC_BOOTSTRAP_SCRIPT=$BCC_OPT/bcc-bootstrap/install.sh
  BAFT_BCC_SERVICE=$SVC
  BAFT_BCC_LISTEN=127.0.0.1:$PORT
  BAFT_NONINTERACTIVE=1
)
install_bcc() { env GITHUB_ACTIONS=true "${inst_env[@]}" bash install.sh --bcc-only >"$WORK/$1.out" 2>"$WORK/$1.err"; }

# The installer does not (yet) wire a backup key into the unit: the operator
# adds a drop-in, which is also what the operations guide documents. The tokens
# are only needed so the BCC can create the two LAB nodes below.
write_dropin() {
  local exec_line
  exec_line="$(grep '^ExecStart=' "$UNIT")"
  mkdir -p "$DROPIN"
  {
    echo '[Service]'
    echo "Environment=TOK_EX=$TOK_EX TOK_IR=$TOK_IR"
    echo 'ExecStart='
    echo "$exec_line --backup-key-file $KEYF"
  } >"$DROPIN/backup.conf"
  chmod 0600 "$DROPIN/backup.conf"
  systemctl daemon-reload
}

TOK_EX="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
TOK_IR="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"

# ---- 1. install BCC, enable the encrypted backup loop ------------------------
log "1. install BCC from the signed release"
install_bcc install1
ADMIN="$(tr -d '\r\n' <$BCC_ETC/admin-token)"
cp "$UNIT" "$WORK/unit.original"
python3 -c 'import os,base64;print(base64.b64encode(os.urandom(32)).decode())' >"$KEYF"
chmod 0600 "$KEYF"
write_dropin
systemctl restart "$SVC"
wait_bcc
ls "$BCC_VAR"/backups/daily-*.baftbak >/dev/null 2>&1 || fail "the backup loop wrote no scheduled backup after start"
[[ "$(stat -c '%U %a' "$KEYF")" == "root 600" ]] || fail "backup key file is not root 0600"
! systemctl show "$SVC" -p Environment -p ExecStart | grep -Fq "$(cat "$KEYF")" || fail "backup key value leaked into the unit/environment"
! tr '\0' '\n' <"/proc/$(systemctl show -p MainPID --value "$SVC")/environ" | grep -q BACKUP_KEY || fail "backup key is in the service environment"
systemctl show "$SVC" -p ExecStart | grep -Fq -- "--backup-key-file $KEYF" || fail "service does not run with --backup-key-file"

# ---- 2. two servers and a tunnel ----------------------------------------------
log "2. enroll two servers and build a tunnel"
JOB_KEY="$($BCC_BIN jobkey show --job-key-file $BCC_ETC/job-key)"
api -d '{"ID":"ex-e2e","Alias":"EX","Address":"127.0.0.1:22","Role":"foreign","AgentTokenEnv":"TOK_EX"}' "$BCC/api/nodes" >/dev/null
api -d '{"ID":"ir-e2e","Alias":"IR","Address":"127.0.0.1:22","Role":"worker","AgentTokenEnv":"TOK_IR"}' "$BCC/api/nodes" >/dev/null
enroll() { # name token metrics-port
  local n="$1" tok="$2" mport="$3"
  printf '%s\n' "$tok" >"$WORK/token-$n"; chmod 600 "$WORK/token-$n"
  env BAFT_INSTALL_FROM=release BAFT_RELEASE_URL="file://$REL/dist" BAFT_ROOT_PUB="$(cat "$REL/root.pub")" \
    BAFT_REVOCATIONS_URL="file://$REL/revocations.json" BAFT_BCC_JOB_KEY="$JOB_KEY" BAFT_AGENT_TOKEN_FILE="$WORK/token-$n" \
    BAFT_AGENT_ALLOW_HTTP=1 BAFT_AGENT_RELEASE_BASE_URL=http://127.0.0.1:18201 \
    BAFT_AGENT_REVOCATIONS_URL=http://127.0.0.1:18201/revocations.json \
    BAFT_AGENT_DIR="/etc/baft-agent-$n" BAFT_AGENT_STATE_DIR="/var/lib/baft-agent-$n" BAFT_AGENT_UNIT="baft-agent-$n" \
    BAFT_AGENT_INTERVAL=2s BAFT_CONFIG_DIR="/etc/baft-$n" BAFT_STATE_DIR="/var/lib/baft-$n" BAFT_SERVICE="baft-$n" \
    BAFT_METRICS_LISTEN="127.0.0.1:$mport" \
    bash install.sh --agent-only --bcc-url "$BCC" --node-id "$n-e2e" >"$WORK/install-$n.log" 2>&1
  systemctl is-active "baft-agent-$n" >/dev/null || fail "agent $n is not running"
}
enroll ex "$TOK_EX" 9291
enroll ir "$TOK_IR" 9292
python3 tests/e2e/echo.py serve 2443 >"$WORK/target.log" 2>&1 & PIDS+=($!)
sleep 1
REQ='{"ex_node":"ex-e2e","ir_node":"ir-e2e","public_address":"127.0.0.1","port":18443}'
PLAN_OK=False
for _ in $(seq 1 60); do
  PLAN="$(api -d "$REQ" "$BCC/api/tunnels/plan")"
  PLAN_OK="$(printf '%s' "$PLAN" | jfield '["ok"]')"
  [[ "$PLAN_OK" == "True" ]] && break
  sleep 1
done
[[ "$PLAN_OK" == "True" ]] || fail "the deployment plan never passed its gates: $PLAN"
HASH="$(printf '%s' "$PLAN" | jfield '["hash"]')"
TID="$(api -d "${REQ%\}},\"plan_hash\":\"$HASH\"}" "$BCC/api/tunnels" | jfield '["id"]')"
phase=""
for _ in $(seq 1 120); do
  phase="$(api "$BCC/api/tunnels?id=$TID" | jfield '["phase"]')"
  [[ "$phase" == active || "$phase" == rolled_back || "$phase" == rollback_failed ]] && break
  sleep 2
done
[[ "$phase" == active ]] || fail "the tunnel ended as '$phase'"
traffic() {
  local i
  for i in $(seq 1 15); do
    python3 tests/e2e/echo.py send 1443 2 2 >"$WORK/traffic.log" 2>&1 && return 0
    sleep 2
  done
  return 1
}
traffic || fail "no traffic through the tunnel before the backup"
log "   tunnel $TID active (plan hash $HASH), traffic passes"

# ---- 3. quiesce and take the encrypted backup --------------------------------
log "3. backup through the admin API (agents quiesced)"
systemctl stop "${AGENT_UNITS[@]}"
sleep 3
AUDIT_LIVE=$STATE.audit.jsonl
BACKUP_NAME="$(api -X POST "$BCC/api/backups/create" | jfield '["name"]')"
BACKUP="$BCC_VAR/backups/$BACKUP_NAME"
[[ -f "$BACKUP" ]] || fail "backup $BACKUP_NAME was not written"
[[ "$(stat -c '%U %a' "$BACKUP")" == "root 600" ]] || fail "backup file is not root 0600"
python3 - "$BACKUP" <<'PY' || fail "backup envelope is not the documented schema 4"
import json, sys
h = json.load(open(sys.argv[1]))["header"]
assert h["magic"] == "BAFT-BCC-BACKUP" and h["schema_version"] == 4 and h["audit_sequence"] > 0, h
PY
api "$BCC/api/tunnels" >"$WORK/tunnels.before.json"
api "$BCC/api/nodes" >"$WORK/nodes.before.json"
systemctl stop "$SVC"
cp "$STATE" "$WORK/state.before.db"
cp "$AUDIT_LIVE" "$WORK/audit.before.jsonl"
[[ ! -e "$STATE-journal" ]] || fail "state journal left behind after a clean stop"

# ---- 4. escrow the operator's secrets, then wipe the host --------------------
log "4. wipe the BCC host"
mkdir -p "$WORK/escrow" "$WORK/offsite"
install -m 0600 "$BCC_ETC/job-key" "$BCC_ETC/admin-token" "$BCC_ETC/access.json" "$KEYF" "$WORK/escrow/"
install -m 0600 "$BACKUP" "$WORK/offsite/"
systemctl disable "$SVC" >/dev/null 2>&1 || true
rm -rf "$DROPIN" "$UNIT" "$BCC_ETC" "$BCC_VAR" "$BCC_OPT" "$BCC_BIN"
systemctl daemon-reload
[[ ! -e "$STATE" && ! -e "$BCC_ETC" && ! -e "$UNIT" ]] || fail "wipe incomplete"

# ---- 5. fresh install, restore ------------------------------------------------
log "5. fresh install, then verify/preview/restore the backup"
install_bcc install2
systemctl stop "$SVC"
cmp -s "$UNIT" "$WORK/unit.original" || { diff "$UNIT" "$WORK/unit.original" >&2 || true; fail "a fresh install did not reproduce the original unit"; }
cmp -s "$BCC_ETC/job-key" "$WORK/escrow/job-key" && fail "a fresh install must generate a NEW job key (the backup does not carry it)"
# The backup holds BCC state, not credentials. The operator puts the escrowed
# job key (agents pin it), admin token, access file and backup key back.
install -m 0600 "$WORK/escrow/job-key" "$BCC_ETC/job-key"
install -m 0600 "$WORK/escrow/admin-token" "$BCC_ETC/admin-token"
install -m 0600 "$WORK/escrow/access.json" "$BCC_ETC/access.json"
install -m 0600 "$WORK/escrow/backup.key" "$KEYF"
ADMIN="$(tr -d '\r\n' <$BCC_ETC/admin-token)"
OFFSITE="$WORK/offsite/$BACKUP_NAME"
BKEY="$(tr -d '\r\n' <$KEYF)"
restore() { env BAFT_BCC_BACKUP_KEY="$1" "$BCC_BIN" restore --backup "$2" --state-file "$STATE" "${@:3}" 2>&1; }

# wrong key / tampered / truncated: refused, installed state untouched
state_hash() { sha256sum "$STATE" "$STATE.audit.jsonl" 2>/dev/null | awk '{printf "%s ", $1}'; }
H0="$(state_hash)"
WRONGKEY="$(python3 -c 'import os,base64;print(base64.b64encode(os.urandom(32)).decode())')"
rc=0; out="$(restore "$WRONGKEY" "$OFFSITE" --yes)" || rc=$?
[[ "$rc" != 0 ]] && grep -q 'backup authentication failed' <<<"$out" || fail "wrong key was not refused as an authentication failure (rc=$rc): $out"
python3 - "$OFFSITE" "$WORK/tampered.baftbak" <<'PY'
import base64, json, sys
e = json.load(open(sys.argv[1]))
ct = bytearray(base64.b64decode(e["ciphertext"])); ct[len(ct) // 2] ^= 1
e["ciphertext"] = base64.b64encode(bytes(ct)).decode()
json.dump(e, open(sys.argv[2], "w"))
PY
rc=0; out="$(restore "$BKEY" "$WORK/tampered.baftbak" --yes)" || rc=$?
[[ "$rc" != 0 ]] && grep -q 'backup authentication failed' <<<"$out" || fail "tampered backup was not refused (rc=$rc): $out"
head -c 300 "$OFFSITE" >"$WORK/truncated.baftbak"
rc=0; out="$(restore "$BKEY" "$WORK/truncated.baftbak" --yes)" || rc=$?
[[ "$rc" != 0 ]] || fail "truncated backup was accepted"
[[ "$(state_hash)" == "$H0" ]] || fail "a refused restore changed the installed state"
log "   wrong key, tampered and truncated backups refused; state untouched"

out="$(env BAFT_BCC_BACKUP_KEY="$BKEY" "$BCC_BIN" restore-preview --backup "$OFFSITE" --state-file "$STATE" 2>&1)" || fail "restore-preview failed: $out"
grep -q 'Backup verified' <<<"$out" || fail "preview did not verify the backup: $out"
grep -Eq 'nodes +[0-9]+ now -> 2 after restore' <<<"$out" || fail "preview does not show the two nodes coming back: $out"
[[ "$(state_hash)" == "$H0" ]] || fail "preview changed the installed state"
rc=0; out="$(restore "$BKEY" "$OFFSITE")" || rc=$?
[[ "$rc" == 4 ]] || fail "restore without --yes exited $rc (want 4): $out"
[[ "$(state_hash)" == "$H0" ]] || fail "restore without --yes changed the installed state"
out="$(restore "$BKEY" "$OFFSITE" --yes)" || fail "restore failed: $out"
log "   restore committed"

# ---- 6. the restored BCC equals the BCC that was backed up -------------------
log "6. compare the restored BCC with the pre-wipe BCC"
cp "$STATE" "$WORK/state.after.db"
cp "$STATE.audit.jsonl" "$WORK/audit.after.jsonl"
python3 - "$WORK/state.before.db" "$WORK/state.after.db" <<'PY' || fail "restored state differs from the pre-wipe state"
import json, sqlite3, sys
VOLATILE = {"nodes": {"health", "last_checked", "latency_ms", "agent_seen", "updated_at"}}
TABLES = ["nodes", "tunnels", "jobs", "counters", "public_ingress_reservations", "public_ingress_operations",
          "cert_rotations", "topology", "change_ledger", "retired_boot_ids"]
def dump(path):
    db = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    out = {}
    for t in TABLES:
        cols = [c[1] for c in db.execute(f"PRAGMA table_info({t})")]
        rows = []
        for r in db.execute(f"SELECT * FROM {t}"):
            row = dict(zip(cols, r))
            if "doc" in row:
                d = json.loads(row["doc"])
                for k in VOLATILE.get(t, ()):
                    d.pop(k, None)
                row["doc"] = d
            rows.append(json.dumps(row, sort_keys=True))
        out[t] = sorted(rows)
    return out
a, b = dump(sys.argv[1]), dump(sys.argv[2])
bad = [t for t in TABLES if a[t] != b[t]]
for t in bad:
    print(f"table {t} differs: before {len(a[t])} rows, after {len(b[t])}", file=sys.stderr)
    for x, y in zip(a[t], b[t]):
        if x != y:
            print("  before:", x[:400], "\n  after: ", y[:400], file=sys.stderr); break
assert not bad, bad
n = [json.loads(r) for r in a["nodes"]]
assert sorted(x["id"] for x in n) == ["ex-e2e", "ir-e2e"], n
assert all(x["doc"].get("agent_token_hash") for x in n), "enrollment (agent token hash) missing"
t = [json.loads(r)["doc"] for r in a["tunnels"]]
assert len(t) == 1 and t[0]["phase"] == "active" and t[0]["plan_hash"], t
print("state tables equal:", {k: len(v) for k, v in a.items() if v})
PY
python3 - "$WORK/audit.before.jsonl" "$WORK/audit.after.jsonl" "$BACKUP_NAME" <<'PY' || fail "restored audit chain is not the backed-up chain plus one restore entry"
import json, sys
before = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
after = [json.loads(l) for l in open(sys.argv[2]) if l.strip()]
name = sys.argv[3]
k = next(i for i, e in enumerate(before) if e["action"] == "backup.create" and e["target"] == name and e["outcome"] == "attempt") + 1
assert after[:k] == before[:k], "audit entries up to the backup are not identical"
assert len(after) == k + 1 and after[-1]["action"] == "backup.restore" and after[-1]["outcome"] == "success", after[k:]
assert after[-1]["prev_hash"] == before[k - 1]["hash"] and after[-1]["sequence"] == k + 1
print(f"audit: {k} entries identical + backup.restore")
PY
"$WORK/audit-verify" --audit "$STATE.audit.jsonl" | grep -q PASS || fail "audit hash chain does not verify after restore"

write_dropin
systemctl start "$SVC"
wait_bcc
systemctl is-active --quiet "$SVC" || fail "restored BCC is not active"
api "$BCC/api/tunnels" >"$WORK/tunnels.after.json"
api "$BCC/api/nodes" >"$WORK/nodes.after.json"
python3 - "$WORK/tunnels.before.json" "$WORK/tunnels.after.json" "$WORK/nodes.before.json" "$WORK/nodes.after.json" "$TID" "$HASH" <<'PY' || fail "API view after restore differs"
import json, sys
tb, ta, nb, na, tid, h = sys.argv[1:7]
tb, ta, nb, na = (json.load(open(p)) for p in (tb, ta, nb, na))
def ts(x): return {t["id"]: (t["phase"], t["plan_hash"], t.get("ex_node"), t.get("ir_node")) for t in (x if isinstance(x, list) else x.get("tunnels", x))}
assert ts(tb) == ts(ta) and ts(ta)[tid][:2] == ("active", h), (ts(tb), ts(ta))
ids = lambda x: sorted(n.get("id", n.get("ID")) for n in x)
assert ids(nb) == ids(na) == ["ex-e2e", "ir-e2e"]
PY
log "   API serves the same nodes, tunnel and plan hash"

log "7. agents reconnect to the restored BCC; the tunnel is in sync and carries traffic"
RESTORED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
systemctl start "${AGENT_UNITS[@]}"
for _ in $(seq 1 60); do
  seen="$(api "$BCC/api/nodes" | python3 -c '
import json,sys
ts=sys.argv[1]
n=json.load(sys.stdin)
print(sum(1 for x in n if x.get("agent_seen","") > ts))' "$RESTORED_AT")"
  [[ "$seen" == 2 ]] && break
  sleep 2
done
[[ "$seen" == 2 ]] || fail "only $seen/2 agents reconnected to the restored BCC (is the job key the escrowed one?)"
drift=""
api -X POST "$BCC/api/tunnels/drift?id=$TID" >/dev/null
for _ in $(seq 1 60); do
  out="$(api "$BCC/api/tunnels?id=$TID" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(len(d.get("drift_jobs",[])),(d.get("drift") or {}).get("state",""))')"
  [[ "${out%% *}" == 0 ]] && { drift="${out#* }"; break; }
  sleep 2
done
[[ "$drift" == IN_SYNC ]] || fail "restored BCC sees the tunnel as '$drift' (want IN_SYNC)"
traffic || fail "no traffic through the tunnel after the restore"
NEW_BACKUP="$(api -X POST "$BCC/api/backups/create" | jfield '["name"]')"
[[ -f "$BCC_VAR/backups/$NEW_BACKUP" ]] || fail "the restored BCC cannot create a backup"
"$WORK/audit-verify" --audit "$STATE.audit.jsonl" | grep -q PASS || fail "audit chain broke after new activity"

log "PASS"
