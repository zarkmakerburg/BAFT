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
if [[ "$(uname -s)" == "Linux" ]] && [[ "$(id -u)" -eq 0 ]]; then
  go test -count=1 -timeout=3m ./internal/ingressauthority -run '^Test(InstalledPolicyBindingLABSurvivesJournalAndRefusesInodeDrift|CommandIntentLABSurvivesRestartAndFencesReplays|CommandIntentLABRejectsCrossOperationJobAndRetireWithoutApply|CommandIntentLABRejectsMalformedDurableRequest)$'
else
  echo "BLOCKED: Linux root-only policy, journal and source-identity acceptance not executed here"
fi
echo "PASS: tested actual signed blocked-refusal path; Apply success and live TCP/nft acceptance remain BLOCKED"
