//go:build unix

package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// Post-restart reconciliation of operational public ingress.
//
// After the Agent process dies, every socket it owned is closed by the kernel:
// the listener is gone by construction. The Runtime map of the new process is
// empty, which is NOT proof of that, so nothing here relies on it. The proof
// is taken from the kernel and the firewall, all of it fresh, all of it
// required:
//
//  1. authority: a held PendingGuard whose lifecycle records a CONSUMED signed
//     lease (ingressauthority.RestartAuthority). A bare journal entry, an
//     unleased intent or a spent/superseded job authorizes nothing.
//  2. sockets: the protected installed-policy collector reports COMPLETE
//     coverage (no unknown source, no stopped foreign service claim) and the
//     numeric-port /proc/net/tcp + tcp6 rows, in every state, contain
//     - no LISTEN at all (this process owns none; any listener is foreign or
//     a still-living older agent: it is reported and never touched), and
//     - no socket with a nonzero inode owned by the agent's uid, in any
//     state (a leaked or inherited socket would invalidate "gone by
//     construction").
//     Inode-less rows (TIME_WAIT and other kernel-owned remnants) and
//     foreign-owned non-listening rows are only reported.
//  3. firewall: the pinned `inet baft ingress` chain (handle and base-chain
//     layout) is read back and holds EITHER exactly the owner-tagged rule for
//     this binding, whose handle equals the journaled handle (a rule that was
//     installed but not yet journaled is accepted only by its exact owner tag,
//     port and layout), OR no rule carrying this port or tag. A foreign rule
//     on the port, a duplicate tag or a handle that differs from the journal
//     denies.
//
// Only then is the exact owned rule deleted BY HANDLE and the durable journal
// advanced to retired. The reconciler can neither bind a socket nor add a
// rule, never touches any other rule, table or socket, never mints or clears a
// lease and never releases the reservation. Everything is idempotent: a crash
// at any step is finished by the next run from the same evidence.

// ErrRestartFenced means evidence was missing, incomplete or contradictory.
// Nothing was changed after the check that failed.
var ErrRestartFenced = errors.New("ingress restart reconciliation fenced")

type RestartOutcome string

const (
	// OutcomeRolledBack: an interrupted Apply was undone (rule removed or
	// proven absent, journal retired).
	OutcomeRolledBack RestartOutcome = "rolled_back"
	// OutcomeRetired: an interrupted Retire was completed.
	OutcomeRetired RestartOutcome = "retired"
	// OutcomeAlreadyRetired: the journal already said retired; the rule was
	// verified absent and nothing was written.
	OutcomeAlreadyRetired RestartOutcome = "already_retired"
	// OutcomeNoEffects: no journal record exists and no rule or socket does.
	OutcomeNoEffects RestartOutcome = "no_effects"
	// OutcomeWouldReconcile: dry run; all evidence held, nothing was written.
	OutcomeWouldReconcile RestartOutcome = "would_reconcile"
	OutcomeFenced         RestartOutcome = "fenced"
)

// RestartSocket is one kernel row on the numeric port that the decision saw.
type RestartSocket struct {
	Family string
	State  string
	Inode  uint64
	Uid    uint32
	// Class: "foreign-listener", "agent-owned", "kernel-remnant" or "foreign".
	Class string
}

// RestartReport is the exact evidence behind one decision.
type RestartReport struct {
	Port         uint16
	Owner        ingressmanifest.Proof
	Direction    ingressauthority.RestartDirection
	JobID        string
	Outcome      RestartOutcome
	JournalPhase MutationPhase
	RuleHandle   uint64
	RulePresent  bool
	Reason       string
	Sockets      []RestartSocket
}

// RestartReconciler holds the injected host evidence sources. Production
// leaves Collect nil (the protected installed-policy collector).
type RestartReconciler struct {
	Journal  MutationJournal
	Firewall *NFTLabBackend
	// Collect returns the numeric-port socket inventory. Nil means
	// ingressinventory.CollectInstalledManagedTCP.
	Collect func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error)
	// AgentUID is the uid the agent's sockets carry in /proc/net/tcp.
	// Zero is a valid uid (root); UIDSet distinguishes "use os.Geteuid()".
	AgentUID uint32
	UIDSet   bool
	// DryRun evaluates every evidence rule and writes nothing.
	DryRun bool
	// Mu, when set, serializes with the executor's transaction.
	Mu *sync.Mutex
}

// RestartReconciler returns the reconciler bound to this executor's durable
// journal and pinned nft backend. It shares the transaction lock.
func (x *OperationalExecutor) RestartReconciler() *RestartReconciler {
	if x == nil || x.transaction == nil {
		return nil
	}
	return &RestartReconciler{Journal: x.transaction.Journal, Firewall: x.transaction.Firewall, Mu: &x.transaction.mu}
}

func (r *RestartReconciler) agentUID() uint32 {
	if r.UIDSet {
		return r.AgentUID
	}
	return uint32(os.Geteuid())
}

