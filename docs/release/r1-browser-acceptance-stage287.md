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
