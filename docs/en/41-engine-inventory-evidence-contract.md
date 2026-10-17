# Engine inventory evidence contract — Stage280 / #228

This is the first shared data/admission contract for future per-node engine inventory. It is not a new installed-engine endpoint, Agent probe, installer or deployment gate. Existing native deployment behavior is retained. FRP, Rathole and other unimplemented engines remain unavailable.

## Implemented contract

`internal/tunnelengine/inventory.go` separates the inventory schema version, native driver contract version, software release version, and installation observation. The authenticated catalog exposes `contract_version: 1` for native BAFT only. Planned entries omit that field. The catalog response remains an array and contains no per-node installed/healthy claim.

| Input | Authority and validation |
|---|---|
| InventoryChallenge | BCC's current persisted signed-job context: node, job, monotonically increasing generation, random 32-byte nonce, issue/deadline |
| InventoryReport | Untrusted response: exact challenge binding and schema, bounded JSON and entry count, known implemented engines, valid installation facts |
| InventorySnapshot | Opaque immutable value constructed by validation; a zero value or unmarshalled caller flags provide no evidence |
| NodeEngineRequirement | Expected node, current probe generation, and release version/digest from trusted release policy |
| EngineSelection | Explicit native BAFT, driver contract v1, IR-initiated connection, TCP only |

Present installation requires a release version and SHA-256 digest. Unknown/absent entries cannot carry artifact claims. Unknown fields, trailing payloads, duplicate engine entries and unsupported drivers/versions are rejected. No executable paths, commands, credentials, caller-defined ready/verified fields or Agent clock are accepted.

`AdvanceInventory` binds observations to the current challenge, limits its lifetime to five minutes, rejects replay/rollback, reused jobs/nonces, mismatched nodes and clock rollback. A delayed receipt retains the original dispatch deadline; it does not renew the evidence lifetime. Invalid candidates return no snapshot and do not mutate the previous value.

`RequirePairInstalled` needs observations from two distinct expected nodes. Both must be fresh, match their current persisted probe generations, and report the independently expected release version/digest. Different architectures may use different digests. Starting a new probe makes older evidence ineligible immediately, even if its old deadline has not expired. Only native v1 IR→EX TCP is supported; an inventory report cannot activate FRP, reverse direction or UDP.

## Trust boundary and work still required

This module validates data and admission prerequisites. It does **not** verify job signatures, authenticate a reporting node, verify a binary's signature/revocation or prove process/path health. The existing `internal/agentjob`, `internal/bcc/jobsign.go`, Agent replay journal and authenticated ACK path remain the basis for those checks. Callers must not construct trusted challenge/requirements from the report itself.

Before live enforcement or an Installed/Ready UI:

1. Persist the next per-node probe generation, fresh nonce, expected job and validity window atomically before signing/dispatch. Bind the challenge to node authorization and reject revoked/re-enrolled identities appropriately.
2. Add a genuinely implemented read-only native inventory action to the existing signed allowlist. Observe actual installed files/version/digest without free-form commands, downloads or secret output. Do not add an allowlisted action without an implementation.
3. Authenticate ACKs against the expected node/current job; strictly decode the report, advance/persist the snapshot and reject stale/out-of-order results. Failed/pending probes must not reuse a previous generation for admission.
4. Derive per-node artifact requirements from the verified release manifest, pinned root and revocations. Keep artifact provenance separate from an Agent assertion of presence.
5. Integrate admission into reviewed plan/create with deterministic PlanHash, migration/compatibility behavior, port reservations, process/route health, rollback and persistence tests. Retain current native behavior until that integration is reviewed.
6. Add live two-node tests and truthful freshness/installation UI before enabling external drivers under #229/#230.

The unit/failure/race tests for this contract do not constitute live-node inventory or completed #228 lifecycle acceptance. No new engine, merge, release or production deployment is authorized by this document.
