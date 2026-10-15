# Bounded same-SHA retry decision

Original main38744416759 is COMPLETED/FAILURE:13 job records SUCCESS,2 FAILURE. Full actual Go1.27.1 tests/race/vet/fuzz job116454176705 SUCCESS (BCC full race472.927s). Full decoded log retained; other original failing logs/snapshots remain unchanged. UI/static SUCCESS.

Fresh PR206 head remains00876e4, base de446b5, identical synthetic merge tree3411b0f. Evidence of observed failures and recurring uninstall watch preserved before action. Retry ONLY the2 failed jobs ONCE at SAME source. This does not rerun successful Go/UI jobs or fix a source defect; failure history must stay visible even if latest workflow turns green. Do not change rate limits/hash guards/timeouts/assertions or claim root-cause resolution. No further automatic retry for this same slice. No code or operational change.
