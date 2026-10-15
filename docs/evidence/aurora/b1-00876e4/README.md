# B1 navigation/content qualification — pending main CI

Source PR206:00876e4fc71e5c7a9876e2f29d77cc50c48aba44
Tree:3411b0f2060167d36feeb5530b1e879e932ac29e
Parent:PR204 de446b5df8f045c69ca388b3c500515143f7812a
Claim:#67/comment6193343127

Three added Node DOM regressions failed on the unmodified compiled parent:
12 existing PASS/3 new FAIL (focus fell from nav/skip links to BODY on read completion,
error and refresh). baseline-regression.log preserves the full failed output.
The source fix restores only an owned allowlisted local link without activating it;
outside focus, controls and main behavior remain covered by existing regressions.
CSS adds long-content wrapping/narrow topbar guards; no rendered-fit claim is made.

Development correction and exact published fetched-head checks PASS:
strict positive/negative TS, byte-identical8-module rebuild, size guards;
27 Aurora+15 DOM+14 classic Node tests,0 FAIL/0 SKIP. The16-case success matrix is
4 links ×2 languages ×2 themes, not device widths or Tab traversal. Error/refresh
tests are additional. No network/subresource/HTML execution or host-global mixing.

Full install, baseline failure, development correction and exact-head local logs retained.
UI38744416746/job116454176232 SUCCESS on Node22.23.3, full decoded log retained.
CI actually checked synthetic merge7de6c2fd956eb6771a5a69666f51f01b8ba2bb2f;
fetched tree equals source3411b0f, parentsde446b5 and00876e4. No actual main merge.
Main38744416759 and static38744416806 pending at this snapshot; append later outcome.
No rerun and no local Go/gofmt execution (tools absent).

Source publication: fresh board/head/assignment/ownership check; isolated branch from
live PR204; remote absent; staged/create_tree SHA equality; expected_sha=de446b5 and
force=false. Five changed source/docs paths,108 additions. No BCC embedding/API/auth/
session/CSRF/Go/persistence/installer or dependency/budget change. PR205 remains a
separate wire-contract branch, not merged or included in the27-test count.

Attempts before tests: two guessed CSS paths were absent; inventory corrected to
preview/shell.css. Broad first tool-name filter returned irrelevant preview services;
narrow browser-control inventory confirmed no supported control. No new dependency admission, browser/server installation or alternate browser
control was attempted. Existing locked37
dev packages installed with scripts disabled. npm proxy and VM warnings retained.
Limited key/token-pattern scan is recorded with publication, not universal assurance.

Shell20314 raw/6698 summed gzip (+341/+74); CSS6046/2006 (+218/+75);
foundation15508/5106 unchanged. Byte guards are not Android/rendering performance.
Browser/preview/screenshots/320px/zoom/font/keyboard/screen-reader/rendered accessibility/
performance OPEN/NOT RUN; native IR↔EX/candidate owner-DEFERRED/OPEN. Full#199 and
B/mobile acceptance remain OPEN. No production/main merge/tag/release/transport/
shaping/pool/exit switching/restart-resume/standby action.
