package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// OperationalExecutor is a real (but currently unreachable) guarded path
// into the existing exact-owner runtime and nft transaction. The ONLY permit
// is Alpha's concrete, live, unforgeable *ingressauthority.Lease; this package
// cannot mint one. An Alpha-supplied MutationJournal is mandatory and no
// read-only inventory/boolean/job JSON can bypass CheckLockedIngress.
//
// A successful Apply is never returned without the owned listener FD + nft
// handle readback while the same Alpha live lease remains valid. The caller
// must hold the lease and commit a durable Agent outcome before releasing it.
type OperationalExecutor struct {
	transaction *NFTLabTransaction
}

func NewOperationalExecutor(runtime *Runtime, nft *NFTLabBackend, journal MutationJournal) (*OperationalExecutor, error) {
	if runtime == nil || nft == nil || nft.Runner == nil || nft.ExpectedChainHandle == 0 || journal == nil {
		return nil, ErrExecutorTrustUnavailable
	}
	return &OperationalExecutor{transaction: &NFTLabTransaction{
		Runtime: runtime, Firewall: nft, Journal: journal,
	}}, nil
}

// ownedRetireListener classifies one real kernel socket as ours only when
// the local Runtime session, immutable proof, verified operation journal,
// exact bound listener and observed kernel inode ALL agree. The caller
// cannot provide a trusted inode or a Boolean permission. Every other
// numeric-port LISTEN (even another IP/family) remains foreign.
func (x *OperationalExecutor) ownedRetireListener(ctx context.Context, b ingressauthority.BindingV1, observed ingressinventory.CoverageEvidence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Proving a single exact own inode never erases unknown or partially
	// inspected services, source trees or a concurrent namespace occupant.
	// Only the actual protected collector can populate this observation.
	if !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return ingressinventory.ErrCoverageIncomplete
	}
	_, plan, err := nftTransactionRecord(b.Spec, b.Proof)
	if err != nil || plan.Port != b.Port {
		return ErrUnverified
	}
	record, found, err := x.transaction.Journal.Current(b.Proof)
	if err != nil || !found || record.Owner != b.Proof || record.Port != b.Port ||
		record.Phase != PhaseVerified || record.FirewallHandle == 0 {
		return errors.Join(ErrUnverified, err)
	}
	r := x.transaction.Runtime
	r.mu.Lock()
	s, ok := r.sessions[b.ListenEndpoint]
	if !ok || s.proof != b.Proof || s.listener == nil || s.forwarder == nil ||
		s.retiring || s.ctx.Err() != nil {
		r.mu.Unlock()
		return ErrUnverified
	}
	tcp, ok := s.listener.(*net.TCPListener)
	if !ok || tcp.Addr().String() != b.ListenEndpoint {
		r.mu.Unlock()
		return ErrUnverified
	}
	// ValidateListener independently inventories the same numeric port and
	// rejects other live owners in both families, not merely same IP.
	if err := s.forwarder.ValidateListener(s.listener); err != nil {
		r.mu.Unlock()
		return errors.Join(ErrConflict, err)
	}
	duplicate, err := tcp.File()
	r.mu.Unlock()
	if err != nil {
		return errors.Join(ErrUnverified, err)
	}
	info, statErr := duplicate.Stat()
	_ = duplicate.Close()
	if statErr != nil {
		return errors.Join(ErrUnverified, statErr)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		return ErrUnverified
	}
	host, _, err := net.SplitHostPort(b.ListenEndpoint)
	if err != nil {
		return ErrUnverified
	}
	expectedFamily := "tcp6"
	if net.ParseIP(host).To4() != nil {
		expectedFamily = "tcp4"
	}
	matches := 0
	for _, claim := range observed.Stopped {
		if claim.Port == b.Port {
			return ErrConflict
		}
	}
	for _, entry := range observed.ActiveTCP {
		if entry.Port != b.Port {
			return ErrUnverified
		}
		if entry.State != "0A" {
			continue
		}
		if entry.Inode == st.Ino && entry.Family == expectedFamily {
			matches++
			continue
		}
		return fmt.Errorf("%w: foreign kernel listener %s inode=%d on shared port %d", ErrConflict, entry.Family, entry.Inode, b.Port)
	}
	if matches != 1 {
		return ErrUnverified
	}
	return ctx.Err()
}

