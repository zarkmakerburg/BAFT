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

## P0 — BCC TLS/SSL certificate provisioning and renewal (2026-10-20)

**Mandatory P0 acceptance gate:** Installing BCC must provide a trustworthy HTTPS certificate workflow, validate the result before declaring installation healthy, and preserve an explicit safe recovery path. This applies to initial install, existing BCC upgrades, domain/IP changes, certificate replacement and renewal. No live host changes without HQ approval.

**Existing implementation audit:** `install.sh` already has `bcc_install_acme`, `bcc_cert_valid`, `bcc_setup_ssl` and a `baft-bcc-cert.timer` for ACME renewal. It requests Let's Encrypt via `acme.sh` HTTP-01 on port 80, supports domain or short-lived IP certificates where ACME profile support exists, and may offer self-signed fallback. **This P0 item is to audit, harden and verify the existing feature, not falsely claim SSL is absent.**

**P0-S1 — Preflight:** Validate DNS A/AAAA or public IP reachability, chosen panel hostname, port 80 HTTP-01 availability, port 443/selected BCC HTTPS listener conflicts, firewall ownership, ACME rate limits and existing reverse proxy. Do not disrupt an unrelated port-80 service to obtain a certificate. Show operator options (existing certificate, DNS challenge where supported, manual certificate, or explicit safe retry).

**P0-S2 — Trust:** Verify certificate SAN against domain/IP, expiration, chain and key pair; never treat self-signed as production-trusted. Explicitly warn about insecure fallback and block automatic SSH node enrollment when trust is inadequate. Preserve root-owned key permissions and avoid leaking keys/tokens in output or logs.

**P0-S3 — Supply chain:** Pin and verify the ACME client artifact and version, use bounded downloads and atomic extraction, prohibit unsafe unverified auto-upgrade, and document supported ACME CA profiles for IP certificates. Preserve existing release verification policy.

**P0-S4 — Renewal:** Test systemd timer enablement, ACME challenge reuse, certificate rotation with atomic writes, service reload/restart and post-renew TLS handshake. Alert on renewal failure and expiry; retain the last known-good certificate and never leave BCC without working HTTPS.

**P0-S5 — UX:** Add an explicit `BCC HTTPS / SSL` step to the standard installer journey, with domain/IP selection, existing-certificate option, challenge preflight, issuance progress, failure explanation and recovery commands. FA/EN, noninteractive and Termux/SSH output must stay consistent; never promise a trusted IP certificate unless issued and verified.

**P0-S6 — Tests:** Cover valid domain, wrong DNS, blocked port 80, occupied port 80, IP-profile unsupported, ACME failure, revoked/expired certificate, self-signed fallback, renewal, key mismatch, interrupted install, rollback and repeated installation. Run disposable Debian/Ubuntu VM integration and CI tests; do not claim production readiness before evidence.

**Exit criteria:** successful verified TLS handshake to the BCC endpoint; certificate matches the configured address; automated renewal proven; secrets protected; all failures preserve previous BCC and unrelated services; HQ approval before merge/release/deploy.

## P0 — Adaptive server network performance and connection reliability (2026-10-20)

**Goal:** The BAFT Linux installer must provide safe, measurable and reversible network optimization for IR/EX nodes and BCC hosts, without promising one universally best sysctl profile. Prioritize real user latency, throughput, connection establishment, packet loss and tunnel stability, not synthetic speed alone.

**Existing implementation:** `install.sh` generates `baft-net` with BBR/fq, socket buffer tuning, TCP MTU probing, speed/latency tests and an enable/disable menu. The existing default tuning is not automatically optimal for every kernel, provider, route or tunnel. Review before replacing any settings.

**P0-N1 — Baseline and discovery:** Before changes, collect kernel/version, NIC/link speed, virtualization/container restrictions, qdisc, congestion control, TCP buffer limits, MTU/PMTU, IPv4/IPv6, DNS latency, packet loss, RTT/jitter, TCP connect and TLS handshake times, retransmits and BAFT-specific tunnel health. Never export private endpoint identifiers or credentials without consent.

