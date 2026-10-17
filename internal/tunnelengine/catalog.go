// Package tunnelengine defines conservative server-side driver capabilities.
// The catalog is not evidence of installation or health on either node.
package tunnelengine

import (
	"fmt"
	"strings"
)

// Capability describes a candidate tunnel driver or carrier.
// Deployable is true only when BCC can plan and orchestrate that driver with
// the current native agent. Final node/path safety gates are still required.
type Capability struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Family       string   `json:"family"`    // direct, reverse, or carrier
	Direction    string   `json:"direction"` // ir_to_ex, ex_to_ir, or configurable
	Deployable   bool     `json:"deployable"`
	Description  string   `json:"description"`
	Reason       string   `json:"reason"`
	Requirements []string `json:"requirements"`
}

// Available is a server-owned, fail-closed capability declaration. Do not
// activate planned drivers by changing this list: the agent adapter, signed
// packaging, isolated systemd lifecycle and verified dual-node E2E must land
// first. Carriers such as WSS/QUIC are not standalone tunnel engines.
func Available() []Capability {
	return []Capability{
		{Name: "baft", Label: "BAFT Secure", Family: "direct", Direction: "ir_to_ex", Deployable: true, Description: "Native BAFT Secure tunnel between Iran and foreign nodes, with reviewed plan, TLS/Noise and isolated instance lifecycle.", Reason: "native BAFT Secure agent lifecycle"},
		{Name: "frp", Label: "FRP", Family: "reverse", Direction: "ex_to_ir", Description: "Planned independent reverse TCP/UDP proxy driver.", Reason: "FRP adapter and two-node admission not implemented", Requirements: []string{"signed pinned binary", "agent lifecycle + rollback", "two-node ingress + data E2E"}},
		{Name: "rathole", Label: "Rathole", Family: "reverse", Direction: "ex_to_ir", Description: "Planned authenticated reverse-proxy driver with independent control and data connections.", Reason: "Rathole adapter and two-node admission not implemented", Requirements: []string{"verified binary and license", "agent lifecycle + rollback", "two-node ingress + data E2E"}},
		{Name: "backhaul", Label: "Backhaul", Family: "reverse", Direction: "ex_to_ir", Description: "Planned reverse tunnel driver with per-instance service and metrics.", Reason: "Backhaul adapter, artifact provenance and node admission not implemented", Requirements: []string{"signed pinned binary", "node compatibility", "two-node isolation E2E"}},
		{Name: "wireguard", Label: "WireGuard", Family: "direct", Direction: "configurable", Description: "Planned separate layer-3 VPN driver, not an Xray inbound or BAFT transport.", Reason: "WireGuard orchestration and route/firewall safety gates not implemented", Requirements: []string{"kernel/userspace compatibility", "route rollback", "two-node data E2E"}},
		{Name: "ssh-socks", Label: "SSH SOCKS", Family: "direct", Direction: "ir_to_ex", Description: "Planned authenticated SSH dynamic SOCKS egress, not a TCP port-forward by itself.", Reason: "SSH dynamic SOCKS tunnel agent lifecycle not implemented", Requirements: []string{"scoped identity", "bounded process lifecycle", "secure credential handling"}},
		{Name: "wss", Label: "WSS", Family: "carrier", Direction: "configurable", Description: "Planned TLS WebSocket carrier; not an independent tunnel engine.", Reason: "WSS carrier adapter and admission not implemented", Requirements: []string{"TLS identity", "carrier compatibility E2E"}},
		{Name: "quic", Label: "QUIC", Family: "carrier", Direction: "configurable", Description: "Planned QUIC/UDP carrier; not an independent tunnel engine.", Reason: "QUIC carrier adapter and admission not implemented", Requirements: []string{"UDP reachability", "carrier compatibility E2E"}},
	}
}

// RequireDeployable checks the shared plan/create boundary. Empty is the
// legacy alias of BAFT only. Every unavailable or unknown driver fails closed.
func RequireDeployable(raw string) (string, error) {
	engine := strings.TrimSpace(raw)
	if engine == "" || engine == "baft" {
		return "baft", nil
	}
	for _, capability := range Available() {
		if capability.Name == engine {
			return "", fmt.Errorf("tunnel engine %q is unavailable: %s", engine, capability.Reason)
		}
	}
	return "", fmt.Errorf("unsupported tunnel engine %q", engine)
}
