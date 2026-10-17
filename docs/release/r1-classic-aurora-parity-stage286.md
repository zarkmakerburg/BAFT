# R1-04: Classic / Aurora capability and API parity — Stage286

Date: 2026-10-17. Authority: issue #67, Stage282 program, R1-04.
Candidate: draft PR #242, branch `hq/r1-stage283-v034-integration`.
Audit input: `de4d9b13d793d24742e0d238fd3a22b8aa732f91`.
Publication SHA, tree and fresh CI conclusions belong in the Stage286 report on
#67; this document does not attempt to embed its own future commit hash.

## Result and evidence boundary

All 17 top-level operational cards in Classic have a unique Aurora tool and a
matching page registration. Both interfaces use the existing BCC handlers and
the same embedded operational implementation. The audit found and corrected an
unreachable engine toolbox, three inaccurate page metadata entries and a native
engine selection that left a collapsed Studio closed. The toolbox now has
Aurora theme-token styles because the Classic stylesheet is not embedded with
its body markup.

This is a source, API-contract and automated behavior parity result. It is not
eight-mode browser acceptance, an SSH installation, a real agent probe, an
external-engine admission, an upgrade/restore test, or a release decision.
R1-05, R1-06 and the final release gates remain separate work.

## How operational cards reach Aurora

`internal/bcc/access.go:serveCommandCenterV3` includes the Classic body and
script only in the authenticated dashboard configuration. The public welcome
and sign-in configurations do not receive operational markup or CSRF material.
`web/bcc-operations-v3.js` registers each card by its original heading, prefixes
embedded element IDs and bridges requests to the secret base path.
`web/bcc-app.js:OPS` maps tools to those headings; `mountOps` places the same
card in the selected page and `parkOps` retains it for polling after navigation.

The card matrix below identifies implemented controls rather than treating a
heading, a route registration, or a planned feature as a working operation.
Paths are relative to the authenticated BCC base. `GET` rows read data;
`POST` rows invoke the named server handler and may audit, queue work or change
managed state. Plan and preview operations can append audit records even when
they do not deploy or restore anything.

## Preserved Classic cards

| Classic card | Aurora page / tool | Controls and implementation | Real API | Capability boundary |
|---|---|---|---|---|
| Live Monitoring | Monitoring / `live` | `loadMonitoring`; periodic refresh | `GET api/monitoring` | Read-only observations, including unknown/unavailable states |
| 7-Day Traffic & Uptime | Monitoring / `history` | Node selector, `loadHistory`, `drawHistory` | `GET api/history?node_id=…` | Read-only recorded history; no synthetic live traffic |
| Register / update node | Servers / `manual` | Save, `saveNode` | `POST api/nodes` | Registry mutation; does not install an agent |
| Add a server over SSH | Servers / `ssh` | Connect/install, `bsRun`; build-tunnel handoff, `bsTunnel` | `POST api/bootstrap/hostkey`, then `POST api/bootstrap`; handoff itself is local | Real privileged bootstrap when configured and invoked; credentials cleared after response; field execution not exercised by this audit |
| Existing tunnels (discovery) | Tunnels / `discovery` | Discover all, `dcStart`; refresh, `loadDiscovery` | `POST api/discovery?all=1`, `GET api/discovery` | Runs discovery jobs and displays observations; does not adopt or replace discovered tunnels |
| Layered health | Monitoring / `health` | Refresh, `loadHealth` | `GET api/health` | Read-only layers, transitions and evidence |
| Universal Tunnel Toolbox | Tunnels / `toolbox` | Refresh, `loadTunnelEngines`; native selection, `tlPickEngine` | `GET api/tunnel-engines`; selection is local | Only backend-admitted native BAFT is selectable. Unavailable engines/carriers stay disabled; selection sends no plan/create request |
| Aurora · Tunnel Control Studio | Tunnels / `tunnels` | Native wizard, plan, deploy, edit, cancel, drift, certificate rotation, decommission; see lifecycle table | Existing `api/tunnels*` handlers | Real native lifecycle, explicit instance and plan-hash gates; no external driver or generic direction execution |
| Change History | Tunnels / `changes` | Node/tunnel filters, `chLoad` | `GET api/change-ledger?node_id=…&tunnel_id=…` | Read-only recorded managed changes; no history-based rollback button |
| Safe SSH Port Migration | Servers / `migrate` | `smPlan`, `smStart`, `smVerify`, through `smCall` | `POST api/ssh-migration` with `action: plan/start/verify` | Real staged migration with existing backend validation and rollback handling; not a cosmetic port setting |
| Route Doctor | Tunnels / `doctor` | Run, `rdRun` | `POST api/route-doctor` | Diagnostic operation; not automatic route repair |
| Automatic Path Discovery | Tunnels / `paths` | `pdRun`, `ppRun`, graph, discovery state, nested Connectivity Matrix and probe results | `POST api/path-discovery`, `POST api/path-probes`; `GET api/path-graph`, `api/path-discovery`, `api/path-matrix`, `api/path-probes` | Active probes can queue transient work. Recommendations are evidence, not an automatic deployed route. Connectivity Matrix belongs to this card, not an eighteenth top-level card |
| Cluster Nodes | Updates / `deploy` | `loadNodes`, checkbox selection, `deploy`, `rotateToken`, `revokeNode`, refresh | `GET api/nodes`; `POST api/deploy`, `api/nodes/rotate-token`, `api/nodes/revoke` | Real deploy/revocation/credential operations; backend permission, job and version checks still apply |
| Recent Jobs | Updates / `jobs` | `loadJobs`, polling | `GET api/jobs` | Read-only recent status; no generic retry/cancel action |
| Immutable Audit Log | Security / `audit` | Refresh, `loadAudit` | `GET api/audit` | Read-only audit entries; no UI deletion/editing |
| Node Finance | Finance / `finance` | `loadFinance`, rate action `setFinance` | `GET api/finance`, `POST api/finance` | Reads measured/accounted totals and writes versioned rates; not billing-provider integration |
| Finance Reports | Finance / `reports` | Period/date/timezone filters, `loadFinanceReport`, `downloadFinanceCSV` | `GET api/finance/report`, optionally `format=csv` | Read-only JSON/CSV export using real report data |