func (r *RestartReconciler) collect(ctx context.Context, b ingressauthority.BindingV1) (ingressinventory.CoverageEvidence, error) {
	collect := r.Collect
	if collect == nil {
		collect = ingressinventory.CollectInstalledManagedTCP
	}
	return collect(ctx, b.NodeID, b.Port)
}

func fence(rep RestartReport, format string, args ...any) (RestartReport, error) {
	rep.Outcome = OutcomeFenced
	rep.Reason = fmt.Sprintf(format, args...)
	return rep, fmt.Errorf("%w: %s", ErrRestartFenced, rep.Reason)
}

// socketEvidence applies rule 2 to one fresh inventory.
func (r *RestartReconciler) socketEvidence(ctx context.Context, b ingressauthority.BindingV1) ([]RestartSocket, error) {
	obs, err := r.collect(ctx, b)
	if err != nil && !errors.Is(err, ingressinventory.ErrClaimConflict) {
		return nil, fmt.Errorf("socket inventory: %w", err)
	}
	if !obs.CoverageComplete || len(obs.UnknownSources) != 0 {
		return nil, fmt.Errorf("socket inventory: %w", ingressinventory.ErrCoverageIncomplete)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, claim := range obs.Stopped {
		if claim.Port == b.Port {
			return nil, fmt.Errorf("%w: stopped service claim on port %d", ErrConflict, b.Port)
		}
	}
	uid := r.agentUID()
	var rows []RestartSocket
	var failure error
	for _, row := range obs.ActiveTCP {
		if row.Port != b.Port {
			return rows, fmt.Errorf("%w: kernel row for port %d in an inventory of port %d", ErrUnverified, row.Port, b.Port)
		}
		s := RestartSocket{Family: row.Family, State: row.State, Inode: row.Inode, Uid: row.Uid}
		switch {
		case row.State == "0A":
			s.Class = "foreign-listener"
			if failure == nil {
				failure = fmt.Errorf("%w: a %s listener (inode %d, uid %d) holds port %d; it is not adopted and not touched", ErrConflict, row.Family, row.Inode, row.Uid, b.Port)
			}
		case row.Inode != 0 && row.Uid == uid:
			s.Class = "agent-owned"
			if failure == nil {
				failure = fmt.Errorf("%w: a %s socket of the agent's uid %d (inode %d, state %s) still exists on port %d", ErrUnverified, row.Family, row.Uid, row.Inode, row.State, b.Port)
			}
		case row.Inode == 0:
			s.Class = "kernel-remnant"
		default:
			s.Class = "foreign"
		}
		rows = append(rows, s)
	}
	return rows, failure
}

func (r *RestartReconciler) lock() func() {
	if r.Mu == nil {
		return func() {}
	}
	r.Mu.Lock()
	return r.Mu.Unlock
}

// restartAuth is the private view of *ingressauthority.RestartAuthority. The
// exported entry point accepts only the concrete type; only in-package tests
// can reach reconcile with a stand-in.
type restartAuth interface {
	Binding() ingressauthority.BindingV1
	Direction() ingressauthority.RestartDirection
	Command() ingressauthority.CommandV1
	Check(context.Context) error
}

// Reconcile decides and, unless DryRun, performs the reconciliation of one
// binding. The returned report always carries the evidence that was seen; a
// non-nil error is an ErrRestartFenced wrap (or a cancelled context) and means
// nothing past the failed check was changed.
func (r *RestartReconciler) Reconcile(ctx context.Context, a *ingressauthority.RestartAuthority) (RestartReport, error) {
	if a == nil {
		return fence(RestartReport{}, "%v", ErrExecutorTrustUnavailable)
	}
	return r.reconcile(ctx, a)
}

func (r *RestartReconciler) reconcile(ctx context.Context, a restartAuth) (RestartReport, error) {
	var rep RestartReport
	if r == nil || r.Journal == nil || r.Firewall == nil || r.Firewall.Runner == nil || r.Firewall.ExpectedChainHandle == 0 || a == nil || ctx == nil {
		return fence(rep, "%v", ErrExecutorTrustUnavailable)
	}
	defer r.lock()()
	b := a.Binding()
	rep.Port, rep.Owner, rep.Direction, rep.JobID = b.Port, b.Proof, a.Direction(), a.Command().JobID
	if err := ctx.Err(); err != nil {
		return rep, err
	}
	if err := a.Check(ctx); err != nil {
		return fence(rep, "authority: %v", err)
	}
	_, plan, err := nftTransactionRecord(b.Spec, b.Proof)
	if err != nil || plan.Port != b.Port {
		return fence(rep, "binding does not yield a valid rule plan")
	}
	rec, found, err := r.Journal.Current(b.Proof)
	if err != nil {
		return fence(rep, "journal: %v", err)
	}
	if found {
		if rec.Owner != b.Proof || rec.Port != b.Port {
			return fence(rep, "journal record does not belong to this binding")
		}
		rep.JournalPhase = rec.Phase
	}

	// Rule 3, first read: nothing is decided from the journal alone.
	rule, present, err := r.Firewall.ObserveOwner(ctx, plan)
	if err != nil {
		return fence(rep, "firewall: %v", err)
	}
	rep.RulePresent, rep.RuleHandle = present, rule.Handle
	if !found {
		if present {
			return fence(rep, "an owner-tagged rule exists (handle %d) but no journal record explains it", rule.Handle)
		}
	} else if err := checkJournalAgainstRule(rec, present, rule); err != nil {
		return fence(rep, "%v", err)
	}

	// Rule 2.
	rows, err := r.socketEvidence(ctx, b)
	rep.Sockets = rows
	if err != nil {
		return fence(rep, "%v", err)
	}

	done := OutcomeRolledBack
	if a.Direction() == ingressauthority.RestartRetire {
		done = OutcomeRetired
	}
	switch {
	case !found:
		rep.Outcome, rep.Reason = OutcomeNoEffects, "no journal record, no owner rule and no socket: the interrupted operation had no host effects"
		return rep, nil
	case rec.Phase == PhaseRetired:
		rep.Outcome, rep.Reason = OutcomeAlreadyRetired, "journal already retired; rule verified absent; no listener or agent socket on the port"
		return rep, nil
	}
	if r.DryRun {
		rep.Outcome = OutcomeWouldReconcile
		rep.Reason = fmt.Sprintf("evidence complete: would finish as %s (journal %s, rule present=%v handle=%d)", done, rec.Phase, present, rule.Handle)
		return rep, nil
	}

	handle := rec.FirewallHandle
	if present {
		handle = rule.Handle
	}
	// Journal the intent first (a restart replays it), then remove by handle.
	if handle != 0 && rec.Phase != PhaseRetireIntent {
		next := MutationRecord{Port: b.Port, Owner: b.Proof, Phase: PhaseRetireIntent, FirewallHandle: handle}
		if err := a.Check(ctx); err != nil {
			return fence(rep, "authority lost: %v", err)
		}
		if err := r.Journal.Advance(next); err != nil {
			return fence(rep, "journal retire intent: %v", err)
		}
		rec = next
		rep.JournalPhase = rec.Phase
	}
	if present {
		// Fresh inventory and rule read immediately before the only
		// destructive step (the TOCTOU fence), then delete BY HANDLE.
		if err := a.Check(ctx); err != nil {
			return fence(rep, "authority lost: %v", err)
		}
		if rows, err = r.socketEvidence(ctx, b); err != nil {
			rep.Sockets = rows
			return fence(rep, "before delete: %v", err)
		}
		rep.Sockets = rows
		again, still, err := r.Firewall.ObserveOwner(ctx, plan)
		if err != nil || !still || again.Handle != handle {
			return fence(rep, "before delete: rule changed (present=%v handle=%d want %d): %v", still, again.Handle, handle, err)
		}
		if err := r.Firewall.Remove(ctx, plan, handle); err != nil {
			return fence(rep, "exact-handle delete: %v", err)
		}
	}
	// Absence is read back from the kernel before the journal may say retired.
	if _, gone, err := r.Firewall.ObserveOwner(ctx, plan); err != nil || gone {
		return fence(rep, "rule still present or unreadable after delete: %v", err)
	}
	if err := a.Check(ctx); err != nil {
		return fence(rep, "authority lost: %v", err)
	}
	if err := r.Journal.Advance(MutationRecord{Port: b.Port, Owner: b.Proof, Phase: PhaseRetired, FirewallHandle: handle}); err != nil {
		return fence(rep, "journal retired: %v", err)
	}
	rep.Outcome = done
	rep.JournalPhase = PhaseRetired
	rep.Reason = fmt.Sprintf("listener gone (no listener or agent-owned socket on port %d), owner rule handle %d %s, journal retired", b.Port, handle, map[bool]string{true: "deleted by handle", false: "proven absent"}[present])
	return rep, nil
}

// checkJournalAgainstRule rejects every combination of journal phase and
// kernel rule that the Apply/Retire state machine cannot have produced.
func checkJournalAgainstRule(rec MutationRecord, present bool, rule FirewallObservation) error {
	switch rec.Phase {
	case PhaseIntent:
		if present || rec.FirewallHandle != 0 {
			return fmt.Errorf("journal says intent but a rule exists or is recorded")
		}
	case PhaseListenerBound:
		if rec.FirewallHandle != 0 {
			return fmt.Errorf("journal says listener_bound but records a rule handle")
		}
		// A rule may exist: the process died after the kernel accepted the
		// add and before the journal recorded its handle.
	case PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired:
		if rec.FirewallHandle == 0 {
			return fmt.Errorf("journal phase %s lacks the rule handle", rec.Phase)
		}
		if present && rule.Handle != rec.FirewallHandle {
			return fmt.Errorf("kernel rule handle %d differs from journaled handle %d", rule.Handle, rec.FirewallHandle)
		}
		if rec.Phase == PhaseRetired && present {
			return fmt.Errorf("journal says retired but the owner rule exists")
		}
	default:
		return fmt.Errorf("unknown journal phase %q", rec.Phase)
	}
	return nil
}
