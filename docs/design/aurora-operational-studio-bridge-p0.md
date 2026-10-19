# Aurora Floating Glass → operational Tunnel Studio (P0)

## Verified existing operational flow
The current operational BCC wizard is embedded in `internal/bcc/dashboard.go`, not `web/bcc-app.js`. Its stage selector is `tnWizardStep(i)` (five stages), validation is `tnWizardValidateStep(i)`, planning is `tnPlan()` via POST `/api/tunnels/plan`, and deployment is `tnDeploy()` via POST `/api/tunnels`. It preserves CSRF through `ah()`, plan hash, conflict/replacement confirmations, and legacy-coexistence checks.

## Isolation
`web/bcc-action-wizard.js` is a *separate preview-only* component for the modern web BCC. It does not call `tnPlan()`, `tnDeploy()`, or any HTTP write. Do not expose a deploy button here before the real operational backend has been integrated and tested. Never replace the classic wizard with a duplicate set of unsafe actions.

## Mappings and gaps
- Aurora `direct` maps to classic `ir-to-ex`.
- Aurora `reverse` maps to classic `ex-to-ir`, currently disabled for native BAFT.
- Operational engine identifier is `baft`, not `native`.
- Current classic `tnClientPort` is *externally managed*, not an actual public ingress listener. The Aurora public port preview must **not** imply ingress deployment.
- Preview node text is not authoritative: use BCC node discovery and server-side role/ownership validation before permitting live actions.
- Shared EX TCP port 443 needs real authenticated multiplexing before allowing concurrent native tunnels.
- Keep live legacy 2053 and existing IR/BG tunnel untouched.

## Required integration gates
1. Discover installed engine capabilities from authoritative BCC endpoint, not a hardcoded list.
2. Bind discovered IR/EX node IDs, not arbitrary aliases.
3. Show full resolved topology and all ports before plan.
4. Use existing `/api/tunnels/plan` and plan hash semantics with authenticated session and CSRF.
5. Recheck plan hash and conflict ownership at commit time.
6. Preserve replace/coexistence warnings and explicit confirmation.
7. Surface job progress, persisted operation state, rollback and final verification.
8. Test both language directions, mobile focus management, and all failure paths.
9. Run CI and two-node staging verification before HQ review.
10. No merge, release, deploy, or live host mutation without HQ approval.
