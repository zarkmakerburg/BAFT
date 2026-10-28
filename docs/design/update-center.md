# BCC Update Center (v0.4.0 P0)

Status: implemented (`internal/bcc/updatecenter*.go`, `web/bcc-updatecenter.js`). Mirror fetch is designed here and deliberately not implemented.

## What already existed (audit) and what this builds on

| Primitive | Where | Used for |
|---|---|---|
| Signed manifest, key cert, root-signed revocations, `VerifyDir`, `CompareVersions`, `TrustState` (no downgrade / re-tag) | `internal/release` | Import verification (same code and rules; see "What BCC verifies") |
| Component digest reports, operator-confirmed platform, `TrustedNodeArtifactName`, `CompareVerifiedComponentDigest` | `internal/bcc/component_report*`, `node_platform*`, `node_version*` | Installed version of each node (never an agent-claimed version) |
| `update_baft vX.Y.Z` signed agent job: pulls the release itself, `VerifyDir` against its pinned root, keeps `.prev`, swaps, restarts, rolls back if not active, advances trust state only on success | `internal/agentjob`, `internal/agent`, `POST /api/deploy` | The upgrade executor. It replaces `baft` and `baft-pair` only, not `baft-agent` or `baft-bcc` |
| Verified installer: `--offline DIR`, `--plan`, `--allow-downgrade`, backups, health check, automatic rollback, `release-state.json` | `install.sh`, `scripts/release/offline_bundle.sh` | Rollback, and any upgrade the agent job does not cover |
| Encrypted backup (schema 4), SQLite migrations (schema 15), restore with anti-rollback merges | `backup.go`, `statedb.go`, `restore_*.go` | Persistence of imports (now schema 16 / backup 5) |
| `baft` terminal menu `Update (planned)` | `cmd/baft/menu.go` | Now read-only guidance |

There is no `baft update` subcommand and no BCC-side executor. **The Update Center therefore invents no executor.** Upgrade = the existing signed `update_baft` job (queued through the existing deploy path); rollback = the verified installer, run by the operator on the node. BCC prepares the exact reviewed steps and tracks the result through component reports.

## Data model (SQLite table `update_center`, inside `state`)

* `UpdateRelease` keyed by manifest-payload SHA-256 (`digest`): version, commit, created_at, signing key id, signed artifact list, received_at, `rollback_only`, and the original signed manifest and certificate envelopes (so every read re-verifies against the current root pin).
* `UpdateRevocationState`: the newest accepted root-signed revocation list (sequence, expiry, revoked key ids, envelope). Only ever moves to a higher sequence.
* Derived on read (never stored as truth): channel (`prerelease` if the version has `-`), signature status (`verified|invalid|unpinned`), revoked, per-node status.

## API (all admin-authenticated; bearer or session + CSRF; strict JSON, size-capped)

* `GET /api/updates`: `{root_pinned, floor, revocation_sequence, revocation_expired, releases[], latest, nodes[], notice_count, bcc_version}`. Node row: `node_id, role, installed_version, latest_version, status, notice_code, notice, last_report_age_seconds, components[]`. `status` is `current | update-available | downgrade-blocked | unknown | revoked`; `unknown` carries a reason (`no_platform`, `no_report`, `stale_report` (>15 min), `unmatched_digest`, `no_release`, `no_root`, `node_revoked`).
* `POST /api/updates/import` (256 KiB): bundle JSON `{schema_version, manifest, cert, revocations, sha256sums}` made by `scripts/release/update_center_bundle.sh` from an offline bundle, or revocations only. 201 imported, 200 unchanged, 400 invalid/unsigned/tampered, 409 revoked / older than floor / re-tag / older revocation list, 413 oversize, 503 no root pinned.
* `POST /api/updates/plan` `{node_id, kind: upgrade|rollback, target_digest}`: deterministic plan `{hash, ok, gates[], steps[], verify[], rollback[], notes[], executor, installer_preview, installer_command, apply}`. The hash covers node, kind, installed and target identity, revocation sequence and every gate.
* `POST /api/updates/apply` `{node_id, kind:"upgrade", target_digest, plan_hash, confirmation:"UPDATE <node> <version>"}`: recomputes the plan; refuses (409) on any hash drift or failing gate, on a second in-flight job for the node, and always for rollback (400). Otherwise it calls the existing `CreateDeployJobs` for exactly that one node (same signed `update_baft` job, same audit intent), with the plan hash in the audit details.

