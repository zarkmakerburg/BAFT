#!/usr/bin/env bash
# BCC backup / restore compatibility drill with the real binaries (no root, no
# systemd, loopback only). It proves, with the actual v0.3.7 baft-bcc built from
# its tag and the baft-bcc of this checkout:
#
#   upgrade   a v0.3.7 backup (backup schema 2) restores into the current BCC
#             and the restored state is served by it
#   downgrade the v0.3.7 binary refuses the current state (SQLite schema 16) and
#             refuses a current backup (backup schema 5); a verified pre-upgrade
#             snapshot is therefore the only way back (KNOWN-LIMITATIONS)
#   legacy    a backup schema 3 backup is still readable by the current restore
#   refusal   wrong key, flipped ciphertext, edited header (AAD), unsupported
#             schema, truncated and empty files, missing/short key, are all
#             refused and leave an existing state file byte-identical
#   key file  --backup-key-file is preferred over the environment key, keeps the
#             key out of /proc/<pid>/environ and refuses a non-private key file
#   gating    restore needs --yes, is refused while BCC runs, and the preview
#             changes nothing
#
#   tests/e2e/bcc_backup_compat.sh
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
OLD_TAG=v0.3.7
OLD_SHA=94c55dccf5af7cd126089374d991463931413121
EXPECT_STATE_SCHEMA=16
EXPECT_BACKUP_SCHEMA=5
WORK="$(mktemp -d)"
PID=""
log() { printf '[bcc-backup-compat] %s\n' "$*"; }
fail() { echo "[bcc-backup-compat] FAIL: $*" >&2; exit 1; }
cleanup() {
  local rc=$?
  trap - EXIT
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
  if [[ $rc -ne 0 ]]; then
    for f in "$WORK"/*/*.log "$WORK"/*.log; do [[ -f "$f" ]] && { echo "== $f" >&2; tail -n 25 "$f" >&2; }; done
  fi
  rm -rf "$WORK"
  exit $rc
}
trap cleanup EXIT

newkey() { python3 -c 'import os,base64;print(base64.b64encode(os.urandom(32)).decode())'; }

# ---- binaries ----------------------------------------------------------------
if ! git cat-file -e "$OLD_TAG^{commit}" 2>/dev/null; then git fetch --no-tags origin "refs/tags/$OLD_TAG:refs/tags/$OLD_TAG"; fi
[[ "$(git rev-parse "$OLD_TAG^{commit}")" == "$OLD_SHA" ]] || fail "$OLD_TAG moved: $(git rev-parse "$OLD_TAG^{commit}")"
mkdir -p "$WORK/old-src"
git archive "$OLD_TAG" | tar -x -C "$WORK/old-src"
go -C "$WORK/old-src" build -o "$WORK/bcc-old" ./cmd/baft-bcc
if [[ -n "${NEW_BCC:-}" ]]; then cp "$NEW_BCC" "$WORK/bcc-new"; else go build -o "$WORK/bcc-new" ./cmd/baft-bcc; fi
go build -o "$WORK/rewrap" ./tests/e2e/tools/rewrapbackup
OLD="$WORK/bcc-old"; NEW="$WORK/bcc-new"

# ---- helpers -----------------------------------------------------------------
sandbox() { # <name> -> directory with admin token, access file; BCC not started
  local d="$WORK/$1"; mkdir -p "$d"
  printf 'compat-token\n' >"$d/admin"; chmod 600 "$d/admin"
  "$NEW" access init --access-file "$d/access.json" >/dev/null
  echo "$d"
}
BCC_PORT=18420
start_bcc() { # <bin> <dir> <logname> [args...]; env comes from BCC_ENV
  local bin=$1 dir=$2 name=$3; shift 3
  BCC_PORT=$((BCC_PORT + 1))
  env -u BAFT_BCC_BACKUP_KEY ${BCC_ENV[@]+"${BCC_ENV[@]}"} "$bin" --listen "127.0.0.1:$BCC_PORT" \
    --state-file "$dir/state.db" --admin-token-file "$dir/admin" --access-file "$dir/access.json" \
    --job-key-file "$dir/jobkey" --backup-dir "$dir/bk" "$@" >"$dir/$name.log" 2>&1 &
  PID=$!
  local i
  for i in $(seq 1 100); do
    curl -fsS -o /dev/null -H 'Authorization: Bearer compat-token' "http://127.0.0.1:$BCC_PORT/api/nodes" 2>/dev/null && return 0
    kill -0 "$PID" 2>/dev/null || { cat "$dir/$name.log" >&2; fail "$bin exited during startup"; }
    sleep 0.1
  done
  fail "$bin did not serve"
}
stop_bcc() { kill "$PID"; wait "$PID" 2>/dev/null || true; PID=""; }
api() { curl -fsS -H 'Authorization: Bearer compat-token' -H 'Content-Type: application/json' "$@"; }
url() { echo "http://127.0.0.1:$BCC_PORT$1"; }
mknode() { api -d "{\"ID\":\"$1\",\"Alias\":\"$1\",\"Address\":\"127.0.0.1:1\",\"Role\":\"foreign\"}" "$(url /api/nodes)" >/dev/null; }
mkbackup() { api -X POST "$(url /api/backups/create)" | python3 -c 'import json,sys;print(json.load(sys.stdin)["name"])'; }
backup_schema() { python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["header"]["schema_version"])' "$1"; }
db_schema() { python3 - "$1" <<'PY'
import sqlite3, sys
db = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
print(db.execute("SELECT MAX(version) FROM schema_migrations").fetchone()[0])
PY
}
db_nodes() { python3 - "$1" <<'PY'
import sqlite3, sys
db = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
print(" ".join(sorted(r[0] for r in db.execute("SELECT id FROM nodes"))))
PY
}
# restore <bin> <key> <state> <backup> [args]: prints output, returns the rc.
restore() {
  local bin=$1 key=$2 state=$3 bak=$4; shift 4
  env BAFT_BCC_BACKUP_KEY="$key" "$bin" restore --backup "$bak" --state-file "$state" "$@" 2>&1
}
expect_refused() { # <label> <expected text> <state> <backup> <key> [bin]
  local label=$1 want=$2 state=$3 bak=$4 key=$5 bin=${6:-$NEW} before after out rc=0
  before="$(sha256sum "$state" 2>/dev/null || echo absent)"
  out="$(restore "$bin" "$key" "$state" "$bak" --yes)" || rc=$?
  [[ "$rc" != 0 ]] || fail "$label: restore unexpectedly succeeded"
  grep -Fq -- "$want" <<<"$out" || fail "$label: wrong refusal reason (want '$want'): $out"
  grep -Fq -- "$key" <<<"$out" && fail "$label: output leaked the backup key"
  after="$(sha256sum "$state" 2>/dev/null || echo absent)"
  [[ "$before" == "$after" ]] || fail "$label: refused restore changed the state file"
  # The read-only preview must refuse for the same reason and change nothing.
  rc=0
  out="$(env BAFT_BCC_BACKUP_KEY="$key" "$bin" restore-preview --backup "$bak" --state-file "$state" 2>&1)" || rc=$?
  [[ "$rc" != 0 ]] || fail "$label: restore-preview unexpectedly succeeded"
  [[ "$(sha256sum "$state" 2>/dev/null || echo absent)" == "$before" ]] || fail "$label: preview changed the state file"
  log "  refused: $label"
}

# =============================================================================
log "1. v0.3.7 creates a backup (schema 2); the current binary restores it"
KEY_OLD="$(newkey)"
D1="$(sandbox old1)"
BCC_ENV=(BAFT_BCC_BACKUP_KEY="$KEY_OLD")
start_bcc "$OLD" "$D1" old
mknode upg-a; mknode upg-b
OLD_BACKUP="$D1/bk/$(mkbackup)"
stop_bcc
[[ "$(backup_schema "$OLD_BACKUP")" == 2 ]] || fail "v0.3.7 backup is not schema 2"
OLD_STATE_SCHEMA="$(db_schema "$D1/state.db")"
[[ "$OLD_STATE_SCHEMA" -lt "$EXPECT_STATE_SCHEMA" ]] || fail "v0.3.7 state schema $OLD_STATE_SCHEMA is not older than $EXPECT_STATE_SCHEMA"

D2="$(sandbox fresh1)"   # a fresh host: no state at all
out="$(env BAFT_BCC_BACKUP_KEY="$KEY_OLD" "$NEW" restore-preview --backup "$OLD_BACKUP" --state-file "$D2/state.db")" || fail "preview of a v0.3.7 backup failed: $out"
grep -q 'Backup verified' <<<"$out" || fail "preview did not verify the v0.3.7 backup: $out"
[[ ! -e "$D2/state.db" ]] || fail "restore-preview created a state file"
rc=0; out="$(restore "$NEW" "$KEY_OLD" "$D2/state.db" "$OLD_BACKUP")" || rc=$?
[[ "$rc" == 4 ]] || fail "restore without --yes exited $rc (want 4): $out"
grep -q 'RESTORE NOT RUN' <<<"$out" || fail "restore without --yes did not say it did nothing: $out"
[[ ! -e "$D2/state.db" ]] || fail "restore without --yes created a state file"
out="$(restore "$NEW" "$KEY_OLD" "$D2/state.db" "$OLD_BACKUP" --yes)" || fail "restore of a v0.3.7 backup onto a fresh host failed: $out"
[[ "$(db_schema "$D2/state.db")" == "$EXPECT_STATE_SCHEMA" ]] || fail "restored state is not schema $EXPECT_STATE_SCHEMA"
[[ "$(db_nodes "$D2/state.db")" == "upg-a upg-b" ]] || fail "restored nodes: $(db_nodes "$D2/state.db")"
BCC_ENV=()
start_bcc "$NEW" "$D2" new
[[ "$(api "$(url /api/nodes)" | python3 -c 'import json,sys;print(" ".join(sorted(n["id"] for n in json.load(sys.stdin))))')" == "upg-a upg-b" ]] || fail "current BCC does not serve the restored nodes"
api "$(url /api/audit)" | python3 -c '
import json,sys
a=json.load(sys.stdin); a=a.get("entries",a) if isinstance(a,dict) else a
assert a and a[-1]["action"]=="backup.restore", a[-3:]' || fail "restore did not append a backup.restore audit entry"
mknode post-restore
BAK_CUR_NAME=""
KEY_NEW="$(newkey)"
stop_bcc

# a current backup (schema 5) from a state at schema 16
BCC_ENV=(BAFT_BCC_BACKUP_KEY="$KEY_NEW")
start_bcc "$NEW" "$D2" new2
CUR_BACKUP="$D2/bk/$(mkbackup)"
stop_bcc
BCC_ENV=()
[[ "$(backup_schema "$CUR_BACKUP")" == "$EXPECT_BACKUP_SCHEMA" ]] || fail "current backup is not schema $EXPECT_BACKUP_SCHEMA"

log "2. downgrade: v0.3.7 refuses the current state and the current backup"
cp "$D2/state.db" "$WORK/state-copy.db"
set +e
timeout 10s "$OLD" --listen 127.0.0.1:18499 --state-file "$WORK/state-copy.db" --admin-token-file "$D2/admin" \
  --access-file "$D2/access.json" --job-key-file "$D2/jobkey" >"$WORK/old-refusal.log" 2>&1
rc=$?
set -e
[[ "$rc" != 0 && "$rc" != 124 ]] || fail "v0.3.7 accepted a schema $EXPECT_STATE_SCHEMA state or hung (rc=$rc)"
grep -q 'newer than this build supports' "$WORK/old-refusal.log" || fail "v0.3.7 refused for another reason: $(cat "$WORK/old-refusal.log")"
log "  v0.3.7 refuses state schema $EXPECT_STATE_SCHEMA"
expect_refused "v0.3.7 restore of a schema-$EXPECT_BACKUP_SCHEMA backup" "unsupported backup schema version $EXPECT_BACKUP_SCHEMA" "$WORK/never-created.db" "$CUR_BACKUP" "$KEY_NEW" "$OLD"
[[ ! -e "$WORK/never-created.db" ]] || fail "v0.3.7 created a state file while refusing"

log "3. legacy: a schema 3 backup is still readable by the current restore"
"$WORK/rewrap" -in "$CUR_BACKUP" -key-file <(echo "$KEY_NEW") -schema 3 -out "$WORK/v3.baftbak"
[[ "$(backup_schema "$WORK/v3.baftbak")" == 3 ]] || fail "rewrap did not produce schema 3"
D3="$(sandbox fresh3)"
out="$(restore "$NEW" "$KEY_NEW" "$D3/state.db" "$WORK/v3.baftbak" --yes)" || fail "schema 3 backup was not restored: $out"
[[ "$(db_nodes "$D3/state.db")" == "$(db_nodes "$D2/state.db")" ]] || fail "schema 3 restore lost nodes"
expect_refused "v0.3.7 restore of a schema 3 backup" "unsupported backup schema version 3" "$WORK/never-created2.db" "$WORK/v3.baftbak" "$KEY_NEW" "$OLD"

log "4. tampered, wrong-key and malformed backups are refused and change nothing"
TARGET="$WORK/target.db"; cp "$D2/state.db" "$TARGET"; cp "$D2/state.db.audit.jsonl" "$TARGET.audit.jsonl"
tamper() { python3 - "$CUR_BACKUP" "$WORK/t.baftbak" "$1" <<'PY'
import base64, json, sys
src, dst, mode = sys.argv[1:4]
raw = open(src, "rb").read()
if mode == "truncate":
    open(dst, "wb").write(raw[: len(raw) // 2]); sys.exit()
if mode == "empty":
    open(dst, "wb").write(b""); sys.exit()
e = json.loads(raw)
if mode == "ciphertext":
    ct = bytearray(base64.b64decode(e["ciphertext"])); ct[len(ct) // 2] ^= 1
    e["ciphertext"] = base64.b64encode(bytes(ct)).decode()
elif mode == "nonce":
    n = bytearray(base64.b64decode(e["nonce"])); n[0] ^= 1
    e["nonce"] = base64.b64encode(bytes(n)).decode()
elif mode == "header-time":
    e["header"]["created_at"] = "2001-01-01T00:00:00Z"
elif mode == "header-audit":
    e["header"]["audit_sequence"] += 1
elif mode == "schema":
    e["header"]["schema_version"] = 6
elif mode == "magic":
    e["header"]["magic"] = "NOT-A-BACKUP"
json.dump(e, open(dst, "w"), indent=2)
PY
}
WRONG="$(newkey)"
expect_refused "wrong key" "backup authentication failed" "$TARGET" "$CUR_BACKUP" "$WRONG"
tamper ciphertext;  expect_refused "flipped ciphertext bit" "backup authentication failed" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper nonce;       expect_refused "flipped nonce bit" "backup authentication failed" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper header-time; expect_refused "edited header created_at (authenticated data)" "backup authentication failed" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper header-audit; expect_refused "edited header audit anchor" "backup authentication failed" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper schema;      expect_refused "unsupported schema 6" "unsupported backup schema version 6" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper magic;       expect_refused "wrong magic" "invalid backup magic" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper truncate;    expect_refused "truncated file" "invalid backup envelope" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
tamper empty;       expect_refused "empty file" "invalid backup envelope" "$TARGET" "$WORK/t.baftbak" "$KEY_NEW"
expect_refused "missing file" "no such file" "$TARGET" "$WORK/does-not-exist.baftbak" "$KEY_NEW"
# Key problems are usage errors (exit 2) before any file is touched.
rc=0; out="$(env -u BAFT_BCC_BACKUP_KEY "$NEW" restore --backup "$CUR_BACKUP" --state-file "$TARGET" --yes 2>&1)" || rc=$?
[[ "$rc" == 2 ]] && grep -q 'BAFT_BCC_BACKUP_KEY is not set' <<<"$out" || fail "missing key: rc=$rc $out"
rc=0; out="$(restore "$NEW" "$(head -c 16 /dev/urandom | base64)" "$TARGET" "$CUR_BACKUP" --yes)" || rc=$?
[[ "$rc" == 2 ]] && grep -q 'exactly 32 bytes' <<<"$out" || fail "short key: rc=$rc $out"
rc=0; out="$(restore "$NEW" "not base64 !!" "$TARGET" "$CUR_BACKUP" --yes)" || rc=$?
[[ "$rc" == 2 ]] && grep -q 'must be base64' <<<"$out" || fail "non-base64 key: rc=$rc $out"
log "  refused: missing, short and non-base64 key"
# ...and the intact backup with the right key still works after all of that.
out="$(restore "$NEW" "$KEY_NEW" "$TARGET" "$CUR_BACKUP" --yes)" || fail "the intact backup no longer restores: $out"
[[ "$(db_nodes "$TARGET")" == "$(db_nodes "$D2/state.db")" ]] || fail "state differs after the intact restore"
python3 - "$TARGET.audit.jsonl" <<'PY' || fail "audit after restore is not a valid chain ending in backup.restore"
import hashlib, json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
assert rows and rows[-1]["action"] == "backup.restore", rows[-1:]
assert [r["sequence"] for r in rows] == list(range(1, len(rows) + 1))
for a, b in zip(rows, rows[1:]):
    assert b["prev_hash"] == a["hash"], (a["sequence"], b["sequence"])
PY

log "5. --backup-key-file: preferred over the environment, private files only"
D5="$(sandbox keyfile)"
KEY_FILE_VALUE="$(newkey)"; KEY_ENV_VALUE="$(newkey)"
printf '%s\n' "$KEY_FILE_VALUE" >"$D5/backup.key"; chmod 600 "$D5/backup.key"
BCC_ENV=(BAFT_BCC_BACKUP_KEY="$KEY_ENV_VALUE")
start_bcc "$NEW" "$D5" kf --backup-key-file "$D5/backup.key"
mknode kf-a
KF_BACKUP="$D5/bk/$(mkbackup)"
ls "$D5"/bk/daily-*.baftbak >/dev/null 2>&1 || fail "the backup loop did not write its first scheduled backup"
tr '\0' '\n' <"/proc/$PID/environ" | grep -q '^BAFT_BCC_BACKUP_KEY=' || true
stop_bcc
BCC_ENV=()
out="$(env BAFT_BCC_BACKUP_KEY="$KEY_FILE_VALUE" "$NEW" restore-preview --backup "$KF_BACKUP" --state-file "$WORK/kf-target.db" 2>&1)" || fail "the file key does not open the backup: $out"
rc=0; env BAFT_BCC_BACKUP_KEY="$KEY_ENV_VALUE" "$NEW" restore-preview --backup "$KF_BACKUP" --state-file "$WORK/kf-target.db" >/dev/null 2>&1 || rc=$?
[[ "$rc" != 0 ]] || fail "the environment key opened a backup made with the key file: the file was not preferred"
log "  the key file wins over BAFT_BCC_BACKUP_KEY"
# With only the file, the key never appears in the process environment.
BCC_ENV=()
start_bcc "$NEW" "$D5" kf2 --backup-key-file "$D5/backup.key"
! tr '\0' '\n' <"/proc/$PID/environ" | grep -q 'BACKUP_KEY' || fail "backup key found in /proc/<pid>/environ"
! tr '\0' ' ' <"/proc/$PID/cmdline" | grep -Fq "$KEY_FILE_VALUE" || fail "backup key found on the command line"
stop_bcc
bad_key_file() { # <label> <expected text> : start BCC with the prepared $D5/bad.key
  local rc=0 out
  out="$(timeout 10s "$NEW" --listen 127.0.0.1:18498 --state-file "$WORK/bk-state.db" --admin-token-file "$D5/admin" \
    --access-file "$D5/access.json" --job-key-file "$WORK/bk-jobkey" --backup-dir "$WORK/bk-bk" --backup-key-file "$D5/bad.key" 2>&1)" || rc=$?
  [[ "$rc" == 2 ]] || fail "$1: BCC exited $rc, want 2: $out"
  grep -Fq -- "$2" <<<"$out" || fail "$1: wrong reason (want '$2'): $out"
  [[ ! -e "$WORK/bk-bk" ]] || fail "$1: BCC created a backup although the key was refused"
  rm -f "$WORK/bk-state.db"*
  log "  refused key file: $1"
}
printf '%s\n' "$KEY_FILE_VALUE" >"$D5/bad.key"; chmod 644 "$D5/bad.key"
bad_key_file "world-readable (0644)" "private regular file"
printf '%s\n' "$KEY_FILE_VALUE" >"$D5/bad.key"; chmod 640 "$D5/bad.key"
bad_key_file "group-readable (0640)" "private regular file"
rm -f "$D5/bad.key"; ln -s "$D5/backup.key" "$D5/bad.key"
bad_key_file "symlink" "backup key file"
rm -f "$D5/bad.key"; printf 'not-a-key\n' >"$D5/bad.key"; chmod 600 "$D5/bad.key"
bad_key_file "not base64 of 32 bytes" "backup key file must"
rm -f "$D5/bad.key"; : >"$D5/bad.key"; chmod 600 "$D5/bad.key"
bad_key_file "empty" "backup key file is empty"
rm -f "$D5/bad.key"; ln "$D5/backup.key" "$D5/bad.key"
bad_key_file "hard link (link count 2)" "backup key file"
rm -f "$D5/bad.key"

log "6. restore is refused while BCC runs"
D6="$(sandbox running)"
BCC_ENV=(BAFT_BCC_BACKUP_KEY="$KEY_NEW")
start_bcc "$NEW" "$D6" run
mknode live-a
rc=0; out="$(restore "$NEW" "$KEY_NEW" "$D6/state.db" "$CUR_BACKUP" --yes)" || rc=$?
[[ "$rc" != 0 ]] || fail "restore succeeded while BCC was running"
grep -q 'stop BCC' <<<"$out" || fail "restore while running: wrong reason: $out"
rc=0; out="$(env BAFT_BCC_BACKUP_KEY="$KEY_NEW" "$NEW" restore-preview --backup "$CUR_BACKUP" --state-file "$D6/state.db" 2>&1)" || rc=$?
[[ "$rc" != 0 ]] || fail "preview succeeded while BCC was running"
[[ "$(api "$(url /api/nodes)" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')" == 1 ]] || fail "the running BCC lost its state"
stop_bcc
BCC_ENV=()

log "PASS"
