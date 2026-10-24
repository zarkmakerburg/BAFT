#!/usr/bin/env bash
# Assert actual guarded test execution from canonical scratch-container go -test.v.
# Never accept a package PASS when an expected test was skipped or absent.
set -euo pipefail
if [[ $# -ne 1 || ! -f "$1" ]]; then
  echo "BLOCKED: expected policy test log path" >&2
  exit 2
fi
log=$1
for family in ProvisionInstalledPolicyLAB InstalledPolicyBindingLAB CommandIntent; do
  matched=0
  while IFS= read -r testname; do
    [[ -n "$testname" ]] || continue
    matched=1
    # Go test verbose output uses exact test names followed by time/status.
    if ! grep -Eq "^--- PASS: ${testname}([[:space:]]|$)" "$log" ||
       grep -Eq "^--- SKIP: ${testname}([[:space:]]|$)" "$log" ||
       grep -Eq "^--- FAIL: ${testname}([[:space:]]|$)" "$log"; then
      echo "BLOCKED: guarded test ${testname} did not PASS without SKIP/FAIL" >&2
      exit 1
    fi
  done < <(sed -nE "s/^=== RUN[[:space:]]+(Test${family}[[:alnum:]_]*)([[:space:]]*)$/\1/p" "$log" | sort -u)
  if [[ $matched -eq 0 ]]; then
    echo "BLOCKED: no executed test under Test${family}" >&2
    exit 1
  fi
  # Catch skipped/failed suffixed family members even without RUN.
  if grep -Eq "^--- (SKIP|FAIL): Test${family}[[:alnum:]_]*([[:space:]]|$)" "$log"; then
    echo "BLOCKED: skipped or failed family Test${family}" >&2
    exit 1
  fi
done
