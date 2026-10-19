# P0 — One-click tunnel creation and automatic public ingress

Tracking: #269; HQ #67. This document is a design checkpoint, not a deployed implementation.

## Existing code integration points
- `internal/bcc/tunnel.go`: signed two-node prepare/commit/health/finalize and rollback lifecycle.
- `internal/bcc/ingress_policy.go`: route-level IR eligibility/selection, not a public TCP port allocator.
- `internal/bcc/smart_ingress.go`: desired/applied endpoint publication, not proof of host-level TCP listener creation.
- `internal/tunnelnode/`: agent-side tunnel lifecycle.
- `web/bcc-app.*`: wizard UI integration.

## Product contract
Basic wizard: select IR node, EX node, EX target TCP port, and public client port. Default public client port = EX 3X-UI inbound port. Subscription domain is NOT a BCC field; 3X-UI manages links and users. Optional Advanced panel exposes dedicated/shared inter-node transport and compatibility details. Never imply changing the 3X-UI custom share address creates an ingress listener.

## Provisioning contract
Preflight IR port occupancy and ownership; allocate/reuse only owned resources. Bind public TCP listener to the specific tunnel's local ingress. Apply firewall change narrowly, persist across reboot, verify actual socket and end-to-end delivery, record desired/applied status, and rollback atomically. Preserve existing 2053 Imperial Crown and all non-owned services. On edit/decommission, preserve sibling tunnels and shared resources until last owner releases them.

## Shared transport gate
Default to TCP/443 only when available and compatible; port alone is not a stealth guarantee. Multiple logical tunnels can share 443 ONLY if native engine proves authenticated stream multiplexing/demultiplexing, per-tunnel routing/isolation, quotas, concurrency and teardown. Otherwise use dedicated listeners and present an actionable explanation. Never bind two independent TCP listeners to the same address:port.

## Required tests
Unit: port conflict, idempotency, ownership, shared refcount, rollback, restart. Integration: 54619 public -> IR local ingress -> BG 127.0.0.1:54619; existing 2053 unaffected; multi-tunnel shared listener; subscription output uses IR address and matching port; FA/EN RTL/LTR mobile. Real-host test is a separate gated activity. No merge/release/deploy without explicit HQ permission.

## Next implementation
1. Map current agent listener/firewall and route provisioning functions and existing tests.
2. Add public-ingress desired-state model and validation to tunnel plan.
3. Implement agent-side owned TCP listener and firewall reconciliation with rollback.
4. Wire simplified wizard and prove CI and end-to-end behavior.

## P0 — Linux installer audit and optimization (added 2026-10-20)

**Scope:** `install.sh` and its called shell helpers, supported Linux distributions, release/offline/source install paths, SSH pairing, BCC-only and agent-only installation, update, downgrade, recovery and uninstall. This is a P0 safety and reliability track alongside Public Ingress, not authorization to change live servers.

**Priority P0-A: preservation and rollback.** Verify every installer path preserves existing BAFT and 3X-UI traffic, especially the legacy BAFT.goldapp.tr:2053 listener. Require an explicit confirmation before replacing an occupied port, service unit, config or firewall rule. Back up config, binaries, units and managed firewall state before mutation. On failure, restore only installer-owned resources and report exact recovery steps; never delete unrelated tunnel state.

**Priority P0-B: supply-chain trust.** Keep signed release verification against the pinned root key, enforce revocations, checksum and version/downgrade rules. Verify offline bundles with the same policy. Treat `BAFT_INSTALL_FROM=source` as a development-only path and do not silently fall back from verified release to unverified source. Ensure downloads have bounded retries/timeouts and atomic staging.

**Priority P0-C: predictable Linux behavior.** Audit root/dependency detection, systemd availability, package manager support, Linux architecture, firewall backend (nftables/iptables/ufw), SELinux/AppArmor and IPv4/IPv6 handling. Validate all ports and bind addresses before service changes. Avoid package installation or global firewall flushing when unnecessary.

**Priority P0-D: UX and observability.** Add a safe preflight / dry-run report covering selected role, target paths, ports, occupied sockets, compatibility, planned actions, backup location and explicit blockers. Print actionable Persian/English messages and stable exit codes; redact tokens, pairing codes and private keys from logs.

**Priority P0-E: reproducible tests.** Run `bash -n`, ShellCheck, shell unit tests and disposable Linux container/VM install-update-rollback scenarios across supported distributions. Include network outage, corrupt artifact, signature mismatch, disk full, service restart failure, rerun/idempotency, noninteractive SSH pairing and preservation of existing tunnel 2053. Run staging integration only after CI; no production deployment without HQ approval.

