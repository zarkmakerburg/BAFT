package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// NFTLabTransaction joins the isolated runtime, a preprovisioned nft chain,
// and a durable journal. It is not a signed Agent executor. Restart always
// requires independent socket and kernel reconciliation before any adoption.
type NFTLabTransaction struct {
	mu       sync.Mutex
	Runtime  *Runtime
	Firewall *NFTLabBackend
	Journal  MutationJournal
	// Successful socket retirement is process-local evidence, never recovered
	// from journal phases. Keep it only while exact-owner cleanup is unfinished.
	stopped map[ingressmanifest.Proof]stoppedIngressSocket
}

type stoppedIngressSocket struct {
	runtime     *Runtime
	firewall    *NFTLabBackend
	handle      uint64
	chainHandle uint64
	removed     bool
	rollback    bool
	// inode is the kernel identity of the exact socket this process closed.
	// Zero (unknown) can never satisfy the fenced operational nft Remove.
	inode uint64
}

func nftTransactionRecord(spec ingressmanifest.Spec, proof ingressmanifest.Proof) (MutationRecord, RulePlan, error) {
	expected, err := ingressmanifest.Preview(spec)
	if err != nil || expected != proof {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	_, portText, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	plan, err := PlanIngressRule(uint16(port), proof)
	return MutationRecord{Port: uint16(port), Owner: proof, Phase: PhaseIntent}, plan, err
}

// effectLeaseFence is private to this implementation. Callers cannot supply
// a test double, Boolean or authorization callback: the ONLY positive guard
// is Alpha's concrete held *Lease, rechecked before every effect boundary.
type effectLeaseFence struct {
	lease   *ingressauthority.Lease
	command ingressauthority.CommandV1
	// Private per-operation observation: never caller-supplied and never a permit.
	pin ingressinventory.ManagedLABExecutionPin
	// Private seams for in-package tests; the production constructors leave
	// them nil so the concrete lease, installed-policy pin and installed
	// collector are used. No exported API can supply any of them.
	authority func(context.Context) error
	installed func(context.Context) error
	collect   func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error)
}

// socketEvidenceAtEffect takes a FRESH exact numeric-port (v4+v6) kernel
// inventory immediately before one nft effect. An active LISTEN on the port
// is expected (ours, or a foreign one that the caller classifies) so
// ErrClaimConflict is tolerated; any other failure or incomplete coverage
// denies the effect.
func (f *effectLeaseFence) socketEvidenceAtEffect(ctx context.Context) (ingressinventory.CoverageEvidence, error) {
	collect := f.collect
	if collect == nil {
		collect = ingressinventory.CollectInstalledManagedTCP
	}
	observed, err := collect(ctx, f.command.Binding.NodeID, f.command.Binding.Port)
	if err != nil && !errors.Is(err, ingressinventory.ErrClaimConflict) {
		return observed, err
	}
	// Evidence about any other numeric port says nothing about this one.
	if observed.Port != f.command.Binding.Port || !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return observed, ingressinventory.ErrCoverageIncomplete
	}
	return observed, ctx.Err()
}

// checkInstall runs immediately before Firewall.Install: the only LISTEN on
// the numeric port (both families) must be our exact journal+runtime inode.
func (f *effectLeaseFence) checkInstall(ctx context.Context, t *NFTLabTransaction) error {
	if f == nil {
		return nil
	}
	if err := f.check(ctx); err != nil {
		return err
	}
	observed, err := f.socketEvidenceAtEffect(ctx)
	if err != nil {
		return err
	}
	_, err = t.exactOwnedListener(ctx, f.command.Binding, observed, PhaseListenerBound)
	return err
}

// checkRemove runs immediately before Firewall.Remove: the kernel table must
// show the exact socket inode we closed is gone from the port in every state.
// A different, foreign LISTEN is never touched or waived as ours, but does
// not make deleting our own exact-handle accept rule unsafe. Unknown inode
// or an unobserved kernel table denies; absence of a map entry is not proof.
func (f *effectLeaseFence) checkRemove(ctx context.Context, t *NFTLabTransaction, inode uint64) error {
	if f == nil {
		return nil
	}
	if err := f.check(ctx); err != nil {
		return err
	}
	if inode == 0 {
		return ErrUnverified
	}
	observed, err := f.socketEvidenceAtEffect(ctx)
	if err != nil {
		return err
	}
	b := f.command.Binding
	for _, claim := range observed.ActiveTCP {
		if claim.Port != b.Port {
			return ErrUnverified
		}
		if claim.Inode == inode {
			return fmt.Errorf("%w: retired socket inode %d still present in kernel (%s state %s)", ErrUnverified, inode, claim.Family, claim.State)
		}
	}
	for _, claim := range observed.Stopped {
		if claim.Port == b.Port {
			return ErrConflict
		}
	}
	t.Runtime.mu.Lock()
	_, bound := t.Runtime.sessions[b.ListenEndpoint]
	t.Runtime.mu.Unlock()
	if bound {
		return ErrUnverified
	}
	return nil
}