Audit: `updates.import` (success in the same SQLite transaction as the state change; failures with a reason), `updates.plan`, `updates.apply` failures, and the existing `deploy.create` intent carrying `update_center:true` and `plan_hash`.

## Security model

BCC **never auto-applies** and never polls anything. A release is usable only if all hold:

1. **Signed by the pinned root.** The root is pinned at BCC start (`--update-root-pub`, else the signed-catalog root); no request can change it. BCC verifies the chain root → release-key cert → manifest with the `internal/release` primitives. Unlike `VerifyDir` it has no artifact bytes: it checks the certificate, key validity window, manifest schema/product/version/commit, `signing_key_id`, the artifact list, and that the supplied `SHA256SUMS` matches `sha256sums_sha256` and the artifacts. Every node still runs full `VerifyDir` on the bytes before it changes anything.
2. **Not revoked**, judged against the newest accepted root-signed list (must not be expired; sequence never goes back, also across restore). A later list revokes already imported releases; the installed-and-revoked state raises a notice.
3. **Not a downgrade.** Floor = newer of BCC's own version and the signed catalog's trust-state version. Below the floor an import is refused unless the header `X-BAFT-Downgrade-Confirmation: DOWNGRADE <version> <commit>` is sent (same convention as `/api/nodes/versions/accept`); it is then stored `rollback_only` and is never offered as an upgrade target. Same version with another commit is refused (re-tag). A node's own anti-downgrade state still applies.
4. **Per node, admin initiated, staged.** One node per apply, typed confirmation, plan hash bound, no job already in flight. Health check and automatic rollback are the agent job's: `.prev` restored if the service is not active; trust state advances only on success. Operators update servers one at a time.
5. Inputs are size-capped and strict-decoded; the browser never renders server text as markup.

**Offline first.** The only release source is an operator-imported bundle. No background fetch, no phone-home.

**Mirror (designed, not implemented).** An optional admin action `POST /api/updates/fetch` would GET a configured `https://` mirror URL: TLS verification on, redirects off, response capped at 256 KiB, destination IPs resolved and checked at dial time (reject loopback, private, link-local, ULA, multicast, unspecified, metadata ranges), then the body goes through the same import path. Off by default.

## UX states

Updates page: root pin and floor, revocation sequence/expiry, latest verified release; servers table (installed, latest, status badge with notice, last report age, `Upgrade plan` / `Rollback plan` with a chosen older release); releases table (signature, revoked, rollback-only). The header shows an `N updates` badge when servers are `update-available` or `revoked`. The plan view shows gates, steps, verification, rollback guidance, exact commands (copy buttons) and, for an OK upgrade plan, the confirmation field. Rollback plans show the installer preview and command only. Persian and English; identifiers stay LTR.

## What it does not do

It does not update BCC itself, does not replace `baft-agent`/`baft-bcc` on nodes through the agent job, does not hold binaries, does not run a rollback, does not push anything to a node, does not fetch from the network, and never trusts an agent-reported version. A node without an operator-confirmed platform or a fresh report stays `unknown` and its plan is blocked.

## Backup, restore, schema

SQLite migration 16 (`update_center`) and backup schema 5. A backup with a header older than 5 may not carry Update Center state. Restore merges instead of overwriting (`mergeUpdateCenter`): releases are unioned and the higher revocation sequence wins, so a restore can neither forget an import nor un-revoke a key. Downgrading to a build without schema 16/backup 5 needs a verified pre-upgrade snapshot (KNOWN-LIMITATIONS).
