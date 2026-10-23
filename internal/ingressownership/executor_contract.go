package ingressownership

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// Delta deliberately does not define a second Binding, Command, Permit or
// Receipt schema. Alpha owns the single shared ingressauthority v1 contract.
// The earlier draft (41d7642) is retained in git history, not wired to a host.
var ErrExecutorTrustUnavailable = errors.New("trusted ingress executor lease is unavailable")

// CheckIngressExecutorAuthority only accepts Alpha's CONCRETE opaque lease
// and a signed-job-shaped CommandV1. No caller-implementable interfaces,
// boolean admission, JSON minting or read-only preflight can bypass the
// authority. The caller must keep the same lock held until durable commit.
// Without Alpha's real durable mint/check implementation this fails CLOSED.
func CheckIngressExecutorAuthority(ctx context.Context, lease *ingressauthority.Lease, command ingressauthority.CommandV1) error {
	if ctx == nil || lease == nil {
		return ErrExecutorTrustUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := command.Validate(time.Now()); err != nil {
		return err
	}
	return lease.CheckLockedIngress(ctx, command)
}

// ObservedIngressV1 is ONLY independently capturable host evidence. It is
// NOT an operational receipt, lease, or authority to mutate. Alpha converts
// independently verified host evidence into a signed-command-bound outcome.
type ObservedIngressV1 struct {
	Listener ingressauthority.ListenerEvidenceV1
	Firewall ingressauthority.FirewallEvidenceV1
	Chain    uint64
}

// ValidateObservedIngressV1 checks coherence of INERT evidence supplied by
// an observer; a caller can fabricate these values, so a successful result
// neither proves host ownership nor authorizes a successful receipt.
func ValidateObservedIngressV1(binding ingressauthority.BindingV1, observed ObservedIngressV1) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	ip, _, err := net.SplitHostPort(binding.ListenEndpoint)
	if err != nil {
		return ErrInvalid
	}
	family := "tcp6"
	if net.ParseIP(ip).To4() != nil {
		family = "tcp4"
	}
	if observed.Listener.Endpoint != binding.ListenEndpoint ||
		observed.Listener.Family != family ||
		observed.Listener.SocketIdentity == "" ||
		observed.Chain == 0 || observed.Firewall.ChainHandle != observed.Chain {
		return ErrUnverified
	}
	plan, err := PlanIngressRule(binding.Port, binding.Proof)
	if err != nil {
		return err
	}
	return VerifyRuleObservation(plan, FirewallObservation{
		Port: plan.Port, Tag: observed.Firewall.OwnerTag,
		Handle: observed.Firewall.RuleHandle,
		Family: observed.Firewall.Family,
		Table:  observed.Firewall.Table,
		Chain:  observed.Firewall.Chain,
	})
}
