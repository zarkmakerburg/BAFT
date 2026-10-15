# Initial main-CI failures — preserved before any retry

Source00876e4fc71e5c7a9876e2f29d77cc50c48aba44, synthetic merge7de6c2fd
with identical source tree3411b0f. UI/static SUCCESS; original main38744416759
has e2e-launch1 job116454176610 and e2e-uninstall job116454176744 FAILURE.
Main test job still running at this snapshot. This source is NOT all-green.

Launch1: traffic through the first tunnel passed; subsequent rotation request at
2026-10-15T13:41:56Z received HTTP429, followed by JSONDecodeError on empty input.
The prior HTTP400 during drift checking is not established as the fatal cause.
The script shares one loopback BCC address with enrolled agents and already notes
rate limits in its polling helper. Which exact requests exhausted the budget is
not proved from this log; do not call a runner slowdown or fixed cause established.

Uninstall: BCC emergency backup was verified, but state.db changed after planning.
The unchanged hash guard refused deletion and restored moved files; script exited
BCC uninstall failure. Concurrent state/checkpoint timing is a hypothesis, not a
proven writer/root cause. Do not disable the hash guard or silently replan deletion.

Actual git diff0 versus de446b5 for both e2e scripts, scripts/release, install.sh,
internal/,cmd/,go.mod/go.sum,ci.yml and web/assets.go. The change affects only five
Aurora/docs paths and does not embed this preview in BCC. Parent/source PR204
and independent wire PR205 were all-green in fresh reads; that is comparative
status evidence, not a fresh reproduction of either current failure on the parent.

Both complete decoded failing logs are retained here; limited private-key/GitHub-token
pattern scan0 matches, not universal secret assurance. They contain throwaway
runner/loopback state and public keys, not production access. Original UI/local
fail/pass snapshots remain untouched. An optional scripts/e2e path read failed;
correct inventory is tests/e2e/{launch1,uninstall}.sh. One display of diagnostic
context was truncated; it is not the archived full logs, which are complete.

Next allowed diagnostic: at most ONE same-SHA retry of ONLY the failed CI jobs,
after recording this evidence and rechecking head/run completion. Preserve original
failure regardless of retry result. No security/KDF/rate-limit/guard/timeout/assertion/
production/source change is authorized by that retry. A successful retry would be
mixed first-failure/retry evidence, not a root fix or uninterrupted clean acceptance.
