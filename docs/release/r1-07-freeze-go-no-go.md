# R1-07 — v0.3.4 freeze and GO/NO-GO record

Plan 2026.10.17.1 (HQ Stage282), queue item R1-07. Recorded 2026-10-17.
This record does not authorize merge, tag, signing, publication or rollout.

## Candidate

| Item | Value |
|---|---|
| PR | #242 (draft, unmerged) `hq/r1-stage283-v034-integration` |
| Frozen source head | `6dfd377d26348b7768df224b4f86087db81f220b` |
| Base | `main` `04e2cc71aa616049f488e72561982f7a2edeb86f` (ancestor of the head; 22 commits ahead) |
| Previous public release | v0.3.3, commit `90b77bb87a6778e7346f6f06820bc79a2680d5d5` |
| Diff vs base | 47 files, +3504 / -126 |

This record and the CHANGELOG entries are added on a branch stacked on the
head above. They touch only `CHANGELOG.md` and this file; no code changes.

## Gate matrix (R1-01 … R1-06)

| ID | Scope | Result | Evidence |
|---|---|---|---|
| R1-01 | PR overlap audit, source choice | PASS | `docs/release/r1-v034-integration-stage283.md` |
| R1-02 | integrated candidate | PASS | PR #242 |
| R1-03 | HTTP create/edit/delete, sibling, IPv4/IPv6, PlanHash | PASS | `docs/release/r1-http-regressions-stage284.md` |
| R1-04 | Classic/Aurora parity, 17 cards | PASS | `docs/release/r1-classic-aurora-parity-stage286.md` |
| R1-05 | real Chromium, 8 modes, 80 page visits, 160 checks | PASS | `docs/release/r1-browser-acceptance-stage287.md`, UI run 39027638926 |
| R1-06 | fresh signed install, v0.3.3 upgrade, seeded rollback, signed-job compatibility (disposable systemd lab) | CONDITIONAL PASS | #67 Stage309–317; cross-version agent run used a test BCC endpoint |
| CI | 19/19 check runs on the exact head | PASS | #67 Stage319 |
| Race | `go test -race` agent, agentjob, bcc on the exact head | PASS | this record |

### Race detector on the exact head

Run in a clean cloud container (linux/amd64, go1.27.1), separate from the Mac
lab run in Stage321, which left the BCC package unfinished.

Command: `go test -race -count=1 ./internal/agent ./internal/agentjob ./internal/bcc`
at `6dfd377d26348b7768df224b4f86087db81f220b`, 4 CPUs.

```
ok  github.com/zarkmakerburg/baft/internal/agent     74.006s
ok  github.com/zarkmakerburg/baft/internal/agentjob   1.051s
ok  github.com/zarkmakerburg/baft/internal/bcc      487.530s
real 9m9s, exit 0, no data race reported
```

## Release trust calendar

Read from the published release assets on 2026-10-17.

| Release | Signing key | Key certificate valid until |
|---|---|---|
| v0.3.1 | `702c62788d60920da8ea80aab93ca3e1` | 2027-10-12T18:01:00Z |
| v0.3.2 | `41f8a2dccb57be62f7a917fb2cc929f2` | 2027-03-31T06:49:22Z |
| v0.3.3 | `41f8a2dccb57be62f7a917fb2cc929f2` | 2027-03-31T06:49:22Z |

`release/keys/revocations.json`: sequence 1, no revoked keys, valid until
2027-03-31T05:08:48Z.

Findings:

1. **v0.3.2 and v0.3.3 were signed with the older key `41f8a2dc…`,** although
   v0.3.1 had already moved to `702c6278…` and the old key was marked for
   revocation. Revoking `41f8a2dc…` now would make the v0.3.2/v0.3.3 artifacts
   (and their offline bundles) fail verification on hosts that apply the new
   revocation list. Proposal: sign v0.3.4 with `702c6278…`; revoke
   `41f8a2dc…` only after every live host runs a release signed by the newer key.
2. **The revocation list and the old key certificate both expire on
   2027-03-31.** A new root-signed revocation list must be issued before then.
   Renewal starts T-30 (2027-03-01); operational warning T-7 (2027-03-24).
   An expired list or certificate has no bypass.
3. The newer key certificate expires 2027-10-12: renewal from 2027-09-12,
   warning from 2027-10-05.

## Changelog

`CHANGELOG.md` had no entries for v0.3.2 or v0.3.3 and still listed the
v0.3.2 installer work as Unreleased. This branch adds the v0.3.2 and v0.3.3
entries from the merged PRs and fills `[Unreleased]` with the v0.3.4
candidate's user-visible changes. At tag time `[Unreleased]` becomes
`[0.3.4] - <date>`.

## Rollback

- Hosts keep the v0.3.3 signed artifacts; rollback is reinstalling v0.3.3
  from its signed release (`BAFT_VERSION=v0.3.3`).
- BCC: upgrade v0.3.3 → candidate and the injected pre-commit failure
  rollback passed in the lab (Stage311, Stage314). Post-commit and crash
  failure classes are not yet tested (P1 below).
- Take a BCC backup before the upgrade; restore is the documented path if
  the schema change has to be undone.

## Blockers and open items

| Priority | Item | Owner |
|---|---|---|
| P0 | Written merge and release authorization from the owner | zark |
| P0 | Choice of signing key for v0.3.4 (finding 1) | zark |
| P0 | Official offline signing of the tagged `main` commit; `scripts/release/accept.sh` on the published tag | zark (signing), HQ (verification) |
| P1 | Decide whether a persisted old-BCC job → new agent drill under real systemd is required before release (R1-06 is conditional without it) | zark |
| P1 | Rollback failure injection after commit and on crash; non-node record preservation | HQ |
| P2 | Five-node lab and live carrier validation | deferred |

## Decision

**NO-GO until the P0 items are closed.** Engineering gates R1-01 … R1-05 and
CI pass on the exact head; R1-06 is conditional. When the owner writes the
authorization and chooses the key, the order is: merge #242 (and this branch)
to `main` → re-run CI on the merge commit → tag → unsigned CI candidate →
offline signing → `accept.sh` → publish → rollout, one host at a time, with
the read-only runner checking each host.
