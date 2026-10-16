# Stage254 — monitoring freshness source qualified; draft only

[Draft PR221](https://github.com/zarkmakerburg/BAFT/pull/221), source
`14cfc6a42186ece49393f5401b69ffa8046771c3`, parent
`ae27cae89e7d88b7e6f75396105fe3c3b904b97f`, tree
`86b8f2cc9a07e3ee3df6ddc628e7c5b7135ca2dd`.

All three workflows completed SUCCESS on attempt1:

| Workflow | Complete result |
| --- | --- |
| [CI38948263357](https://github.com/zarkmakerburg/BAFT/actions/runs/38948263357) | 15/15 jobs SUCCESS; whole-tree race, vet and protocol fuzz PASS; disposable install/SSH/offline/BCC/schema/uninstall/fault/provenance/upgrade/rollback/Launch1 PASS |
| [R3.138948263416](https://github.com/zarkmakerburg/BAFT/actions/runs/38948263416) | Whole-tree correctness and race, vet, statistical/ECRL differential, record fuzz, installer/build/artifact PASS |
| [GoSec38948263379](https://github.com/zarkmakerburg/BAFT/actions/runs/38948263379) | SUCCESS, zero reported issues under the existing configured policy; not a blanket vulnerability audit |

No retry/cancellation by this worker. Main's conditional failure-report step
was normally skipped because correctness passed; mandatory race/vet/fuzz all
completed SUCCESS. R3 BCC package75.891s correctness/426.884s race; main BCC
race431.488s. Full logs, not excerpts, are stored under logs/: all15 main jobs,
R3 and GoSec, **17 logs / 3775868 characters**. Log manifest records Git blob
identities and lengths. Earlier15 completed logs were independently fetched,
rehash-checked with no truncation; their SHA256s are also retained.

Actions actually checked out synthetic PR merge
`5bc5a4ac79c13f878425f8042be634c41fa6a72b`, parents ae27cae/14cfc6a.
GitHub Git API and independent Git fetch prove its complete tree identical to
source14cfc6a (whole-repository diff exit0). Run metadata head_sha is14cfc6a.
These are source-content tests on that exact tree, not a literal checkout of
14cfc6a and not a mainline merge. See synthetic-commit.json and
tested-merge-tree.txt. No green transfers to release binaries or other branches.

Six paths change from live reviewed main: MonitoringSnapshot now requires
current dated probe/agent/telemetry evidence, preserves independent node-online
reachability, leaves expired routes UNKNOWN, and Overview rejects invalid RTT
and future current rates/agent freshness. The legacy node-online summary is
not tunnel/process/data-path health. Existing schema/vocabulary/handlers stay
compatible. The fixed-time regression has22 evidence cases with three duration
inputs, two real httptest/auth+Overview scenarios and a future aggregate-rate
assertion. Baseline and intermediate failures preceded the corresponding fixes.
initial-regression_test.go.txt and regression-strengthening.diff preserve the
actual older test and the later stronger assertions; do not claim the initial
baseline used the final test unchanged.

**Unknowns and limits retained:** The initial Mac baseline/focused/intermediate
raw logs could not be recovered after connection loss. Their observed results
and commands are in attempts.md, not a fabricated transcript. Final staged Mac
full BCC -race result remains UNKNOWN. Linux full-source correctness/race is
independent evidence, not a reinterpretation of that Mac run. Human review not
submitted; draft remains unmerged. Browser/rendered/keyboard/accessibility/
performance NOT RUN. Native IR↔EX owner-DEFERRED/OPEN. Layered-health sampler,
alerts, historical charts/uptime and per-route observation timestamps need
separate review; this patch does not resolve all health views.

No Aurora replacement/mount, dependency, auth/session/CSRF/write/persistence,
alert/runtime/transport/shaping/pool/restart/standby/installer/signer change.
No main merge, tag, real signature, release, deploy or operational node change
by this worker. HQ's v0.3.2 signing lane remains independent; this draft is not
automatically promoted into its selected candidate. Rollback for this isolated
work is simply to keep the current production/main selection unchanged. If HQ
later selects it for integration, review current target HEAD and rerun affected
gates before promotion. #199 stays open; bounded source delivery does not close
the broader project. Initial Stage253 evidence and all source histories remain
in ancestry, with lease/expected_sha and force:false publication.
