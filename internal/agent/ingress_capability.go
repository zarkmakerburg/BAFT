package agent

import (
	"context"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// CapabilityHeader advertises the operational ingress v1 path to BCC.
const (
	CapabilityHeader        = "X-BAFT-Agent-Capabilities"
	CapabilityOperationalV1 = "operational-ingress-v1"
)

// capabilityHeaders lists, in one header, operational-ingress-v1 (only when
// the real path is configured AND this host has a loadable trusted policy;
// otherwise BCC keeps its enqueue gate closed) and the transport capabilities
// of the installed baft binary. With nothing to declare no header is sent.
// The advertisement is informational: execution still re-pins the policy
// under the held guard, and BCC only uses transport tokens to refuse plans.
func (a *Agent) capabilityHeaders(ctx context.Context) map[string]string {
	var tokens []string
	if a.operationalIngressProvisioned(ctx) {
		tokens = append(tokens, CapabilityOperationalV1)
	}
	tokens = append(tokens, a.transportCapabilities(ctx)...)
	if len(tokens) == 0 {
		return nil
	}
	return map[string]string{CapabilityHeader: strings.Join(tokens, ",")}
}

func (a *Agent) operationalIngressProvisioned(ctx context.Context) bool {
	op := a.cfg.IngressOperational
	if op == nil || op.Store == nil || op.Executor == nil {
		return false
	}
	check := a.policyProvisioned
	if check == nil {
		check = func(ctx context.Context, nodeID string) error {
			_, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, nodeID)
			return err
		}
	}
	return check(ctx, a.cfg.NodeID) == nil
}