**Acceptance gates:** audited install matrix; tests and CI green; zero unintended host changes; proven rollback/idempotency; security review of privileged operations; documentation updated; HQ permission before any merge, release or deploy.

## P0 — Installer CLI menus, tables and end-to-end interaction standard (2026-10-20)

Source reviewed: `install.sh` (~3,231 lines). Existing UI helpers include `ui_capable`, `ui_ask_prompt`, `ui_choice`, `ui_cols`, `ui_rule`, `ui_table`, `phase`, `kv`, `confirm_plan`. The current first-run role menu offers EX, IR and Panel. The installer also contains an explicit plan/confirmation flow and pairing-code handoff. These are **audit observations, not tested behavior**.

**P0-M1 — Navigation contract:** Design one consistent numbered menu hierarchy for Install EX / Install IR / Install BCC / Agent only / Existing installation repair or upgrade / Diagnostics / Exit. Do not change the semantics of noninteractive flags. Provide Back, Cancel, Help and confirmation semantics; prevent accidental role changes, and show prerequisites before selecting IR or EX.

**P0-M2 — Status and action vocabulary:** Standardize visible states (Ready, Needs action, Warning, Blocked, Completed), icon meanings, error codes, action labels and headings across all steps. Keep visual emphasis separate from severity. Make FA/EN translations complete rather than mixing fragments unpredictably; preserve machine-readable English output and stable command flags.

**P0-M3 — Tables and terminal width:** Audit `ui_table` for ANSI escape width, Unicode grapheme/display width, right-to-left mixed strings, `|` delimiter injection, long unbroken values, empty cells, `fold` behavior and small terminal widths. Its current width arithmetic uses Bash character counts and clamps minimum terminal width to 64, so 40–63-column terminals can overflow. Provide a plain-text fallback rather than forced box drawing; do not trust unbounded row content. Keep copyable addresses, ports and commands outside decorative tables.

**P0-M4 — Accessibility:** Respect NO_COLOR, TERM=dumb, redirected streams and UTF-8 detection; also test color-blind-friendly severity text, screen readers, SSH/Termux, 40/60/80/120-column displays and narrow mobile terminal views. No information may depend solely on color or icons.

**P0-M5 — Input discipline:** Validate every menu answer with explicit allowed values, retry bounds, EOF handling and noninteractive error paths. Separate normal answers from secrets; never echo pairing codes or admin tokens in diagnostics. Provide safe defaults for optional fields and explicit destructive-change confirmations.

**P0-M6 — Step progress:** Show stage name, stage number, current action, planned changes, what is preserved and next expected user action. Progress is not a percentage unless grounded in real completed work. Distinguish preflight, download/verify, backup, configure, pair, start, health verification and rollback.

**P0-M7 — Plan/Confirm contract:** Keep `--plan`/JSON plan consistent with human-readable tables. Display ports, listeners, ownership, target paths, service changes and rollback scope before applying. Current `confirm_plan` skips confirmation for fresh installs; review this separately for conflicts and privileged changes without introducing unnecessary prompts in unattended installs.

**P0-M8 — Pairing UX:** Present EX-first and IR-second workflow, expiration, SSH host-key verification and recovery steps clearly. Ensure pairing/reply codes remain single-line copyable, masked in logs, never wrapped into table cells, and do not appear in telemetry.

**P0-M9 — Recovery UX:** For all errors, state what succeeded, what failed, whether rollback ran, the backup path (if safe), what remains active and a concrete next command. Never print “installed successfully” before service health is verified. Preserve existing BAFT.goldapp.tr:2053 and all non-owned resources.

**P0-M10 — Golden tests and acceptance:** Add shell snapshot/golden tests for terminal capabilities and locales, menu choice/invalid/EOF, width/wrapping, output streams, plan JSON parity, secret redaction, pairing handoff and interrupted install. Require `bash -n`, ShellCheck, noninteractive CI and disposable Debian/Ubuntu VM tests. Document before/after examples and only land behavior changes after evidence. No production changes or HQ-gated merge/release/deploy.

**Sequencing:** (1) snapshot current menus/tables/output, (2) define CLI design tokens and stable output schema, (3) refactor pure presentation helpers, (4) validate input and pairing UX, (5) test install/upgrade/rollback scenarios, (6) stage and obtain HQ approval. Do not redesign the installer by replacing working safety gates.
