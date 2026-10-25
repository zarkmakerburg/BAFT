#!/usr/bin/env bash
# Installer ingress-chain provisioning (install.sh -> baft-ingress-chain helper).
# Runs the helper that install.sh embeds against a REAL nft inside a private
# network namespace (unshare -n) or a --network none container; it never
# touches the host's netns. Needs root + nft + unshare.
#
#   sudo tests/e2e/installer_ingress_chain.sh [--write-fixtures DIR]
#
# Cases: fresh create + idempotent rerun | foreign objects byte-identical |
# different existing layout refused and untouched | boot ensure is a no-op
# without our marker | remove only our empty chain (refused with a rule;
# foreign valid chain never removed).
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
FIXDIR=""
[[ "${1:-}" == "--write-fixtures" ]] && FIXDIR="$(mkdir -p "${2:?dir}" && cd "$2" && pwd)"

if [[ "${BAFT_ICT_INNER:-}" != 1 ]]; then
  [[ "$EUID" -eq 0 ]] || { echo "run as root"; exit 1; }
  command -v nft >/dev/null && command -v unshare >/dev/null || { echo "need nft and unshare"; exit 1; }
  exec unshare -n env BAFT_ICT_INNER=1 BAFT_ICT_FIX="$FIXDIR" bash "$0" "$@"
fi
FIXDIR="${BAFT_ICT_FIX:-}"

W="$(mktemp -d)"
trap 'rm -rf -- "${W:?}"' EXIT
fail() { printf '[ingress-chain] FAIL: %s\n' "$*" >&2; exit 1; }
log() { printf '[ingress-chain] %s\n' "$*"; }

# Extract the helper exactly as install.sh embeds it.
sed -n "/^  cat <<'BAFT_INGRESS_HELPER'\$/,/^BAFT_INGRESS_HELPER\$/p" install.sh | sed '1d;$d' >"$W/helper.sh"
[[ -s "$W/helper.sh" ]] || fail "helper not found in install.sh"
bash -n "$W/helper.sh"
mkdir -p "$W/sysd"
export BAFT_INGRESS_CHAIN_MARKER="$W/owner" BAFT_SYSTEMD_DIR="$W/sysd"
H() { bash "$W/helper.sh" "$@"; }
snap() { nft -j list ruleset | sed -E 's/"handle": *[0-9]+/"handle":N/g'; }
rc() { local r=0; "$@" >"$W/out" 2>&1 || r=$?; echo "$r"; }

# ---- 1. fresh create, then idempotent rerun ----
[[ "$(rc H ensure)" == 0 ]] || { cat "$W/out"; fail "fresh ensure"; }
nft -j list ruleset | grep -q '"name": *"ingress"' || fail "chain missing"
h1="$(H check 2>/dev/null)"; [[ "$h1" =~ ^[1-9][0-9]*$ ]] || fail "check handle '$h1'"
grep -qx "handle=$h1" "$W/owner" || fail "marker handle"
grep -qx "table_created=1" "$W/owner" || fail "marker table_created"
[[ "$(stat -c %a "$W/owner")" == 600 ]] || fail "marker mode"
before="$(nft -j list ruleset)"
[[ "$(rc H ensure)" == 0 ]] || fail "second ensure"
[[ "$(nft -j list ruleset)" == "$before" ]] || fail "rerun changed the ruleset"
[[ "$(H check 2>/dev/null)" == "$h1" ]] || fail "handle changed on rerun"
[[ -z "$FIXDIR" ]] || { nft -j list ruleset >"$FIXDIR/installer_chain_ruleset.json"; log "wrote accept fixture"; }
log "fresh + idempotent ok (handle $h1)"

# ---- 2. removal refused while a rule exists, then allowed ----
nft add rule inet baft ingress counter
[[ "$(rc H remove)" == 3 ]] || fail "remove with a rule must be refused"
nft list chain inet baft ingress | grep -q counter || fail "rule lost"
[[ -f "$W/owner" ]] || fail "marker dropped on refusal"
nft flush chain inet baft ingress
[[ "$(rc H remove)" == 0 ]] || { cat "$W/out"; fail "remove empty"; }
! nft list table inet baft >/dev/null 2>&1 || fail "table we created should be gone"
[[ ! -f "$W/owner" ]] || fail "marker should be gone"
log "remove only when empty ok"

# ---- 3. foreign objects are never touched ----
nft -f - <<'EOF'
add table inet foreign
add chain inet foreign input { type filter hook input priority 0; policy drop; }
add rule inet foreign input tcp dport 22 accept
add table ip nat
add chain ip nat post { type nat hook postrouting priority 100; }
add rule ip nat post masquerade
EOF
f0="$(nft list table inet foreign; nft list table ip nat)"
[[ "$(rc H ensure)" == 0 ]] || fail "ensure beside foreign"
[[ "$(nft list table inet foreign; nft list table ip nat)" == "$f0" ]] || fail "foreign objects changed"
[[ "$(rc H remove)" == 0 ]] || fail "remove beside foreign"
[[ "$(nft list table inet foreign; nft list table ip nat)" == "$f0" ]] || fail "foreign objects changed by remove"
log "foreign untouched ok"

# ---- 4. existing baft table with other content: table kept on remove ----
nft add table inet baft
nft add chain inet baft other
[[ "$(rc H ensure)" == 0 ]] || fail "ensure into existing baft table"
grep -qx "table_created=0" "$W/owner" || fail "must not claim a pre-existing table"
[[ "$(rc H remove)" == 0 ]] || fail "remove"
nft list chain inet baft other >/dev/null || fail "foreign baft chain removed"
nft list chain inet baft ingress >/dev/null 2>&1 && fail "our chain should be gone"
nft delete chain inet baft other; nft delete table inet baft
log "pre-existing table preserved ok"

