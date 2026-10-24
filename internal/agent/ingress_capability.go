package agent

import (
	"context"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// CapabilityHeader advertises the operational ingress v1 path to BCC.
const (
	CapabilityHeader        = "X-BAFT-Agent-Capabilities"
	CapabilityOperationalV1 = "operational-ingress-v1"
)

// capabilityHeaders advertises operational-ingress-v1 only when the real path
// is configured AND this host has a loadable trusted policy. Otherwise no
// header is sent and BCC keeps its enqueue gate closed. The advertisement is
// informational: execution still re-pins the policy under the held guard.
func (a *Agent) capabilityHeaders(ctx context.Context) map[string]string {
	op := a.cfg.IngressOperational
	if op == nil || op.Store == nil || op.Executor == nil {
		return nil
	}
	check := a.policyProvisioned
	if check == nil {
		check = func(ctx context.Context, nodeID string) error {
			_, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, nodeID)
			return err
		}
	}
	if check(ctx, a.cfg.NodeID) != nil {
		return nil
	}
	return map[string]string{CapabilityHeader: CapabilityOperationalV1}
}