// ownedInode reads the kernel inode of the exact socket this Runtime holds
// for endpoint+proof. It trusts no caller value.
func (t *NFTLabTransaction) ownedInode(endpoint string, proof ingressmanifest.Proof) (uint64, error) {
	r := t.Runtime
	r.mu.Lock()
	s, ok := r.sessions[endpoint]
	if !ok || s.proof != proof || s.listener == nil {
		r.mu.Unlock()
		return 0, ErrUnverified
	}
	tcp, ok := s.listener.(*net.TCPListener)
	if !ok {
		r.mu.Unlock()
		return 0, ErrUnverified
	}
	duplicate, err := tcp.File()
	r.mu.Unlock()
	if err != nil {
		return 0, errors.Join(ErrUnverified, err)
	}
	info, statErr := duplicate.Stat()
	_ = duplicate.Close()
	if statErr != nil {
		return 0, errors.Join(ErrUnverified, statErr)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		return 0, ErrUnverified
	}
	return st.Ino, nil
}

// exactOwnedListener classifies one real kernel socket as ours only when the
// Runtime session, immutable proof, journal record at one of phases, bound
// listener and observed kernel inode ALL agree, and no other numeric-port
// LISTEN (any IP/family) or stopped foreign claim exists.
func (t *NFTLabTransaction) exactOwnedListener(ctx context.Context, b ingressauthority.BindingV1, observed ingressinventory.CoverageEvidence, phases ...MutationPhase) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return 0, ingressinventory.ErrCoverageIncomplete
	}
	_, plan, err := nftTransactionRecord(b.Spec, b.Proof)
	if err != nil || plan.Port != b.Port {
		return 0, ErrUnverified
	}
	record, found, err := t.Journal.Current(b.Proof)
	if err != nil || !found || record.Owner != b.Proof || record.Port != b.Port || !slices.Contains(phases, record.Phase) {
		return 0, errors.Join(ErrUnverified, err)
	}
	if record.Phase != PhaseListenerBound && record.FirewallHandle == 0 {
		return 0, ErrUnverified
	}
	r := t.Runtime
	r.mu.Lock()
	s, ok := r.sessions[b.ListenEndpoint]
	if !ok || s.proof != b.Proof || s.listener == nil || s.forwarder == nil ||
		s.retiring || s.ctx.Err() != nil {
		r.mu.Unlock()
		return 0, ErrUnverified
	}
	tcp, ok := s.listener.(*net.TCPListener)
	if !ok || tcp.Addr().String() != b.ListenEndpoint {
		r.mu.Unlock()
		return 0, ErrUnverified
	}
	// ValidateListener independently inventories the same numeric port and
	// rejects other live owners in both families, not merely same IP.
	if err := s.forwarder.ValidateListener(s.listener); err != nil {
		r.mu.Unlock()
		return 0, errors.Join(ErrConflict, err)
	}
	r.mu.Unlock()
	inode, err := t.ownedInode(b.ListenEndpoint, b.Proof)
	if err != nil {
		return 0, err
	}
	host, _, err := net.SplitHostPort(b.ListenEndpoint)
	if err != nil {
		return 0, ErrUnverified
	}
	expectedFamily := "tcp6"
	if net.ParseIP(host).To4() != nil {
		expectedFamily = "tcp4"
	}
	matches := 0
	for _, claim := range observed.Stopped {
		if claim.Port == b.Port {
			return 0, ErrConflict
		}
	}
	for _, entry := range observed.ActiveTCP {
		if entry.Port != b.Port {
			return 0, ErrUnverified
		}
		if entry.State != "0A" {
			continue
		}
		if entry.Inode == inode && entry.Family == expectedFamily {
			matches++
			continue
		}
		return 0, fmt.Errorf("%w: foreign kernel listener %s inode=%d on shared port %d", ErrConflict, entry.Family, entry.Inode, b.Port)
	}
	if matches != 1 {
		return 0, ErrUnverified
	}
	return inode, ctx.Err()
}

func (f *effectLeaseFence) check(ctx context.Context) error {
	if f != nil && f.authority != nil {
		if err := f.authority(ctx); err != nil {
			return err
		}
		return f.installed(ctx)
	}
	if f == nil {
		// Legacy LAB rollback may use a fresh cleanup context after
		// the original Apply context is cancelled. Preserve existing
		// LAB cancellation/rollback behavior; only live operational
		// leases get the additional per-effect authority fence.
		return nil
	}
	if err := CheckIngressExecutorAuthority(ctx, f.lease, f.command); err != nil {
		return err
	}
	// Reobserve installed profile, exact parsed source file membership/bytes,
	// current executable, cgroup and namespace BEFORE each owned mutation.
	// On drift, do not continue effects or attempt unleased cleanup.
	return ingressinventory.CheckInstalledManagedLABAtEffect(ctx, f.command.Binding.NodeID, f.pin)
}

