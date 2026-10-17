# Independent Arange-tun Panel UX Study for BAFT

Reference: github.com/MAHDI-byte64/Arange-tun, reviewed SHA f108493, files internal/webui/assets/dashboard.html, internal/webui/handlers_tunnels.go, internal/manage/tunnel.go; AGPL-3.0. Reuse product concepts only; do not copy AGPL code, HTML/CSS or assets.

Code-verified pattern: grouped Reverse/Direct type chooser → engine-specific introduction → server/client role form → control and forwarded ports → shared CLI/web lifecycle with per-tunnel service and health. The Arange reverse topology commonly has IR listening for user traffic and EX dialing IR. The current native BAFT topology instead has IR dialing the BAFT listener on EX. BAFT must show this difference before construction.

BAFT Stage265 implements truthful connection-direction selection and visual path (IR dialer → EX listener → EX destination service); reverse disabled until independently implemented. Stage263 adds explicit user-selected/generated EX destination service port and manual follow-up destination service endpoint guidance, not automatic IR public ingress. #228 capability driver contract precedes #229 FRP and #230 Rathole. Multiport mapping and real managed IR public ingress are future work and must not be advertised as completed. Preserve BAFT's signed-agent orchestration, reviewed PlanHash, isolated siblings and transactional rollback rather than adopting build-on-server installers. Claims about DPI/fingerprint must be measured, not copied from marketing text.

Acceptance: driver/topology fail closed, per-engine truthful capabilities, role/port differentiation, regression coverage, two-node E2E before production exposure. No direct code reuse without explicit AGPL licensing compliance review.
