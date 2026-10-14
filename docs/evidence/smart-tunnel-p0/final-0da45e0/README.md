# Node-local resource preflight — exact published source evidence

Recorded 2026-10-14. PR #200 remains draft; #199 remains open.

## Source and publication

- Continued from `59cc9dfe719a535914b58e358d2760938fad2f37`.
- Resource commit: `4f9b8578ab33e8b5c552c2740b10dad5685f647b`, tree `0cc225806a2140d606ea7a0cfab28ce57142e8b4`.
- Final source: `0da45e07d71c5f7a84f29bc07c9f98d644745f4b`, tree `e407bf1ff919e8b8bfd7e646b62f83f9617308fd`, parent `4f9b8578ab33e8b5c552c2740b10dad5685f647b`.
- Local resource and Unix-fix commits b29a1e5 and 9297517 have those same respective trees. Connector-created commits were fetched again and tested on clean detached checkouts. CLI publication lacks authentication; GitHub git-object APIs published with exact expected_sha and force=false. Existing history was preserved.
- GitHub PR synthetic merge `4f0fe9f45c8a5d7995239cee95461f1c260d87e9` was fetched and has the SAME tree as final source. The predecessor synthetic merge `66eb1b049ab01dbce56340b5e9f7ffe4f67379f2` likewise matches 4f9.
- This archive is a separate documentation-only child of final source. It does not move the reviewed PR head.

## Behavior and limits

Both BCC-signed prepare jobs now explicitly request verify_resources=true. Both agents inspect real TCP4/TCP6 listener tables and selected instance config/unit/marker/generation/receipt ownership before creating preparation artifacts; commit repeats the checks before changing active artifacts. Missing, malformed, truncated or inaccessible evidence refuses the change. Reuse needs the matching installed configuration, stable systemd MainPID/process identity/network namespace and actual FD socket inode. Unknown Unix files/sockets remain intact. Unix accepted clients are separated from the stream listener so an open management connection does not falsely invalidate the owner.

The baseline actual occupied-port regression failed: preparation returned a pairing code instead of rejecting the collision. The fixed test checks refusal before key, stage, claim, service or generation mutation. Tests also cover a late bind before commit without consuming the pairing secret, safe retry, unmanaged/corrupt/symlink artifacts, IPv4/IPv6 wildcard overlap, changed owner and live socket inventory.

This is bounded passive inspection, not an atomic cross-process reservation. General interfaces/routes, capacity, OS permission/UID/GID audit beyond existing PKI, shared installer resources and mixed-version/shared-binary coexistence remain open. Adapter fixtures explicitly supply modeled listener inventory because they pre-bind synthetic proof servers; they are not host acceptance. The source adds no production listener/probe, service/firewall mutation during resource inspection, transport/shaping, automatic restart recovery, EX-pool switching or standby resumption.

## Final exact-SHA local checks

Go 1.27.1, linux/amd64, clean source checkout at 0da45e0.

| Command | Result | Log |
| --- | --- | --- |
| go test ./... -count=1 -timeout=10m | PASS; includes explicit environment skips below | logs/199-resource-0da-full.log |
| go test -race ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout=10m | PASS (35.951s / 46.708s / 1.065s); same local limitations | logs/199-resource-0da-race.log |
| go test ./internal/tunnelnode -run '^(TestResource\|TestProcTCP\|TestPassiveInventory\|TestTwoLiveInstances)' -count=1 -timeout=5m -v | PASS for executed tests; three SKIPs | logs/199-resource-0da-focused.log |
| go test -race ./internal/bcc -run '^(TestScopedTunnel\|TestPreflight\|TestTunnel)' -count=1 -timeout=5m -v | PASS, 3.540s; both signed prepare flags checked | logs/199-resource-0da-bcc-race.log |
| go vet ./internal/tunnelnode ./internal/agent ./internal/agentjob ./internal/bcc | PASS, exit 0, no output | logs/199-resource-0da-vet.log |
| scripts/check_ci_supply_chain.sh | PASS, 13 workflows | logs/199-resource-0da-policy.log |

The runtime cannot inspect another process's /proc network namespace/FD ownership and refuses AF_UNIX socket creation. Three focused tests explicitly SKIP locally:
- TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle
- TestResourcePreflightDoesNotUnlinkAnUnrelatedUnixSocket
- TestResourceUnixListenerEvidenceWithActiveManagementClient

