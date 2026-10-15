# B1 FINAL source qualification — mixed first failure / bounded retry

Source PR206:00876e4fc71e5c7a9876e2f29d77cc50c48aba44
Tree:3411b0f2060167d36feeb5530b1e879e932ac29e
Parent/current PR204:de446b5df8f045c69ca388b3c500515143f7812a
Draft OPEN/unmerged; no submitted review or inline thread at final check.

## Implemented and tested

Owned nav/skip-link focus survives asynchronous success/error/refresh repaint
without activating the link or taking outside focus. Three new regressions failed
on the compiled parent (12 PASS/3 FAIL) and pass with the bounded correction.
CSS adds wrapping for long aliases/labels and narrow topbars; rendered320px fit
remains unverified. No source claim that full B/mobile acceptance is complete.

Exact fetched head: strict positive/negative TypeScript, byte-identical8-module
rebuild/size and27 Aurora+15 DOM+14 classic Node tests PASS/0 FAIL/0 SKIP.
The success matrix executes4 links×2 languages×2 themes in Node DOM. No Tab/layout/
screen-reader/device acceptance follows from those programmatic-focus tests.
Source5 paths/108 additions; shell20314/6698 raw/summed gzip (+341/+74),
CSS6046/2006 (+218/+75); foundation unchanged. No dependency/lock/budget increase.

Source updates used expected_sha=de446b5/force=false, staged/API tree equality;
fetched exact head/tree clean. CI synthetic merge7de6c2fd956eb6771a5a69666f51f01b8ba2bb2f
was fetched: tree equals source3411b0f, parentsde446b5/00876e4. No main merge.
PR205 wire contracts remain independent and are not counted among these27 tests.

## Complete CI history

Latest THREE workflows SUCCESS:
- UI38744416746/job116454176232; complete decoded Node22.23.3 log retained.
- Static38744416806 SUCCESS, status qualification.
- Main38744416759 latest15 job records SUCCESS, after ONE failed-jobs-only retry.

Original main attempt:13 SUCCESS/2 FAILURE. Full actual Go1.27.1 tests/race/vet/fuzz
job116454176705 SUCCESS; BCC full race472.927s. Complete3,181,900-character decoded
log retained as initial-main-test116454176705.log (blobc408fcf51cb51de994e184568621f56d4542ec5a).
Original launch1116454176610 FAILED on HTTP429 during rotation then empty-input
JSONDecodeError. Original uninstall116454176744 FAILED because state.db changed
after planning: the hash guard refused deletion and restored moved files.
Both complete failed logs and all earlier pending snapshots remain unchanged.

ONE same-SHA failed-jobs-only retry was accepted after original run completion.
Retry uninstall116463557767 SUCCESS, full log retained (blobf0aa70f1fa93f3ea3aa9179fedb5e29ac410a923).
Retry launch1116463558164 SUCCESS, full log retained (blob449861e258fbca851d76e4e9435db76277a9eeef).
Latest15 records include13 carried successes with new aliases; they are not13
new executions or a second Go run. No source/rate-limit/KDF/hash-guard/timeout/
assertion change and no additional automatic retry. This is mixed failure/retry
evidence, not uninterrupted clean runs or proof of a root fix.

## Remaining Core watch and acceptance boundaries

The uninstall failure recurred in prior HQ6183781297/6183792979/6183803327, with
a later passing run6183844343 that explicitly kept the watch open. Verified reports
are preserved in prior-uninstall-watch.md. Exact writer/request-budget causes
remain unresolved. Keep plan-time hash/verified backup/restoration protections;
a deterministic Core regression and reviewed fix require separate scoped ownership.

Initial7 evidence files and3 later small text/JSON files were fetched/exact-match
verified; large initial native/Go log blob refs matched submitted content hashes.
npm/VM/action warnings and failed path-read attempts remain recorded. Limited
private-key/GitHub-token pattern scans0 matches, not universal secret assurance.
Final append must be fetched/verified before reporting/releasing claim6193343127.

Browser/served preview/screenshots/320px/zoom/font loading/key traversal/screen-reader/
rendered accessibility/performance OPEN/NOT RUN. Local Go/gofmt NOT RUN (absent);
CI is development evidence on throwaway runners. Native IR↔EX/candidate remains
owner-DEFERRED/OPEN. Full#199 and B acceptance remain OPEN. No BCC embedding/API/
auth/session/CSRF/installer/persistence/production change, main merge/tag/release,
transport/shaping/pool/exit switching/restart-resume/standby action.