### Native Studio lifecycle and local helpers

| Action | Implementation | Real API or local effect |
|---|---|---|
| Refresh/select an existing tunnel | `loadTunnels`, `tnEdit` | `GET api/tunnels`, `GET api/tunnels?id=…`; editing fills the form before a new plan |
| Preview | `tnPlan` | `POST api/tunnels/plan`; reviewed request and returned hash retained together |
| Deploy/create or replace | `tnDeploy` | `POST api/tunnels` with reviewed `plan_hash`; HTTP 409 requires review of the fresh plan |
| Cancel and rollback | `tnCancel` | `POST api/tunnels/cancel`; signed rollback keeps the selected instance scope |
| Drift check | `tnDrift` | `POST api/tunnels/drift?id=…`; displays subsequent tunnel observations |
| Rotate TLS certificate | `tnRotate` | `POST api/tunnels/rotate-cert`; existing rotation transaction, not a local status toggle |
| Remove a managed tunnel | `tnDecommission` | `POST api/tunnels/decommission/plan`, confirmation, then `POST api/tunnels/decommission` with hash |
| Suggest service/target port, advance/restart wizard, choose engine | `tnWizardGenerateServicePort`, `tnWizardSuggest`, `tnWizardStep`, `tnNew`, `tlPickEngine` | Local form helpers. Suggestions are advisory and reserve no remote listener; changed targets invalidate review |

The Stage284 HTTP regression document remains the source for lifecycle,
endpoint validation, sibling preservation and rollback test scope. Agent
observations in those tests are fixtures, not proof of live-host behavior.

## Aurora additions outside the Classic card matrix

| Surface | Backing source / action | Availability and limit |
|---|---|---|
| Dashboard totals, graph/map, selected node/tunnel, health, costs, events | `GET api/overview?window=…`, `api/alerts`, `api/audit` | Read-only server observations. Range, selection, search and map layout are local presentation controls |
| Dashboard Route Doctor | Open selected tunnel locally, then `POST api/route-doctor` from its Run action | Real diagnostic; opening the panel alone does not run it |
| Dashboard backup shortcut | `POST api/backups/create` after confirmation | Creates an encrypted backup under configured backend policy; not an export of fake preview data |
| Server summary and country selector | Overview plus `GET api/nodes`; `POST api/nodes/location` | Country edit writes registry metadata. Add-server control opens the existing SSH card |
| Monitoring alerts | `GET api/alerts` | Read-only active alerts; no unwired acknowledgement/action button |
| SSL / certificates | `GET api/cert-rotations`; link to Tunnels | Read-only rotation history; rotation itself is the native Studio action |
| Backup list and creation | `GET api/backups` returning `{backups: […]}`; `POST api/backups/create` | Real backend entries and creation. Non-201 reconciliation-required response is shown as uncertain rather than success |
| Restore preview | `POST api/backups/restore-preview` with a listed filename | Verification/preview only; may audit. Actual restore remains an offline/operator procedure, with no misleading web restore button |
| Language, theme, map style, range preferences | Local preference store | Presentation preferences; no invented server settings endpoint |
| Session, sign-in/sign-out, Classic link | Secret-base login/logout routes, session cookie, CSRF-protected API bridge | Real access flow. Classic retains its own sign-in and remains accessible |
| Support, documentation, community/donation navigation and copy helpers | Links and local copy actions | Information/navigation, not server administration or backend health evidence |
| Public preview / demo | Explicit preview mode and fixture data | Labeled DEMO; API helper refuses preview requests and operational scripts are not injected as a signed-in session |

