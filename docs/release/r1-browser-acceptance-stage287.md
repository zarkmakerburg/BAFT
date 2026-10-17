# R1-05 browser acceptance method — Stage287

Date: 2026-10-17. Authority: #67 / Stage282 / R1-05. Candidate: draft PR #242.
The input head `db6307db939a8c5ba0987da51d34572f8ff91cdf` passed all four
workflows, including 15/15 jobs in main CI. Fresh outcomes, source SHA, tree,
run ID, artifact ID, defects and visual review are recorded in the Stage287
report on #67. Adding this method is not itself a browser PASS.

## Real browser and real server

The `browser` job in `.github/workflows/bcc-v3-ui-tests.yml` checks out the
exact PR head, builds `web/qa/fixture`, installs pinned Playwright 1.64.1 and
Chromium, and runs `web/qa/bcc-browser-regressions.cjs`. No production URL,
account, release key or operational node is used. Workflow permissions stay
read-only. The existing Node checks remain required before the browser job.

The fixture uses the production BCC handler, cookie login, CSRF validation,
SQLite store, node registry, native plan validation and engine catalog. Each
profile has its own loopback-only listener, temporary store and access file,
random credentials and ephemeral signing/backup keys. Two fixture nodes have
independent X25519 public keys and agent tokens. Their contact is simulated by
two real local HTTP agent polls; no agent process executes jobs. Health,
tunnel, discovery and bootstrap loops do not run. A plan can therefore become
ready for UI review without installing, deploying or changing a host.

Credentials are passed from the fixture to its parent through a private pipe,
kept in memory and excluded from evidence. Screenshots precede credential
entry or show the authenticated app. The browser refuses outside network
requests and unintended API writes. Allowed writes are native plan preview
and disposable node country metadata. No tunnel create/deploy request is
allowed. The temporary process/state is removed after each profile.

## Coverage matrix

All profiles visit Dashboard, Servers, Tunnels, Monitoring, Security, SSL,
Backups, Updates, Finance and Settings. Shared configuration lives in
`web/qa/bcc-browser-matrix.cjs`; the manual screenshot helper now uses the same
eight profiles and all ten pages on mobile too.

| Profile | Viewport | Language / direction | Theme |
|---|---|---|---|
| desk-fa-dark | 1440 × 960 | FA / RTL | Dark |
| desk-fa-light | 1440 × 960 | FA / RTL | Light |
| desk-en-dark | 1440 × 960 | EN / LTR | Dark |
| desk-en-light | 1440 × 960 | EN / LTR | Light |
| mob-fa-dark | 390 × 844 | FA / RTL | Dark |
| mob-fa-light | 390 × 844 | FA / RTL | Light |
| mob-en-dark | 390 × 844 | EN / LTR | Dark |
| mob-en-light | 390 × 844 | EN / LTR | Light |

For each profile, automated browser assertions cover:

- Public pages exclude operational markup and CSRF material; real login sets
  an HTTP-only session with SameSite protection and no credential local storage.
- Actual navigation opens all ten pages, moves focus to main content and
  mounts/opens all 17 Classic operation cards. Screenshots and document-width
  checks accompany the pages. An overflow failure still produces a screenshot.
- The catalog has exactly one admitted action. External choices remain
  disabled; selecting native BAFT opens a collapsed Studio without an API write.
- Real UI input, IPv6 destination port suggestion and native backend plan
  preview produce a reviewed hash and a correctly enabled deployment control.
  A failed subsequent preview retains that review. Editing the destination
  invalidates it; no deployment request occurs.
- A failed catalog refresh followed by an older successful response remains
  unavailable. A fresh successful refresh recovers the catalog.
- A real write without CSRF is rejected; the actual UI country selector writes
  disposable metadata with CSRF and without an Authorization bearer header.
- Language/theme switches, keyboard navigation/focus and reduced-motion rules
  are exercised using actual DOM interaction.
- Actual logout invalidates catalog access. Clearing the test cookie and then
  receiving a real 401 sends the browser to sign-in and removes operational UI.

Only failed plan preview and catalog error/out-of-order responses are stubbed
in Playwright. Those cases prove browser error/race behavior, not backend or
agent fault handling. Other requests use the running BCC. The native HTTP
regressions from Stage284 retain the separate signed-job/ACK/lifecycle proof.

## Evidence and acceptance

The first complete diagnostic run, UI workflow `39025580527`, browser job
`117375819876`, used Chromium `156.0.8078.4` on source
`18b094bce9b0c305389fc0e5070f32bdc0e83f50`. It executed all eight profiles and
mounted all 17 cards in each. Desktop checked 10/10 pages; mobile checked 9/10
because Tunnels overflowed. Artifact `12098746696` contains 116 PNGs and the
summary; ZIP SHA-256 is
`56be02a6763c641b5e66fc43fe5c2159cf259ef8a3476e53788bf84bca3a1f5f`.
This was a failing diagnostic, not acceptance: 36 failure records include
shared causes and cascading checks rather than 36 independent product defects.

The diagnostic reproduced horizontal overflow on all four mobile profiles:
the 390px viewport expanded to 513/515px on Tunnels, and the reviewed plan
expanded to 923px. Grid items retained intrinsic minimum widths and the gates
table lacked its own scrolling wrapper. The bounded correction gives the
operation/tool grids shrinkable columns/items, constrains native selects and
wraps the plan table with the existing horizontal table scroller. It preserves
all columns and does not hide document overflow to make the check pass.

Visual inspection also found the selected wizard step almost white on the
light theme. Native wizard color variables now resolve to Aurora's active
theme tokens. The browser checks the selected-step text against the current
theme text color. Direction chips and the destination port picker receive
the body-only embed styles they were missing.

The discovery table also displayed Go's unset timestamp `0001-01-01…` as if it
were an observation. It now uses the existing not-run state for an absent,
invalid or year-one timestamp. Page assertions reject that sentinel in rendered
observations.

Two harness corrections are separate from product changes: mobile navigation
opens the drawer using its expanded state, and logout first opens the account
popover. The expiry case starts independently of logout success. The late
catalog test awaits the old response's completion before checking the UI;
diagnostic overflow geometry and per-profile notices improve failure evidence.
Fresh browser execution on the corrected source is still required. Its outcome
is recorded on #67 rather than assumed from this diagnostic.

The artifact contains PNG screenshots and `summary.json` with browser version,
source SHA, executed profiles/pages/cards/checks, plan gate outcomes and scrubbed
failures. Logs do not substitute for visual inspection of the screenshots.
The upload runs after failures too and retains evidence for seven days. #67
must link each actual run and artifact and record visual findings; archived
reviewable evidence should be preserved with the handoff.

Local Chromium installation in this session returned truncated non-ZIP data;
the real-browser job provides the execution environment. Local Go fixture
build/vet, HTTP smoke, Node tests, syntax and workflow policy checks remain
useful narrower checks and do not satisfy the real-browser gate.

Even a successful eight-profile run does not qualify live-host execution,
external engines, full accessibility conformance, all browsers/viewport sizes,
carrier continuity, package upgrade/old queued jobs, restore or a release.
R1-06 and final R1-07 gates remain open under the Stage282 program.