**P0-N2 — Safe profiles:** Offer Automatic (recommended), Conservative/stability, Balanced, Throughput, Low-latency and Custom. Detect BBR availability and actual bottlenecks; use CUBIC or provider-safe defaults when BBR is unsupported or worse in measured tests. Avoid indiscriminate large buffers, disabling security protections, or setting unsupported sysctls. Distinguish host TCP optimization from transport protocol/application-layer tuning.

**P0-N3 — Connection robustness:** Audit BAFT transport keepalive, dial/connect timeouts, bounded retry with jitter, exponential backoff, reconnection and route health checks, DNS fallback, MTU/MSS handling, socket/resource limits, and simultaneous-connection behavior. Any changes must preserve tunnel ownership and not interfere with 3X-UI or existing listeners.

**P0-N4 — Ownership and rollback:** Snapshot each affected sysctl/qdisc/MTU/limit and existing config, identify conflicting files in `/etc/sysctl.d`, and write only BAFT-owned settings. On disable/rollback restore the exact prior effective values when safe, rather than blindly forcing `cubic`/`fq_codel`. Test network reachability after applying, revert automatically if regression thresholds are breached, and preserve SSH access.

**P0-N5 — Installer UX:** Add a dedicated `Network Optimization` step after preflight, with before/after measurements, chosen profile, expected tradeoffs, reversible actions and explicit opt-in for host-wide changes. The operator can skip tuning. Integrate with `baft-net` diagnostics and BCC telemetry, with FA/EN text, noninteractive flags and no fabricated speed improvements.

**P0-N6 — Benchmark and acceptance:** Use repeatable multi-run IR↔EX and client↔IR measurements under idle and load, packet loss, jitter, reconnect time and throughput. Include poor routes, BBR absent, restricted containers, IPv6, high-latency links, MTU blackholes, competing sysctl policies and 3X-UI coexistence. Record baseline and confidence intervals; ship a profile only when measured benefits outweigh regressions. CI, disposable VM tests and staged real-host validation required.

**Release gate:** Existing BAFT.goldapp.tr:2053 and all active services remain intact. No global network mutation, merge, release or deployment without explicit HQ authorization.

## P0 — Complete keyboard-operated Linux management console (2026-10-20)

**User requirement / release blocker:** Every supported BAFT text command and operational feature must be discoverable under its appropriate submenu in a Linux terminal management console. Keyboard navigation must support **Tab / Shift+Tab** to move focus and **Enter** to activate; arrows and typed numeric shortcuts may be supported in addition, never as the only navigation. Include Back, Home, Cancel, Help and Exit. Preserve existing CLI flags and noninteractive automation compatibility.

**Current audit:** `install.sh` generates `baft-panel` (status/link/reset/restart/logs/ssl/renew/update/net) and `baft-net` (status/on/off/speed/latency/route/port). Both currently use `read -r` numeric prompts and a one-shot menu, not focusable Tab/Enter controls. This requirement is **not yet implemented**. Some BAFT features live in BCC web UI or the agent and must not be represented as functioning Linux commands until a real, permission-checked backend exists.

**P0-T1 — Capability inventory:** Build a machine-readable command/action catalog from the actual `baft`, `baft-pair`, `baft-bcc`, `baft-agent`, `baft-panel`, `baft-net` and installer help plus documented supported operations. Map every action to an owning subsystem, role (IR/EX/BCC), privileges, side effects, backend availability and menu path. Include install, upgrade/downgrade, backup/restore, pairing, tunnels, ingress, nodes, telemetry, incidents, security/SSL, network diagnostics and uninstall **only where implemented**; mark unavailable items clearly, not as working buttons.

**P0-T2 — Hierarchy:** Home > Installation & Repair; Server & Service; Node Enrollment & Pairing; Tunnels & Public Ingress; Network Optimization & Diagnostics; BCC & Access; TLS Certificates; Backup/Restore & Versions; Logs/Telemetry/Incidents; Security & Advanced; Help/Exit. Each submenu needs breadcrumb, concise description and consistent order; role-inapplicable actions must be disabled with reason.

