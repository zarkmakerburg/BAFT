#!/usr/bin/env bash
# Gamma actual operational-path negative acceptance on canonical shared Go APIs.
# No mock successful Apply, no host networking mutation, no production.
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
if [[ "${GAMMA_ISOLATED_LAB:-}" != "1" ]]; then
  echo "BLOCKED: requires explicit isolated LAB acknowledgement" >&2
  exit 2
fi
for f in internal/agent/public_ingress_operational_receipt_test.go internal/bcc/ingress_receipt_tls_integration_test.go internal/ingressauthority/policy_bootstrap_linux_test.go; do
  test -f "$f" || { echo "BLOCKED: missing canonical implementation/test: $f" >&2; exit 2; }
done
echo "Gamma operational contract: actual signed RunOnce + enrolled HTTPS blocked receipt + durable authority refusal"
go test -count=1 -timeout=3m ./internal/agent -run '^Test(OperationalIngressSignedPollReportsBlockedReceiptAndNeverUsesLAB|BlockedIngressReceiptRefusesUntrustedTransportsBeforeSendingToken|BlockedIngressReceiptNeverFollowsRedirect|BlockedIngressReceiptAckRequiresExactUnverifiedPendingObject|BlockedIngressReceiptAckReadsNPlusOneBytes)$'
go test -count=1 -timeout=3m ./internal/bcc -run '^Test(IngressReceiptRealTLSReceiverKeepsUnverifiedEvidencePending|SignedAgentRefusalReachesActualBCCReceiptReceiver)$'
# Policy acceptance requires disposable scratch-container rootfs and the policy
# opt-in/sentinel INSIDE the container. Linux UID0 on the host is insufficient.
# Do NOT create /baft-policy-lab or write /etc on the host.
policy_harness=tests/e2e/managed_ingress_policy_lab.sh
if [[ ! -f "$policy_harness" ]]; then
  echo "BLOCKED: canonical disposable policy harness unavailable in this branch; cannot claim policy PASS" >&2
  exit 2
fi
if ! command -v docker >/dev/null || ! docker info >/dev/null 2>&1; then
  echo "BLOCKED: disposable Docker LAB unavailable; policy coverage NOT RUN" >&2
  exit 2
fi
policy_log="$(mktemp "${TMPDIR:-/tmp}/gamma-policy-evidence.XXXXXX")"
trap 'rm -f "$policy_log"' EXIT
if ! bash "$policy_harness" >"$policy_log" 2>&1; then
  cat "$policy_log" >&2
  echo "BLOCKED: canonical scratch policy LAB failed, no policy PASS" >&2
  exit 1
fi
# Exit 0 and === RUN alone are insufficient: Go may mark a guarded
# family SKIP while reporting package PASS. Require at least one actual
# individual PASS per family, and reject any skipped matching guarded test.
for name in ProvisionInstalledPolicyLAB InstalledPolicyBindingLAB CommandIntent; do
  if ! grep -Eq "^=== RUN[[:space:]]+Test${name}" "$policy_log" ||
     ! grep -Eq "^--- PASS: Test${name}([^[:alnum:]_]|$)" "$policy_log" ||
     grep -Eq "^--- SKIP: Test${name}([^[:alnum:]_]|$)" "$policy_log"; then
    cat "$policy_log" >&2
    echo "BLOCKED: policy LAB family Test${name} not proven executed and passing without skips" >&2
    exit 1
  fi
done
echo "PASS: actual signed blocked-refusal path and isolated-container policy tests executed; operational Apply/listener/TCP/receipt remains BLOCKED"