func (t *NFTLabTransaction) ApplyWithLease(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if lease == nil || cmd.Action != "public_ingress_apply" || cmd.Binding.Spec != spec || cmd.Binding.Proof != proof {
		return ErrExecutorTrustUnavailable
	}
	// Refuse unsigned/unheld operations before even capturing private scope.
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, cmd.Binding.NodeID)
	if err != nil {
		return err
	}
	fence := &effectLeaseFence{lease: lease, command: cmd, pin: pin}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return t.apply(ctx, spec, proof, fence)
}
func (t *NFTLabTransaction) Apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	return t.apply(ctx, spec, proof, nil)
}
func (t *NFTLabTransaction) apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	record, plan, err := nftTransactionRecord(spec, proof)
	if err != nil {
		return err
	}
	endpoint, err := canonicalEndpoint(spec.Config.Listen)
	if err != nil {
		return err
	}
	current, found, err := t.Journal.Current(proof)
	if err != nil {
		return err
	}
	t.Runtime.mu.Lock()
	existing := t.Runtime.sessions[endpoint]
	t.Runtime.mu.Unlock()
	if existing != nil {
		if !found || current.Owner != proof || current.Port != plan.Port || current.Phase != PhaseVerified || current.FirewallHandle == 0 || existing.proof != proof {
			return ErrUnverified
		}
		observed, err := t.Firewall.Inspect(ctx, plan)
		if err != nil || observed.Handle != current.FirewallHandle {
			return errors.Join(ErrFirewallUnverified, err)
		}
		if err := fence.check(ctx); err != nil {
			return err
		}
		return t.Runtime.Apply(ctx, spec, proof)
	}
	if found && (current.Owner != proof || current.Port != plan.Port || current.Phase != PhaseIntent) {
		return ErrUnverified
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Runtime.Apply(ctx, spec, proof); err != nil {
		return err
	}
	record.Phase = PhaseListenerBound
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		if fence != nil {
			// Operational lease might have disappeared while recording.
			// Do not close even our own listener without first checking
			// the original held lock under a fresh valid operation context.
			// Leave the pending claimed state for Alpha reconciliation.
			if leaseErr := fence.check(ctx); leaseErr != nil {
				return errors.Join(err, leaseErr)
			}
		}
		return errors.Join(err, t.Runtime.Retire(endpoint, proof))
	}
	if err := ctx.Err(); err != nil {
		if fence != nil {
			return err
		}
		return errors.Join(err, t.Runtime.Retire(endpoint, proof))
	}
	if err := fence.checkInstall(ctx, t); err != nil {
		// Do not roll back without the held lock or against a changed
		// socket table. Durable recovery retains the exact in-progress
		// operation as uncertain.
		return err
	}
	observed, err := t.Firewall.Install(ctx, plan)
	rollback := func(cause error) error {
		if err := fence.check(ctx); err != nil {
			return errors.Join(cause, err)
		}
		inode, _ := t.ownedInode(endpoint, proof)
		stopErr := t.Runtime.Retire(endpoint, proof)
		if !listenerRetirementVerified(stopErr) {
			return errors.Join(cause, stopErr)
		}
		if observed.Handle == 0 {
			return errors.Join(cause, stopErr)
		}
		if t.stopped == nil {
			t.stopped = make(map[ingressmanifest.Proof]stoppedIngressSocket)
		}
		t.stopped[proof] = stoppedIngressSocket{runtime: t.Runtime, firewall: t.Firewall, handle: observed.Handle, chainHandle: t.Firewall.ExpectedChainHandle, rollback: true, inode: inode}
		// Cleanup must outlive a cancelled apply, but must not block the
		// transaction lock indefinitely if the kernel backend is unresponsive.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(cause, stopErr, t.retireLocked(cleanupCtx, spec, proof, fence))
	}
	if err != nil {
		return rollback(err)
	}
	record.FirewallHandle = observed.Handle
	record.Phase = PhaseFirewallInstalled
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if latest, err := t.Firewall.Inspect(ctx, plan); err != nil || latest.Handle != observed.Handle {
		return rollback(errors.Join(ErrFirewallUnverified, err))
	}
	record.Phase = PhaseVerified
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return nil
}

