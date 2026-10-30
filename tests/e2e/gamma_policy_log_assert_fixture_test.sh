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
# Regression: go test -v prints subtest results indented. A skipped subtest under
# a PASSing parent, a CRLF capture, and an oversized whitespace-only line must
# not be accepted/ignored; a longer test name must not satisfy a shorter one.
sed 's|^=== RUN   TestCommandIntentRejectsReplay$|&\n    --- SKIP: TestCommandIntentRejectsReplay/needs_root (0.00s)|' "$tmp/full" > "$tmp/sub-skip"
grep -q '^    --- SKIP: TestCommandIntentRejectsReplay/needs_root' "$tmp/sub-skip"
if bash tests/e2e/gamma_policy_log_assert.sh "$tmp/sub-skip" 2>/dev/null; then
  echo 'FAIL: indented subtest SKIP under PASSing parent was accepted' >&2
  exit 1
fi
sed 's/$/\r/' "$tmp/full" > "$tmp/crlf"
bash tests/e2e/gamma_policy_log_assert.sh "$tmp/crlf"
{
  cat "$tmp/full"
  head -c 5000000 /dev/zero | tr '\0' ' '
  echo
  echo '--- SKIP: TestCommandIntentRejectsReplay (0.00s)'
} > "$tmp/oversized"
if bash tests/e2e/gamma_policy_log_assert.sh "$tmp/oversized" 2>/dev/null; then
  echo 'FAIL: SKIP after oversized whitespace line was accepted' >&2
  exit 1
fi
# A longer name (TestXExtra) must not stand in for the missing TestX family member.
grep -v 'TestInstalledPolicyBindingLABRefusesChangedInode' "$tmp/full" > "$tmp/missing-family"
if bash tests/e2e/gamma_policy_log_assert.sh "$tmp/missing-family" 2>/dev/null; then
  echo 'FAIL: log with an entire guarded family missing was accepted' >&2
  exit 1
fi
echo 'PASS: complete log accepted; suffixed SKIP, RUN-only, indented subtest SKIP, oversized whitespace and missing family refused'
