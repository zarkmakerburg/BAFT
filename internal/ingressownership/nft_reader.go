package ingressownership

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	// Cap output during execution, not only when parsing, so a compromised
	// nft process cannot force unbounded allocations before validation.
	var output bytes.Buffer
	cmd.Stdout = &limitedNFTWriter{dst: &output, remaining: 4 << 20}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, errors.Join(ErrFirewallUnverified, err)
	}
	return output.Bytes(), nil
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

// limitedNFTWriter refuses oversized snapshots without retaining extra bytes.
type limitedNFTWriter struct {
	dst       *bytes.Buffer
	remaining int
}

func (w *limitedNFTWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, ErrFirewallUnverified
	}
	n, err := w.dst.Write(p)
	w.remaining -= n
	return n, err
}
