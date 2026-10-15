# B1: owned navigation focus and narrow-content guards

Parent: live PR204 `de446b5df8f045c69ca388b3c500515143f7812a`.
Ownership: #67/comment6193343127 / codex-20261015-1340-aurora-b1-navigation.
This is an incremental source slice; the B/mobile acceptance gate remains OPEN.

## Behavior

While a DEMO read was pending, focusing a navigation or skip link and then
receiving either data or an error replaced that link and left focus on BODY.
The same happened after starting a manual refresh and moving to a menu link.
Three added Node DOM regressions failed on the unmodified compiled parent:
12 existing tests PASS, all3 added tests FAIL. Full output is retained.

The shell now remembers only its owned, explicitly allowlisted navigation/skip
link and focuses its replacement after repaint. It does not activate the link,
change the selected route or take focus from outside the shell. Existing
language/theme/reload controls and main-content focus rules are preserved.
The allowlist prevents arbitrary selector/URL interpolation; links are plain
local fragments built by the shell. The module is compiled, not hand-edited.

Preview CSS allows long aliases, translated navigation and field labels to
wrap, bounds field labels to their container, and lets narrow topbar items
wrap instead of forcing a single line. No horizontal-overflow hiding or
truncation is added. These are source-level guards, not evidence that every
320px/zoom/font/browser layout now fits; actual rendering must confirm that.

## Checks and boundaries

Development strict positive/negative TypeScript, byte-identical8-module rebuild,
size guards and27 Aurora +15 DOM +14 classic Node tests PASS/0 FAIL/0 SKIP.
The added success matrix executes4 links ×2 languages ×2 themes; rejection and
refresh races are separate tests. This checks programmatic DOM focus, not Tab
traversal, screen-reader behavior or real browser keyboard acceptance.
DOM tests keep network/subresource/HTML execution disabled in an isolated realm.

Shell:20,314 raw /6,698 summed gzip bytes (+341/+74 versus parent).
CSS:6,046 raw /2,006 gzip bytes (+218/+75). Foundation15,508/5,106 unchanged.
These are byte guards, not transfer, CPU, memory, FPS or Android measurements.
No dependency/lock/engine/budget increase, BCC embedding or route activation.

Fresh exact published-SHA tests and actual CI must still be recorded after
publication. CI of parent or independent PR205 does not qualify this source.
This branch is independently based on PR204: PR205's wire tests are not assumed
merged or included in the27-test count. Both tracks retain their own history.

Attempts: fresh board/HEAD/CI/issue assignment checked, unique claim recorded,
isolated worktree created. Two guessed CSS paths were absent before inventory
located `web/aurora/preview/shell.css`. Existing locked37-package install with
scripts disabled succeeded; initial3 focus failures and final passes retained.
npm proxy and VM experimental warnings remain explicit. No local Go/gofmt or
supported browser control is available; none was installed or substituted.

Browser/served preview/screenshots/320px/390px/768px/ultrawide/zoom/font loading/
key traversal/screen-reader/reduced-motion/rendered accessibility/performance
remain OPEN/NOT RUN. Native IR↔EX/candidate remains owner-DEFERRED/OPEN.
No API/auth/session/CSRF, operational state, incident/installer or production
change. No main merge/tag/release/transport/shaping/pool/exit switching/
restart-resume/standby action. Full #199 is not accepted by this bounded fix.