**P0-T3 — Input engine:** Implement an actual terminal widget/navigation engine (e.g. vetted `dialog`/compatible TUI library or well-tested pure terminal handling), supporting Tab, Shift+Tab, Enter, arrows, Escape and accessibility fallback. Do not confuse Bash Tab completion with keyboard focus. Provide safe text-only numeric/CLI fallback for TERM=dumb, no TTY, SSH and restricted Termux. Test raw terminal restoration on SIGINT, EOF, failure and resize.

**P0-T4 — Action execution:** All selectable actions must dispatch to the same validated, authenticated and authorization-checked backend as their CLI/BCC equivalents. No shell `eval`, command injection or string-interpolated secrets. Show a preflight plan and explicit confirmation for mutating/destructive operations; maintain RBAC, ownership, idempotency and rollback. Read-only operations should remain available without unnecessary prompts.

**P0-T5 — UX:** FA/EN locale, Unicode-aware width, RTL/LTR mixed text, responsive 40/60/80/120-column layout, color and no-color modes, searchable actions, keyboard shortcut hints, clear disabled state, progress, success/failure details and copyable command alternatives. Display current node role, BAFT version, connection health, BCC status and tunnel counts only from verified live data.

**P0-T6 — Tests:** Catalog completeness test comparing documented CLI operations with menu coverage; keyboard integration via PTY for Tab/Shift+Tab/Enter/Escape; command dispatch unit tests; privileged-operation confirmation; screen-reader/plain output; missing dependencies, narrow terminals, resize, network loss and interrupted operations. Verify no regressions in existing scripts, services, tunnels or 3X-UI.

**Implementation order:** Inventory and schema → reusable TUI core and fallback → migrate `baft-panel` and `baft-net` → integrate installer and supported BAFT/agent commands → permissions and rollback → PTY/VM integration tests → HQ acceptance. No unsupported functionality is to be silently exposed as available, and no merge/release/deploy without HQ permission.

### P0-T7 — Dual keyboard selection: Tab/Enter **and** numbered shortcuts (2026-10-20)

Every selectable menu and submenu entry MUST show a stable visible number and support both (a) Tab / Shift+Tab focus navigation followed by Enter, and (b) typing the displayed number followed by Enter. Both input methods dispatch to the **same** validated action with the same permission checks, confirmations and result handling. Arrow-key navigation is an optional additional method, never a replacement for either required method.

Use 0 consistently for Back/Exit, with explicit context and no ambiguous hidden shortcuts. For menus with more than nine items, accept multi-digit choices as a complete input (e.g. 10, 11) without premature dispatch on the first digit. Reject invalid, unavailable and role-incompatible selections clearly and without executing a different action. Preserve the current numeric-only fallback on TERM=dumb, redirected I/O and terminals without interactive widget support; CLI subcommands remain scriptable. Add PTY regression tests for Tab, Shift+Tab, Enter, digits, multi-digit choices, invalid values, disabled items, Escape and EOF. Acceptance requires parity between keyboard focus and numbered selection on every menu level.

## BAFT v0.4.0 — next planned release / P0 acceptance (2026-10-20)

**HQ release designation:** The next planned BAFT release is **v0.4.0** (not v0.3.x). This is a **target**, not a tag or published release. Do not create a tag, merge, release, deploy, or modify operational hosts without explicit HQ approval.

**Mandatory release candidate gates:** P0 installer compatibility and rollback; verified BCC HTTPS certificate and renewal; complete Linux management command inventory with numbered shortcuts and Tab/Shift+Tab + Enter; safe network optimization with measurable outcomes; public ingress preflight, ownership, agent application/reconciliation and conflict checks; Aurora/BCC additive integration (not replacement); preserving 3X-UI subscription ownership, existing tunnels and BAFT.goldapp.tr:2053; security and supply-chain verification; FA/EN and RTL/LTR; automated tests and staged real-host evidence. Do not claim unimplemented P0 items are complete.

**Sequence:** Implement and test P0 work on feature branch → obtain passing CI, independent security review and controlled VM evidence → document remaining risks and staged real-host test plan → request HQ approval → only then merge, tag `v0.4.0`, publish release artifacts and deploy. Release notes must distinguish shipped from deferred items and link evidence.
