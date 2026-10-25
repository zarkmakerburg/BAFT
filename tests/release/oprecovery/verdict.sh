#!/usr/bin/env bash
# Zero-SKIP verdict for the restart-recovery LAB: every required test must have
# a real top-level PASS in the test2json log; any SKIP, FAIL or MISSING test
# (or a test that passed with no required name) turns the gate red.
#
#   tests/release/oprecovery/verdict.sh <log>
set -uo pipefail
log=${1:?usage: verdict.sh <log>}
required=(
  TestOperationalRecovery_KillMidApplyRuleInstalledBeforeVerified
  TestOperationalRecovery_KillMidApplyRuleInstalledButNotJournaled
  TestOperationalRecovery_KillMidRetireBeforeRuleDelete
  TestOperationalRecovery_KillMidRetireAfterIntentWithListenerStillLive
  TestOperationalRecovery_VerifiedIngressAfterPlainAgentRestart
  TestOperationalRecovery_ForeignListenerFencesAndIsNeverTouched
  TestOperationalRecovery_UnleasedIntentAndUnjournaledRuleAreNeverTouched
  TestOperationalRecovery_RecoveryCannotReviveSpentJobs
  TestOperationalRecovery_RetireThenReapplyOnSamePort
  TestOperationalRecovery_ReleasedPortRefusesReplaySameAndLowerGeneration
  TestOperationalRecovery_ForeignListenerBlocksRelease
  TestOperationalRecovery_NoReleaseWithoutVerifiedRetirement
  TestOperationalRecovery_RestartBetweenRetireAndRelease
)
[[ -s "$log" ]] || { echo "GATE RED: empty log"; exit 1; }
bad=0
for t in "${required[@]}"; do
  pass=$(jq -r --arg t "$t" 'select(.Test==$t and .Action=="pass") | .Action' "$log" | wc -l)
  other=$(jq -r --arg t "$t" 'select((.Test==$t or (.Test!=null and (.Test|startswith($t+"/")))) and (.Action=="fail" or .Action=="skip")) | .Action' "$log" | wc -l)
  if [[ "$pass" -eq 1 && "$other" -eq 0 ]]; then echo "PASS    $t"; else echo "NOT-PASS $t (pass=$pass fail-or-skip=$other)"; bad=1; fi
done
if [[ $(jq -r 'select(.Action=="skip")|.Test' "$log" | wc -l) -ne 0 ]]; then echo "SKIP present: zero-SKIP violated"; bad=1; fi
extra=$(jq -r 'select(.Test!=null and (.Test|contains("/")|not) and .Action=="pass")|.Test' "$log" | sort -u | grep -vxFf <(printf '%s\n' "${required[@]}") || true)
[[ -z "$extra" ]] || { echo "unlisted passing tests (add to the required list): $extra"; bad=1; }
[[ $bad -eq 0 ]] && echo "GATE GREEN" || echo "GATE RED"
exit "$bad"
