# iMonitor ideas integrated into the BAFT roadmap

Owner requested independent use of useful iMonitor ideas on 2026-10-14.
Reference reviewed: `iranmonitor/imonitor@5f39fe2c9da144dc83e5c5bb666665836fbcc4e6`.
BAFT starting point: `codex/smart-tunnel-p0@d8bb3b69decfba2ba57430bcfe61eab64a6ba3cc`, draft #200, epic #199.
This is a requirements mapping and source review, not a runtime comparison.
No upstream code, test protocol, transport or dependency is imported.

| Priority | Useful behavior | BAFT integration and acceptance |
| --- | --- | --- |
| P0 | Separate connection acceptance from delivery | Doctor v2 and signed data/seal jobs already exist in #200. Missing, stale, different-instance or different-generation evidence stays NOT_ASSESSED. Do not implement a parallel health engine. |
| P0 | Inventory interfaces, addresses, routes and sockets | Extend authenticated agent preflight on BOTH nodes using existing discovery jobs. Record observation time, namespace, completeness, bind address/protocol/port, conflicting service and ownership. Missing or truncated inventory cannot establish availability. Recheck/reserve at commit; an earlier snapshot alone cannot prevent a competing bind. |
| P0 | Explain resource ownership | Prepare candidate certificates separately; verify identity and complete file trees before cutover and cleanup. Show impact on the selected instance and retain unrelated tunnel resources. This slice implements staged EX PKI, not pre-cutover payload verification. |
| P1 | Bounded upload/download measurement | Extend #145/#146/#147 with an explicitly selected tunnel/route and generation. Proposed initial lab budget: 10s per direction, 64 MiB TOTAL across both directions, one active test per instance/node pair, cancellation and deadline. These are proposed safe ceilings, not accepted performance defaults. |
| P1 | Receiver-confirmed throughput | Store sender attempted bytes separately from receiver-confirmed bytes, elapsed time, direction, route identity, node identities, config digest, completion/error and measurement method. No direct-path substitute. TCP retransmissions are not measured UDP loss. |
| P1 | Guided Persian UI | Reuse #194 enrollment/offline IP-country and current builder. Explicit Add vs Edit/Replace, basic vs expert, effective-config preview, impact/rollback summary and actionable unknown/error labels. Preserve manual location choice; geographic location is not node identity. |
| P1 | Explain observable traffic | Describe outer TLS, inner Noise, authentication, exposed endpoints/metadata and observed transport family. No inferred browser/YouTube fingerprint or blocking percentage. Reuse #74 field evidence. |
| P1 | Resource and check history | Reuse BCC health/telemetry storage; show source timestamp and gaps per node and tunnel. An external probe is a separate viewpoint, not proof of IR-to-EX delivery. |
| P2 | Reliable notifications/public status | Reuse incident and alert state. Separate incident creation from recipient delivery; durable outbox, bounded retry/backoff, idempotent receipt and sanitized optional public output. |

## Verified upstream reference points

- [speedtest.rs](https://github.com/iranmonitor/imonitor/blob/5f39fe2c9da144dc83e5c5bb666665836fbcc4e6/src/speedtest.rs): explicit duration/stream/byte bounds, token challenge and receiver result. Its 32 GiB byte ceiling is not adopted as a BAFT default.
- [runner.rs](https://github.com/iranmonitor/imonitor/blob/5f39fe2c9da144dc83e5c5bb666665836fbcc4e6/src/runner.rs): distinguish what each probe actually observes; runtime fingerprint/resource ownership ideas.
- [app.rs](https://github.com/iranmonitor/imonitor/blob/5f39fe2c9da144dc83e5c5bb666665836fbcc4e6/src/app.rs): external-region reports and alert bookkeeping. Mapping every unsuccessful external check to a blocked region, or setting notified before delivery, is not adopted.
- [workspace.js](https://github.com/iranmonitor/imonitor/blob/5f39fe2c9da144dc83e5c5bb666665836fbcc4e6/src/ui/workspace.js): guided form/help as product inspiration, implemented independently in BCC.

The supplied Persian analysis also describes inconsistent public open-source
announcements and repository licensing. This slice needs no copied code and
makes no new legal determination. SSH host identity checks, signed agent jobs,
BAFT backup consistency and existing health hysteresis remain required.

## Execution order and remaining gates

1. Staged PKI/config preparation and safe rollback in #200; actual payload still verified after commit and before backup destruction. A separate isolated pre-cutover probe route is not implemented.
2. Physical-host inventory, ownership and reservation; #197 installer/shared-binary coexistence. Do not upgrade a binary used by sibling services without explicit compatibility evidence.
3. Enrollment/Doctor UX on the existing backend, followed by bounded measurement.
4. Lab profile comparison, history, notification delivery and public status.

No production deploy, main merge, tag/release, new transport/shaping, EX pool,
automatic exit replacement or process-restart resume is authorized by this document.
New staged transactions must be completed/rolled back with the same new binary;
old-binary downgrade during an in-flight staged transaction is not supported or tested.