# ---- 5. different existing layouts are refused and left alone ----
declare -a BAD=(
  "type filter hook input priority 0; policy accept;"
  "type filter hook input priority -100; policy drop;"
  "type filter hook forward priority -100; policy accept;"
  "type route hook output priority -100; policy accept;"
)
i=0
for spec in "${BAD[@]}"; do
  i=$((i+1))
  nft add table inet baft
  nft add chain inet baft ingress "{ $spec }"
  b="$(snap)"
  [[ "$(rc H ensure)" == 3 ]] || { cat "$W/out"; fail "layout '$spec' must be refused"; }
  [[ "$(snap)" == "$b" ]] || fail "refused layout was modified: $spec"
  [[ ! -f "$W/owner" ]] || fail "marker written for foreign layout"
  grep -q REFUSED "$W/out" || fail "refusal not reported"
  [[ "$(rc H remove)" == 0 ]] || fail "remove without marker must be a no-op"
  nft list chain inet baft ingress >/dev/null || fail "foreign chain removed without marker"
  [[ "$i" != 1 ]] || { [[ -z "$FIXDIR" ]] || { nft -j list ruleset >"$FIXDIR/installer_chain_ruleset_foreign_layout.json"; log "wrote refuse fixture"; }; }
  nft delete chain inet baft ingress; nft delete table inet baft
done
log "foreign layouts refused ok"

# ---- 6. a valid chain we did not create is left; boot ensure needs the marker ----
nft add table inet baft
nft add chain inet baft ingress '{ type filter hook input priority -100; policy accept; }'
[[ "$(rc H ensure)" == 0 ]] || fail "adopt-nothing ensure"
[[ ! -f "$W/owner" ]] || fail "must not claim a pre-existing valid chain"
[[ "$(rc H remove)" == 0 ]] && nft list chain inet baft ingress >/dev/null || fail "pre-existing chain must survive remove"
nft delete chain inet baft ingress; nft delete table inet baft
[[ "$(rc H ensure --boot)" == 0 ]] || fail "boot ensure without marker"
nft list table inet baft >/dev/null 2>&1 && fail "boot ensure without marker created something"
log "no-claim + boot no-op ok"

# ---- 7. reboot simulation: marker present, chain gone -> recreated ----
[[ "$(rc H ensure)" == 0 ]] || fail "ensure"
nft flush ruleset
[[ "$(rc H ensure --boot)" == 0 ]] || fail "boot ensure"
[[ "$(H check 2>/dev/null)" =~ ^[1-9][0-9]*$ ]] || fail "boot did not recreate"
H remove 2>/dev/null
log "boot recreate ok"
# ---- 8. installer wiring: unit ordering, idempotence, policy flag default off ----
block() { sed -n '/^render_ingress_helper() {$/,/^install_agent() {$/p' install.sh | sed '$d'; }
mkdir -p "$W/stubs"
printf '#!/bin/sh\necho "$@" >>%s/sysctl.calls\n' "$W" >"$W/stubs/systemctl"; chmod +x "$W/stubs/systemctl"
printf '#!/bin/sh\necho "$@" >>%s/agent.calls\n' "$W" >"$W/stubs/baft-agent"; chmod +x "$W/stubs/baft-agent"
(
  export PATH="$W/stubs:$PATH"
  warn() { echo "WARN $*" >&2; }; ok() { :; }; die() { echo "DIE $*" >&2; exit 1; }
  BAFT_AGENT_UNIT=baft-agent; BAFT_INGRESS_UNIT=baft-ingress-chain.service
  BAFT_INGRESS_HELPER="$W/lib/ingress-chain.sh"; BAFT_INGRESS_MARKER="$W/owner"
  BAFT_INGRESS_UNIT_FILE="$W/sysd/baft-ingress-chain.service"
  BAFT_AGENT_BIN="$W/stubs/baft-agent"; BAFT_NODE_ID=n1; INGRESS_CHAIN=1; PROVISION_INGRESS_POLICY=0
  eval "$(block)"
  install_ingress_chain 2>/dev/null || exit 1
  install_ingress_chain 2>/dev/null || exit 1   # rerun is idempotent
  provision_ingress_policy
  [[ ! -f "$W/agent.calls" ]] || { echo "policy ran by default" >&2; exit 1; }
  PROVISION_INGRESS_POLICY=1; provision_ingress_policy
  grep -qx -- "--provision-ingress-policy --node-id n1" "$W/agent.calls" || { echo "policy flag not honoured" >&2; exit 1; }
) || fail "installer wiring"
grep -q '^Before=baft-agent.service$' "$W/sysd/baft-ingress-chain.service" || fail "unit must order before the agent"
grep -q "^ConditionPathExists=$W/owner$" "$W/sysd/baft-ingress-chain.service" || fail "unit must be gated on the marker"
grep -q "enable baft-ingress-chain.service" "$W/sysctl.calls" || fail "unit not enabled"
[[ -x "$W/lib/ingress-chain.sh" ]] || fail "helper not installed"
H remove 2>/dev/null; [[ ! -f "$W/sysd/baft-ingress-chain.service" ]] || fail "remove must delete the boot unit"
log "installer wiring ok"
log "PASS"