func (x *OperationalExecutor) preflightRetire(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if x == nil || x.transaction == nil || x.transaction.Runtime == nil || x.transaction.Firewall == nil || x.transaction.Journal == nil {
		return ErrExecutorTrustUnavailable
	}
	if cmd.Action != "public_ingress_retire" {
		return ingressauthority.ErrBinding
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	observed, scanErr := ingressinventory.CollectInstalledManagedTCP(ctx, cmd.Binding.NodeID, cmd.Binding.Port)
	// A normal collector observes our original bound listener as an active
	// LISTEN conflict. This is not authorization to ignore the numeric port.
	// Independently prove the exact journal+runtime+kernel identity first.
	if err := x.ownedRetireListener(ctx, cmd.Binding, observed); err != nil {
		if scanErr != nil && !errors.Is(scanErr, ingressinventory.ErrClaimConflict) {
			return errors.Join(err, scanErr)
		}
		return err
	}
	if scanErr != nil && !errors.Is(scanErr, ingressinventory.ErrClaimConflict) {
		return scanErr
	}
	if !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return ingressinventory.ErrCoverageIncomplete
	}
	return CheckIngressExecutorAuthority(ctx, lease, cmd)
}

func (x *OperationalExecutor) preflight(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease, action string) error {
	if x == nil || x.transaction == nil || x.transaction.Runtime == nil || x.transaction.Firewall == nil || x.transaction.Journal == nil {
		return ErrExecutorTrustUnavailable
	}
	if cmd.Action != action {
		return ingressauthority.ErrBinding
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	// The sole source of managed-node roots is the private installed policy
	// at the fixed path. No caller can pass a source list or a completeness
	// assertion. A matching foreign process or any unsupported stopped-service
	// class DENIES host changes. Unknown claims are not deleted/restarted.
	_, err := ingressinventory.CollectInstalledManagedTCP(ctx, cmd.Binding.NodeID, cmd.Binding.Port)
	if err != nil {
		return err
	}
	// A complete profile can only be a preflight observation: Alpha's held
	// authority must independently recheck its immutable binding/lock.
	return CheckIngressExecutorAuthority(ctx, lease, cmd)
}

// Apply will only bind the exact listener and install a narrow owner-tagged
// nft rule in a provisioned, pinned chain AFTER a real held Alpha lease and
// complete installed-policy source inventory are independently verified.
// Alpha's currently published *Lease cannot mint or authorize anything, so
// the code correctly returns ErrUnverified without side effects in v1 today.
func (x *OperationalExecutor) Apply(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) (ObservedIngressV1, error) {
	if err := x.preflight(ctx, cmd, lease, "public_ingress_apply"); err != nil {
		return ObservedIngressV1{}, err
	}
	if err := x.transaction.ApplyWithLease(ctx, cmd.Binding.Spec, cmd.Binding.Proof, cmd, lease); err != nil {
		return ObservedIngressV1{}, err
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		// With a lost operation lock, do NOT mutate further or claim
		// successful cleanup. Preserve Alpha's durable claim as uncertain.
		return ObservedIngressV1{}, errors.Join(ErrUnverified, err)
	}
	evidence, err := x.inspectExactOwned(ctx, cmd.Binding)
	if err != nil {
		return ObservedIngressV1{}, err
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return ObservedIngressV1{}, errors.Join(ErrUnverified, err)
	}
	return evidence, nil
}

// RetiredIngressEvidence is a readback of the exact resource retired, not
// a signed receipt or a transferable clearance for the same port.
type RetiredIngressEvidence struct {
	Port          uint16
	Owner         ingressmanifest.Proof
	NFTRuleHandle uint64
	Phase         MutationPhase
}

func (x *OperationalExecutor) Retire(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) (RetiredIngressEvidence, error) {
	if err := x.preflightRetire(ctx, cmd, lease); err != nil {
		return RetiredIngressEvidence{}, err
	}
	_, plan, err := nftTransactionRecord(cmd.Binding.Spec, cmd.Binding.Proof)
	if err != nil {
		return RetiredIngressEvidence{}, err
	}
	before, found, err := x.transaction.Journal.Current(cmd.Binding.Proof)
	if err != nil || !found || before.Phase != PhaseVerified || before.Port != plan.Port || before.FirewallHandle == 0 {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, err)
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return RetiredIngressEvidence{}, err
	}
	retireErr := x.transaction.RetireWithLease(ctx, cmd.Binding.Spec, cmd.Binding.Proof, cmd, lease)
	if !listenerRetirementVerified(retireErr) {
		return RetiredIngressEvidence{}, retireErr
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, retireErr, err)
	}
	after, found, err := x.transaction.Journal.Current(cmd.Binding.Proof)
	if err != nil || !found || after.Phase != PhaseRetired || after.Owner != before.Owner || after.FirewallHandle != before.FirewallHandle {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, retireErr, err)
	}
	if err := x.transaction.Firewall.verifyAbsent(ctx, plan, before.FirewallHandle); err != nil {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, retireErr, err)
	}
	x.transaction.Runtime.mu.Lock()
	_, stillBound := x.transaction.Runtime.sessions[cmd.Binding.ListenEndpoint]
	x.transaction.Runtime.mu.Unlock()
	if stillBound {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, retireErr)
	}
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return RetiredIngressEvidence{}, errors.Join(ErrUnverified, retireErr, err)
	}
	return RetiredIngressEvidence{Port: plan.Port, Owner: plan.Owner, NFTRuleHandle: before.FirewallHandle, Phase: after.Phase}, retireErr
}

