// Package tunnelengine defines conservative server-side driver capabilities.
// Declaring a planned driver here must never silently activate it.
package tunnelengine

import (
	"fmt"
	"strings"
)

type Capability struct {
	Name       string `json:"name"`
	Deployable bool   `json:"deployable"`
	Reason     string `json:"reason"`
}

// Available is a capability declaration, NOT evidence of agent implementation.
// Keep future drivers explicitly disabled until installed and verified on both nodes.
func Available() []Capability {
	return []Capability{
		{Name: "baft", Deployable: true, Reason: "native BAFT Secure agent lifecycle"},
		{Name: "frp", Deployable: false, Reason: "FRP adapter and two-node admission not implemented"},
		{Name: "rathole", Deployable: false, Reason: "Rathole adapter and two-node admission not implemented"},
		{Name: "wss", Deployable: false, Reason: "WSS adapter and carrier admission not implemented"},
		{Name: "quic", Deployable: false, Reason: "QUIC adapter and carrier admission not implemented"},
	}
}

// RequireDeployable checks the engine at the shared backend plan+create
// boundary. The empty value preserves old BAFT callers while no other
// engine is available. Unknown or planned engines fail closed.
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
