package ingressownership

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// NFTRunner is injected for isolated tests. The production runner executes
// only a read-only nft command, never a firewall mutation.
type NFTRunner interface {
	ReadRuleset(context.Context) ([]byte, error)
}

type NFTCommandReader struct{ Binary string }

func (r NFTCommandReader) ReadRuleset(ctx context.Context) ([]byte, error) {
	if r.Binary != "/usr/sbin/nft" && r.Binary != "/sbin/nft" {
		return nil, ErrFirewallUnverified
	}
	cmd := exec.CommandContext(ctx, r.Binary, "-j", "-a", "list", "ruleset")
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Join(ErrFirewallUnverified, err)
	}
	return output, nil
}

// InspectNFTIngress requires fresh, complete read-only evidence. A timeout,
// invalid snapshot or competing rule always fails closed.
func InspectNFTIngress(ctx context.Context, runner NFTRunner, plan RulePlan) (FirewallObservation, error) {
	if runner == nil {
		return FirewallObservation{}, ErrFirewallUnverified
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := runner.ReadRuleset(ctx)
	if err != nil {
		return FirewallObservation{}, errors.Join(ErrFirewallUnverified, err)
	}
	observations, err := ParseNFTIngressRules(raw)
	if err != nil {
		return FirewallObservation{}, err
	}
	return ReconcileRuleObservation(plan, observations)
}
