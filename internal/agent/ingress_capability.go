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
	// CapabilityOperationalHostV1 is the ADR-003 safe host mode on an
	// ordinary server: a different contract from the isolated LAB, so BCC
	// can require the per-port operator acknowledgement before queuing.
	CapabilityOperationalHostV1 = "operational-ingress-host-v1"
	// CapabilityPathProbeSpeedV1 says this agent can run and serve the
	// time-bounded BAFTS1 throughput flow of path_probe_run (speed_seconds).
	CapabilityPathProbeSpeedV1 = "path-probe-speed-v1"
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
		if a.cfg.IngressOperational.Profile == ingressinventory.ExecutionProfileHostV1 {
			tokens = append(tokens, CapabilityOperationalHostV1)
		} else {
			tokens = append(tokens, CapabilityOperationalV1)
		}
	}
	tokens = append(tokens, a.transportCapabilities(ctx)...)
	// Agent-native feature: true of this agent build whatever binary is installed.
	tokens = append(tokens, CapabilityPathProbeSpeedV1)
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