func (t *NFTLabTransaction) RetireWithLease(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if lease == nil || cmd.Action != "public_ingress_retire" || cmd.Binding.Spec != spec || cmd.Binding.Proof != proof {
		return ErrExecutorTrustUnavailable
	}
	// Refuse unsigned/unheld operations before even capturing private scope.
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, cmd.Binding.NodeID)
	if err != nil {
		return err
	}
	fence := &effectLeaseFence{lease: lease, command: cmd, pin: pin}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return t.retire(ctx, spec, proof, fence)
}
func (t *NFTLabTransaction) Retire(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	return t.retire(ctx, spec, proof, nil)
}
func (t *NFTLabTransaction) retire(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.retireLocked(ctx, spec, proof, fence)
}

// retireLocked shares exact-owner cleanup with failed Apply while t.mu is held.
func (t *NFTLabTransaction) retireLocked(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	record, plan, err := nftTransactionRecord(spec, proof)
	if err != nil {
		return err
	}
	current, found, err := t.Journal.Current(proof)
	if err != nil {
		return err
	}
	stopped, alreadyStopped := t.stopped[proof]
	if !found || current.Owner != proof || current.Port != plan.Port {
		return ErrUnverified
	}
	if alreadyStopped && (stopped.runtime != t.Runtime || stopped.firewall != t.Firewall || stopped.chainHandle != t.Firewall.ExpectedChainHandle) {
		return ErrUnverified
	}
	if alreadyStopped && stopped.rollback && (current.Phase == PhaseListenerBound || current.Phase == PhaseFirewallInstalled) {
		if stopped.handle == 0 || (current.Phase == PhaseListenerBound && current.FirewallHandle != 0) || (current.Phase == PhaseFirewallInstalled && current.FirewallHandle != stopped.handle) {
			return ErrUnverified
		}
		observed, err := t.Firewall.Inspect(ctx, plan)
		if err != nil || observed.Handle != stopped.handle {
			return errors.Join(ErrFirewallUnverified, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		record.Phase, record.FirewallHandle = PhaseRetireIntent, stopped.handle
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := t.Journal.Advance(record); err != nil {
			return err
		}
		current = record
	}
	if current.FirewallHandle == 0 || (current.Phase != PhaseVerified && current.Phase != PhaseRetireIntent && !(current.Phase == PhaseRetired && alreadyStopped && stopped.removed)) || (alreadyStopped && stopped.handle != current.FirewallHandle) {
		return ErrUnverified
	}
	if alreadyStopped && stopped.removed {
		if current.Phase != PhaseRetireIntent && current.Phase != PhaseRetired {
			return ErrUnverified
		}
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := t.Firewall.verifyAbsent(ctx, plan, stopped.handle); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		record.Phase, record.FirewallHandle = PhaseRetired, current.FirewallHandle
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := t.Journal.Advance(record); err != nil {
			return err
		}
		delete(t.stopped, proof)
		return nil
	}
	observed, err := t.Firewall.Inspect(ctx, plan)
	if err != nil {
		return err
	}
	if observed.Handle != current.FirewallHandle {
		return ErrFirewallUnverified
	}
	record.Phase = PhaseRetireIntent
	record.FirewallHandle = current.FirewallHandle
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A terminal transport error must not mask *successful exact socket
	// retirement*. Only a typed verified retirement permits deleting the
	// independently inspected exact-owned nft rule; preserve that cause in
	// the outcome even when firewall/journal cleanup completes.
	var retireErr error
	if !alreadyStopped {
		if err := fence.check(ctx); err != nil {
			return err
		}
		// Capture the exact kernel identity BEFORE closing so the later
		// fenced nft Remove can prove that precise inode left the port.
		inode, _ := t.ownedInode(spec.Config.Listen, proof)
		retireErr = t.Runtime.Retire(spec.Config.Listen, proof)
		if !listenerRetirementVerified(retireErr) {
			return retireErr
		}
		if t.stopped == nil {
			t.stopped = make(map[ingressmanifest.Proof]stoppedIngressSocket)
		}
		stopped = stoppedIngressSocket{runtime: t.Runtime, firewall: t.Firewall, handle: current.FirewallHandle, chainHandle: t.Firewall.ExpectedChainHandle, inode: inode}
		t.stopped[proof] = stopped
	}
	if err := fence.checkRemove(ctx, t, stopped.inode); err != nil {
		return errors.Join(retireErr, err)
	}
	if err := t.Firewall.Remove(ctx, plan, observed.Handle); err != nil {
		return errors.Join(retireErr, err)
	}
	stopped.removed = true
	t.stopped[proof] = stopped
	record.Phase = PhaseRetired
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := t.Journal.Advance(record); err != nil {
		return errors.Join(retireErr, err)
	}
	delete(t.stopped, proof)
	return retireErr
}