### API-only or future work

The router also contains topology/reconcile, ingress policy/evaluation/
distribution/smart-provider, path-probe cancellation and certificate-rotation
cancellation APIs. Their registration does not imply a dedicated working
Classic or Aurora control. They remain API-only unless explicitly listed above;
no placeholder operational button is added in this stage. Agent job/ACK/traffic
and enrollment endpoints are machine interfaces, not missing admin cards.

The Stage283 inventory evidence library remains a schema/contract foundation;
there is still no connected live inventory probe/store/API. The future roadmap
of 15 tunnel families exceeds today's limited catalog and is not completed by
making the toolbox visible. External engines require real packaged drivers,
admission evidence and lifecycle/rollback tests. WSS/QUIC are carriers rather
than independent tunnel engines. Native V1 keeps its existing topology;
generic engine/dialer/listener/direction modeling remains D-01 onward.
Application account, subscription and payload configuration management are
outside the tunnel-control contract.

## Reproduced defects and bounded fixes

1. The Classic toolbox heading had no card route, `OPS` key or rendered Aurora
   tool. The mount script parked it hidden indefinitely. Added all three links
   and FA/EN title/subtitle keys; all 17 cards now have exactly one mapping.
2. Route metadata said Cluster Nodes belonged to Servers and finance cards to
   Security, although their actual tools render in Updates and Finance.
   Corrected metadata to the existing page placement.
3. `tlPickEngine('baft')` scrolled to the Studio but did not open its enclosing
   Aurora `<details>`. It now opens that holder before scrolling. Classic has
   no such holder and keeps working. Unsupported choices still return early.
4. Toolbox tile styles lived only in the Classic document head, which the
   authenticated body embed excludes. Added scoped Aurora grid/tile/status
   rules using current light/dark tokens, narrow-width sizing and reduced-motion
   handling. Visual acceptance remains R1-05.

No backend handler, schema, dependency, deployment policy, driver admission,
queued job or signing configuration changes in this stage.

## Verification and next handoff

- New reverse-coverage test derives all top-level Classic card headings and
  checks unique registration, rendered tool and matching page metadata. Its
  parent-state failure identifies the unreachable toolbox; the prior test only
  checked tools that were already registered.
- New catalog/Studio VM behavior test collapses the Studio, rejects an
  unsupported engine, chooses BAFT and checks that the Studio opens while the
  sole request remains the authenticated catalog `GET`. This fails against the
  unchanged parent implementation.
- Focused two-file run: 9/9 pass after the two new tests first failed on the
  parent state. Full UI workflow command: 71/71 pass, no skips, using
  `node --test web/tests/*.test.cjs internal/bcc/testdata/*.test.cjs`.
- Focused Go race checks cover the dashboard, native Command Center session,
  Classic sign-in, secret path, login/CSRF and fail-closed engine catalog:
  PASS (7.341s). Documentation checks pass (0.026s), vet passes for
  `./internal/bcc ./web`, JavaScript syntax passes for 16 files/scripts, and
  `git diff --check` passes. Supply-chain policy passes for all 13 workflows.
- The audit input's four workflows, including 15/15 main CI jobs, passed. These
  results do not qualify the changed Stage286 head; its new workflow IDs and
  conclusions must be recorded separately on #67.

R1-05 must run an actual browser against the authenticated integration build
in FA/RTL and EN/LTR, dark and light, mobile and desktop (eight modes). Exercise
all page/card navigation, collapsed toolbox-to-Studio handoff, theme/language
changes, keyboard focus, narrow layout, disabled catalog choices, loading/error
and stale responses, session expiry/CSRF, and native reviewed-plan state.
Use disposable local/test data and fixtures for destructive operations; record
that fixture evidence separately from real-node admission. Node VM checks do
not satisfy this browser gate. R1-06 then covers package updates, queued old
jobs, backup/restore and operational security before R1-07 release qualification.
