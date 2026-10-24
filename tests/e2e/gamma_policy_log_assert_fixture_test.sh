#!/usr/bin/env bash
# Falsification fixtures: complete actual-style output, suffixed SKIP, RUN-only.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/full" <<'LOG'
=== RUN   TestProvisionInstalledPolicyLABPersistsSharedIdentity
--- PASS: TestProvisionInstalledPolicyLABPersistsSharedIdentity (0.01s)
=== RUN   TestInstalledPolicyBindingLABRefusesChangedInode
--- PASS: TestInstalledPolicyBindingLABRefusesChangedInode (0.01s)
=== RUN   TestCommandIntentRejectsReplay
--- PASS: TestCommandIntentRejectsReplay (0.01s)
PASS
LOG
bash tests/e2e/gamma_policy_log_assert.sh "$tmp/full"
sed 's/--- PASS: TestProvisionInstalledPolicyLABPersistsSharedIdentity/--- SKIP: TestProvisionInstalledPolicyLABPersistsSharedIdentity/' "$tmp/full" > "$tmp/skipped"
if bash tests/e2e/gamma_policy_log_assert.sh "$tmp/skipped"; then
  echo 'FAIL: suffixed SKIP was accepted' >&2
  exit 1
fi
grep -v '^--- PASS: TestProvisionInstalledPolicyLAB' "$tmp/full" > "$tmp/run-only"
if bash tests/e2e/gamma_policy_log_assert.sh "$tmp/run-only"; then
  echo 'FAIL: RUN-only policy test was accepted' >&2
  exit 1
fi
echo 'PASS: complete log accepted; suffixed SKIP and RUN-only refused'
