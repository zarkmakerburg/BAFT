# Handoff: server and tunnel cards (BCC Command Center)

Owner request (HQ #67, 2026-10-18): clicking a server name on the **Servers**
page opens a server card (details, BAFT/Agent/BCC versions, its tunnels,
actions). Clicking a tunnel in it opens a tunnel card (details, drift, cert,
edit, update, safe removal). Anything a standard card has, these must have.

## Done in this PR
- `web/bcc-app.js` — block `/* Server and tunnel cards. */`: `openServerCard`,
  `paintServerCard`, `paintTunnelCard`, `runCardAct`, `editTunnel`.
  Data: `GET /api/tunnels`, `GET /api/nodes/versions` (from #252), `overview.nodes`, `nodeDetail`.
- Every action calls the **existing reviewed function** of the Classic operations
  script (`rotateToken`, `revokeNode`, `tnEdit`, `tnDrift`, `tnRotate`,
  `tnDecommission`, `tnCancel`). No new API, no new mutation path. Keep it so.
- Actions by phase: active → edit/drift/rotate/remove; `decommission_failed` →
  retry removal; in-progress → cancel; terminal → none. Revoked server or preview → all disabled.
- CSS fix: `.ops-card>.top>b` (it used to hide every server name on the Updates page cards).
- Tests: `web/tests/server-tunnel-cards.test.cjs`.

## Not done (next agent: continue here, in this order)
1. Translate tunnel phase badges in the cards (reuse `phaseTr` in `web/bcc-operations-v3.js`).
2. Per-server **update** button: today it only jumps to `#updates`. Wire it to the
   owner-approved Update Manager plan→confirm flow once BCC "Update All"
   (P0-UUM-03/04) exists. Do not invent a deploy call.
3. Tunnel card: show cert-rotation history (`GET /api/cert-rotations`, filter by `tunnel_id`) and last jobs.
4. Make the dashboard's server rows open the same card (`serverRow` in `bcc-app.js`).
5. Deep link (`#servers/<id>`, `#servers/<id>/<tunnel>`) so a card can be shared.
6. Real agent ↔ real BCC e2e so the version badges show `verified` from real data (open item of #252).

## Rules (from HQ)
Small PRs with tests, evidence for every claim, no merge/release/deploy without
HQ approval, never touch running user services or tunnels.

## Known pre-existing failure (not from this PR)
`web/tests/aurora-card-routing.test.cjs` fails on the #252 base: the Classic
"BCC Backups" card has no Aurora route in `web/bcc-operations-v3.js` `routes`.
