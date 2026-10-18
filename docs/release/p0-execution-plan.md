# P0 execution plan after v0.3.4

Recorded 2026-10-17 from the owner's P0 directives in issue #67 and the state of
the repository that day. This is a plan and a status table, not a release claim:
nothing here authorizes merge, tag, signing, publication or rollout, and a row
is "done" only where a PR, its tests and CI are named.

## Baseline (checked 2026-10-17)

| Item | State |
|---|---|
| `main` | `272232b` (v0.3.4 source). The published v0.3.4 is signed by key `702c62788d60920da8ea80aab93ca3e1` and verifies against the pinned root. |
| #247 menu fix | head `b152223`, 17/17 check runs SUCCESS, open, not merged. |
| #248 `baft update check` | draft, stacked on #247. |
| #249 per-component inventory | draft, stacked on #248. |
| #250 `baft update apply` | draft, stacked on #249. |
| Other open PRs that touch these P0s | #197 (add a tunnel to a server that already has one), #200 (smart tunnel), #231 / #234 / #239 / #240 (service-agnostic wizard and toolbox catalog), #235 (installer skel fix), #246 (owner release). They are older drafts on their own bases; none was merged or rebased here. |

## What exists, what does not (evidence)

| Owner P0 | Exists today | Does not exist | Evidence |
|---|---|---|---|
| **P0-UUM-01** truthful per-component versions | A host-local, byte-exact judgement of Core, agent and BCC against the signed release (#249). | Any version or digest reported by an agent to BCC. A BCC `Node` has `AgentSeen`, `Health`, `LatencyMS` and no software version. The agent protocol (jobs / ack / traffic) carries none. | `internal/bcc/store.go:31`; `cmd/baft/update_inventory.go` |
| The 2026 production gap (Core updated, BCC old) | Explained and caught: `release-state.json` is one file per prefix shared by Core and BCC, so it says "current" while the panel binary is old. | | `install.sh` `BAFT_RELEASE_STATE` default; #249 tests |
| **P0-UUM-02** signed release check, plan, Update All, rollback (CLI) | `baft update check` (#248), component judgement (#249), `baft update apply` (#250: role-aware, plan first, allow-listed steps, detached run with a log and an inherited lock, BCC backup gate). The installer's own backup / rollback is tested at each apply point (`installer_faults.sh` 2-6). | Update All and single-component update **from BCC**; a compatibility matrix. The agent's `update_baft` job replaces only `baft` and `baft-pair` (not the agent, not BCC, no config / unit / state backup). | `internal/agent/agent.go:416`; `tests/e2e/installer_faults.sh` |
| **P0-UUM-03** UI parity | Nothing. | Version / Update controls on Classic and Aurora. | no match in `web/bcc-app.js`, `web/bcc-operations-v3.js` |
| **P0-UUM-04/05** continuity, verification | Installer rollback tests; the real CLI against the real installer in section 7 of `installer_faults.sh` (transport, BCC and agent updates with byte-identical credentials; a stopped agent and a missing BCC backup refused). | Two-node live canary; SSH / network interruption on the BCC path; 404 / 429 browser regressions. | |
| **P0-TW4-01** manual wizard without extra fields | | The wizard still asks for "EX service destination (IP:port)" (`#tnTarget`) and has a "Suggest destination port" button. Targets are not resolved from inventory. The engine inventory contract exists in `main` but is not connected to live probes. | `web/bcc-operations-v3.js:170`; `internal/tunnelengine/inventory.go` |
| **P0-TW4-02/03** port generation and conflict checks | A client-side random port (`crypto.getRandomValues`) checked against BCC's own inventory only. The page says it does not scan or reserve OS sockets. | Node-level bind / reserved / protocol checks, atomic re-validation on apply, a verified replacement flow. | `web/bcc-operations-v3.js:347-366` |
| **Smart Auto** | `/api/ingress/smart*` (ingress policy and provider evaluation). | Engine / port selection from live node capability, the two-node Smart Auto flow. | `internal/bcc/smart_ingress*.go` |
| **P0-TW4-05** Linux menus | #247: unavailable actions are visibly disabled. #248-#249: Update is a real check; apply is a real, gated flow. | Repair; Backup / Restore actions beyond guidance text; BCC menu beyond printed commands; arrow-key navigation; tunnel create / edit / delete from the CLI. | `cmd/baft/menu.go` |
| **P0-SC-TC** server and tunnel cards | Tunnels are independent instances with their own generation baselines. | Per-server and per-tunnel cards in either UI. The 17 "cards" of R1-04 are top-level operational tools (Servers, Tunnels, Backups, ...), not the card of one server. | `internal/bcc/store.go:46-50`; no "server card" / "tunnel card" in `web/`; `docs/release/r1-classic-aurora-parity-stage286.md` |
| **P0-SRV-VER** upgrade / downgrade | Anti-downgrade in the installer and trust state; `--allow-downgrade` exists. | Version selection, downgrade preflight (schema compatibility), reverse-migration refusal, history. | `install.sh` `BAFT_ALLOW_DOWNGRADE`; `internal/release` |
| **P0-BCC-BR** backup / restore centre | Encrypted daily / weekly backups, `restore-preview`, offline `restore`, `verify-backup`, `/api/backups*`. | Scoped backups (per server, per tunnel, selective restore), off-host copy, a full-backup manifest that proves coverage. | `internal/bcc/backup*.go`, `restore_*.go` |
| **P0-BCC-UC** update centre | | Everything. | |
| **P0-BCC-NOTIFY** release notification | An alert engine and audit log. | Release polling, an inbox / bell, per-version dedupe, persistence. | `internal/bcc/alert_engine.go` |
| **Parity** CLI x Classic x Aurora | | A shared management contract. Today the CLI and BCC each have their own code paths. | |

## Order of work (dependencies first)

1. **A. Baseline** (open): #247 green and merged by the owner; #248, #249 and #250 reviewed. Retarget the stack to `main` as each lands. `r3` runs only for PRs based on `main`, so it has not run on the stack yet.
2. **B. Shared management contract** (design first): one versioned operations API that both the CLI and BCC call (node and component inventory, release catalog, update plan / apply, job status). Without it every later row is built twice.
   - First slice: an **agent -> BCC component report** of binary digests (the technique #249 uses locally), compared in BCC with the signed release digests. This is the missing truth for P0-UUM-01, the server card's version fields and the update badge.
3. **C. Smart tunnel creation**: node-level capability and port checks (P0-TW4-02/03) before the wizard change, because the wizard's removal of the destination field is only safe once a destination can be proven from inventory. Then the manual wizard, then Smart Auto.
4. **D. Cards**: server cards need B's inventory; tunnel cards need C's instance identity.
5. **E. Update / backup / notification in BCC**: update centre and notifications need B; scoped backups need D's identities.
6. **F. Release gate**: exact-head CI, race, multi-tunnel isolation (Germany), interruption tests, independent signing. Owner approval for merge / tag / publish / deploy.

## What the update flow decides (and may be changed)

- Apply runs the release's own signed installer, once per installed component, in the mode it was installed in (`--role ir|ex` from the node's config, `--agent-only`, `--bcc-only`). Owner-approved rule.
- Order: BCC, transport service, agent. `baft` and `baft-pair` are files the transport service runs and the agent installer also installs, and an installer restarts only its own service; the transport must replace them first or its process keeps the old code while the files look current. (An earlier draft put the agent first; an independent review found that defect, and #250 now carries the fix and a test.)
- An installer step may change binaries, the release state and restarts only. Keys, certificates, config, pairing, tokens, access files and units may not, and an unknown step is refused. This is checked on the installer's `--plan` and again on the plan it prints after it has downloaded the release. The one accepted unit change is the BCC unit's `# baft-bootstrap-sha256` comment, which moves with the bootstrap script a release ships; any other unit change is reported after the run.
- No pass may replace `baft` while another BAFT transport service on the host runs it and is not part of the run.
- BCC needs a `*.baftbak` backup newer than 26 hours, or `--skip-backup-check`. **My choice** of limit.
- The installer runs in its own session with its output in `<prefix>/update-logs/`, so a dropped session cannot leave a half-applied install.

## Decisions for the owner

1. **Manual wizard inputs.** The 2026-10-17 19:38Z directive says exactly four inputs (Iran node, foreign node, tunnel port, ingress port). The 2026-10-17 report lists five with the engine first. With only the Native engine selectable the difference is invisible today; which is the contract once a second engine is real?
2. **Agent -> BCC report.** Binary digests compared with signed release digests (proposed), or self-reported version strings? Digests need no cooperation from old binaries; strings need every binary to carry one.
3. **Hosts with BCC-built tunnels.** `baft update apply` recognises only the unit `install.sh` writes. A transport unit written by the BCC tunnel builder (ownership markers, `baft.managed.json`, possibly several instances next to `baft.service`) is refused, and so is any run that would replace `baft` under such a service. That is the safe default, but it means those hosts cannot use `baft update apply` for the transport yet. The fix is a coordinated per-instance restart owned by BCC (P0-UUM-03/04), not a longer allow-list in the CLI. Do you want that first, or recognition of the primary managed unit as an interim?
4. **Allowing unit changes in an update** when a future release legitimately changes a unit template (currently refused, apply by hand).
5. **BCC backup freshness** limit and whether `--skip-backup-check` should exist at all.
6. **Retire the older key** `41f8a2dc…` only after every live host runs a release signed by `702c…` (the standing R1-07 decision); the update flow does not change that.