// inspectExactOwned provides an independent owner-inode and nft handle readback,
// not a data-only assumption from a prior mutation command's success.
func (x *OperationalExecutor) inspectExactOwned(ctx context.Context, b ingressauthority.BindingV1) (ObservedIngressV1, error) {
	r := x.transaction.Runtime
	r.mu.Lock()
	s, ok := r.sessions[b.ListenEndpoint]
	if !ok || s.proof != b.Proof || s.listener == nil || s.forwarder == nil || s.ctx.Err() != nil {
		r.mu.Unlock()
		return ObservedIngressV1{}, ErrUnverified
	}
	ln := s.listener
	if err := s.forwarder.ValidateListener(ln); err != nil {
		r.mu.Unlock()
		return ObservedIngressV1{}, errors.Join(ErrUnverified, err)
	}
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		r.mu.Unlock()
		return ObservedIngressV1{}, ErrUnverified
	}
	duplicate, err := tcp.File()
	r.mu.Unlock()
	if err != nil {
		return ObservedIngressV1{}, errors.Join(ErrUnverified, err)
	}
	info, statErr := duplicate.Stat()
	_ = duplicate.Close()
	if statErr != nil {
		return ObservedIngressV1{}, errors.Join(ErrUnverified, statErr)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		return ObservedIngressV1{}, ErrUnverified
	}
	plan, err := PlanIngressRule(b.Port, b.Proof)
	if err != nil {
		return ObservedIngressV1{}, err
	}
	rule, err := x.transaction.Firewall.Inspect(ctx, plan)
	if err != nil {
		return ObservedIngressV1{}, errors.Join(ErrUnverified, err)
	}
	nftHandle := x.transaction.Firewall.ExpectedChainHandle
	family := "tcp6"
	host, _, err := net.SplitHostPort(b.ListenEndpoint)
	if err != nil {
		return ObservedIngressV1{}, err
	}
	if net.ParseIP(host).To4() != nil {
		family = "tcp4"
	}
	observation := ObservedIngressV1{
		Listener: ingressauthority.ListenerEvidenceV1{
			Endpoint: b.ListenEndpoint, Family: family, SocketIdentity: "inode:" + strconv.FormatUint(st.Ino, 10),
		},
		Firewall: ingressauthority.FirewallEvidenceV1{
			Family: rule.Family, Table: rule.Table, Chain: rule.Chain,
			ChainHandle: nftHandle, RuleHandle: rule.Handle, OwnerTag: rule.Tag,
		},
		Chain: nftHandle,
	}
	if err := ValidateObservedIngressV1(b, observation); err != nil {
		return ObservedIngressV1{}, fmt.Errorf("observed ingress does not match canonical owner: %w", err)
	}
	return observation, nil
}
