# P0-UUM-03 — component digest receipt persistence design

Status: **design only**. This document does not imply that the BCC receiver, signed-manifest verification, or durable replay defense is implemented.

## Current architecture

BCC stores `state` in SQLite through `internal/bcc/statedb.go`. `writeStateDB` opens a transaction and writes the state with `writeStateTx`, then reconciles uncertain commits by reading back durable state. `readStateDB` reconstructs `state` and `normalizeState` supplies map defaults. Existing agent authorization is `Store.authorizedHashLocked(nodeID, token, now)` and excludes revoked nodes.

## Required implementation

1. Add a per-node `ComponentDigestReceipt` map to `state` and initialize it in `normalizeState` and `OpenStore` (or ensure normalization covers all startup paths).
2. Add an **append-only schema migration** for a `component_digest_receipts (node_id TEXT PRIMARY KEY, doc TEXT NOT NULL)` table. Include it in `stateTables` and both `writeStateTx` and `readStateDB`. Never edit a previously applied migration.
3. Implement one Store operation holding the mutex across: node/token authentication, freshness and monotonic replay check against the **persisted** cursor, digest/manifest verification, state update, `saveLocked`, and rollback of in-memory state on save failure. Respect `persistenceUncertain` fail-closed behavior.
4. Bind the receiver to a new POST `/api/agent/component-report` endpoint. Enforce bounded body, strict JSON, node ID matching authenticated identity, size/length constraints and controlled error messages. Do not mark agent versions `current` based solely on self-reported data.
5. Agent reports only explicit component paths and observed SHA-256 hashes, with UTC observation time; use the existing bearer-token transport and TLS configuration. Do not transmit binary bytes or secrets.
6. BCC must compare the observed hash to a **verified signed release manifest** and distinguish `verified`, `mismatch`, `unknown`, and `stale`. Unknown/failed verification must never become `verified`.

## Acceptance gates

- Fresh authenticated report persists across BCC restart and can be read back.
- Duplicate, out-of-order, stale, future, wrong-token and revoked-node reports are rejected.
- Failed SQLite commit and uncertain commit fail closed; restart reconciliation is tested.
- Signed-manifest mismatch and missing manifest show unknown/mismatch rather than `current`.
- Existing JSON-to-SQLite migration and backup/restore tests remain green.
- Full PR exact-head CI, static analysis and BCC UI gates pass before merge/release.

## Safety note

Do not merge PR #252 merely because the digest helper and isolated tests pass. The authenticated endpoint and durable, signed verification must be integrated and exercised first.
