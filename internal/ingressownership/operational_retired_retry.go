package ingressownership

import (
	"context"
	"os"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// completedRetireRetry is a read-only completion path, never an adoption or
// deletion permit. The durable retired journal is necessary but insufficient:
// a concrete live signed lease, the installed execution pin, fresh complete
// socket inventory and exact firewall absence must all agree under the same
// held guard. The caller still posts the command-bound receipt and separately
// performs ReleaseVerified. A spent job cannot mint a lease to enter here.
func (x *OperationalExecutor) completedRetireRetry(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) (RetiredIngressEvidence, bool, error) {
	var zero RetiredIngressEvidence
	if x == nil || x.transaction == nil || x.transaction.Runtime == nil || x.transaction.Firewall == nil ||
		x.transaction.Firewall.Runner == nil || x.transaction.Firewall.ExpectedChainHandle == 0 || x.transaction.Journal == nil {
		return zero, false, ErrExecutorTrustUnavailable
	}
	if cmd.Action != "public_ingress_retire" {
		return zero, false, ingressauthority.ErrBinding
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return zero, false, err
	}
	t := x.transaction
	// Match the normal transaction's lock order: transaction, held guard,
	// then runtime/backend. Close or supersession cannot interleave with the
	// guarded readback, and other transaction effects cannot race this path.
	t.mu.Lock()
	defer t.mu.Unlock()
	b := cmd.Binding
	record, found, err := t.Journal.Current(b.Proof)
	if err != nil {
		return zero, false, err
	}
	if !found || record.Phase != PhaseRetired {
		return zero, false, nil // Existing live/partial-retire path owns these.
	}
	_, plan, err := nftTransactionRecord(b.Spec, b.Proof)
	if err != nil || record.Owner != b.Proof || record.Port != b.Port || record.FirewallHandle == 0 {
		return zero, true, ErrUnverified
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, b.NodeID)
	if err != nil {
		return zero, true, err
	}
	fence := &effectLeaseFence{lease: lease, command: cmd, pin: pin}
	err = fence.atEffect(ctx, fence.retiredSocketEvidence, func() error {
		latest, present, err := t.Journal.Current(b.Proof)
		if err != nil || !present || latest != record {
			return ErrUnverified
		}
		// The backend verifies the pinned chain/layout and refuses any rule
		// carrying this numeric port, owner tag OR the old exact handle.
		if err := t.Firewall.verifyAbsent(ctx, plan, record.FirewallHandle); err != nil {
			return err
		}
		t.Runtime.mu.Lock()
		_, bound := t.Runtime.sessions[b.ListenEndpoint]
		t.Runtime.mu.Unlock()
		if bound {
			return ErrUnverified
		}
		if err := fence.retiredSocketEvidence(ctx); err != nil {
			return err
		}
		latest, present, err = t.Journal.Current(b.Proof)
		if err != nil || !present || latest != record {
			return ErrUnverified
		}
		return nil
	})
	if err != nil {
		return zero, true, err
	}
	return RetiredIngressEvidence{Port: plan.Port, Owner: plan.Owner,
		NFTRuleHandle: record.FirewallHandle, Phase: PhaseRetired, RuleAbsent: true}, true, nil
}

// retiredSocketEvidence uses the same conservative empty-port requirements
// as verified reservation release. A foreign listener is never adopted, closed
// or waived; incomplete inventory and stopped-service claims also deny success.
func (f *effectLeaseFence) retiredSocketEvidence(ctx context.Context) error {
	observed, err := f.socketEvidenceAtEffect(ctx)
	if err != nil {
		return err
	}
	port := f.command.Binding.Port
	for _, claim := range observed.Stopped {
		if claim.Port == port {
			return ErrConflict
		}
	}
	uid := uint32(os.Geteuid())
	for _, row := range observed.ActiveTCP {
		if row.Port != port {
			return ErrUnverified
		}
		if row.State == "0A" {
			return ErrConflict
		}
		if row.Inode != 0 && row.Uid == uid {
			return ErrUnverified
		}
	}
	return ctx.Err()
}