They are NOT counted as local real-process/Unix passes. Existing PKI uid/gid mutation also has its previously documented environment limitation. Production ownership validation is not disabled. A controlled long-lived child tests the environment before the sibling fixture; BAFT_REQUIRE_RESOURCE_EVIDENCE=1 turns these resource prerequisite skips into failures.

## Native Linux CI evidence

[CI 38527050350](https://github.com/zarkmakerburg/BAFT/actions/runs/38527050350) is associated with final source 0da45e0; fetched synthetic checkout tree matches exactly.

Main test job 115729769307:
- Unit and integration tests: SUCCESS.
- Node-local tunnel resource acceptance: SUCCESS. It runs the exact focused regex THREE TIMES with BAFT_REQUIRE_RESOURCE_EVIDENCE=1. This includes the two-real-process H2/TLS/Noise sibling lifecycle, unknown Unix socket preservation and real held-management-client regression, with environment skips forbidden.
- This confirms the existing Germany echo flow/PID/generation/artifacts while Bulgaria is prepared/activated/address-port-CA edited/rolled back/failed/retired. Local process fixture service control is simulated; it is not itself real-systemd evidence.
- Aggregate CI is still running its race step. R3 subsequently completed SUCCESS; final-status.json records that newer observation while the earlier ci-status.json is retained.
- BCC UI 38527050336 and static-analysis 38527050497: SUCCESS.
- Real-systemd signed-release launch job 115729769458: completed SUCCESS, including build/break/rollback acceptance on a throwaway GitHub runner. This is current-source evidence, not a production deployment.

Predecessor 4f9 test job 115726121168 completed SUCCESS, including full race/vet/fuzz and required three-run native resource acceptance. The excerpt in native-4f9-resource.log contains the actual required-step output; no SKIPs. Predecessor systemd launch job 115726121158 also completed SUCCESS. These are historical and do not replace final-source evidence; final source additionally fixes the accepted-Unix-client distinction.

## Attempts retained without relabeling

All available resource-slice test logs are in logs/ and included in manifest-sha256.json. Preliminary uncommitted development trees are not claimed as final-SHA acceptance.

| Attempt group | Observed result |
| --- | --- |
| before | Expected baseline FAIL: actual occupied listener was accepted |
| initial | Earlier focused sanity PASS, before required resource fixture |
| first / owner-debug / owner-debug2 | FAIL while enabling real-process resource checks; child process namespace access unavailable |
| expanded1 | Executed cases PASS, explicit child-process SKIP |
| expanded2 | FAIL from AF_UNIX EPERM; subsequently guarded as environment capability with mandatory CI failure |
| focused-final / adapters / full / race / vet / ci-policy / bcc-race | Development-tree executed checks PASS; local native restrictions explicitly recorded |
| b29-focused | Local source-tree check PASS for executed cases with explicit SKIPs |
| 4f9-* | Fetched exact predecessor source full/race/focused/BCC/vet/policy PASS, local skips unchanged |
| unix-client / unix-final | Additional Unix parser and held-client fixture checks; local native SKIPs retained |
| 0da-* | Final fetched exact source checks above |
| 0da-bcc-race-minimal | Initial narrow selection PASS, expanded afterward to include scoped/preflight cases |
| 0da-policy-command-error | Incorrect policy-script path, exit 2; corrected command PASS without overwriting failed attempt |

## Remaining work

PKI is staged before cutover, but full candidate configuration completes in CommitEX after the IR reply and actual scoped payload proof occurs AFTER commit, BEFORE backup release. Coordinated pre-cutover candidate handshake/payload, atomic resource reservation and broader fault/field acceptance remain P0.

Installer draft #197 was reread at unchanged fb1950b295442d891aa6a09481d030e49c507b24. Its independent service/path names and shared binary/release/baft-net ownership need coordinated coexistence validation; none is claimed here. Follow with guided SSH/default-IP-country UX, continuous fresh Doctor observations and the explanatory/performance requirements in #199.

No production/server changes, main merge, tag/release or disallowed role resumption. Board #67 claim 6162552502 covers this slice and is released in the final board report.

