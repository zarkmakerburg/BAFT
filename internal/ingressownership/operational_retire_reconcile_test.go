package ingressownership

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func mustPreview(t *testing.T, spec ingressmanifest.Spec) ingressmanifest.Proof {
	t.Helper()
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func portOf(t *testing.T, endpoint string) uint16 {
	t.Helper()
	_, text, err := net.SplitHostPort(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		t.Fatal(err)
	}
	return uint16(n)
}

func listenerInode(t *testing.T, tx *NFTLabTransaction, endpoint string) uint64 {
	t.Helper()
	tx.Runtime.mu.Lock()
	var proof ingressmanifest.Proof
	if s := tx.Runtime.sessions[endpoint]; s != nil {
		proof = s.proof
	}
	tx.Runtime.mu.Unlock()
	inode, err := tx.ownedInode(endpoint, proof)
	if err != nil {
		t.Fatal(err)
	}
	return inode
}

// partialRetireFixture reproduces the recovery-liveness seam without a real
// listener: the owned socket was already closed by THIS process (recorded with
// its kernel inode), RetireIntent is durable, and the exact nft rule remains
// because Remove failed. No Alpha lease is minted; tests call the private
// evidence gates and the fenced effect with injected, in-package fakes.
type partialRetireFixture struct {
	x      *OperationalExecutor
	tx     *NFTLabTransaction
	fake   *nftLabFake
	cmd    ingressauthority.CommandV1
	plan   RulePlan
	handle uint64
	inode  uint64
}

func newPartialRetireFixture(t *testing.T) *partialRetireFixture {
	t.Helper()
	cmd := executorCommandV1(t)
	cmd.Action = "public_ingress_retire"
	b := cmd.Binding
	fake := &nftLabFake{chainHandle: 7}
	nft := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	journal := new(LabJournal)
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: nft, Journal: journal}
	record, plan, err := nftTransactionRecord(b.Spec, b.Proof)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound} {
		record.Phase = phase
		if err := journal.Advance(record); err != nil {
			t.Fatal(err)
		}
	}
	rule, err := nft.Install(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record.FirewallHandle = rule.Handle
	for _, phase := range []MutationPhase{PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent} {
		record.Phase = phase
		if err := journal.Advance(record); err != nil {
			t.Fatal(err)
		}
	}
	const inode = 4242424
	tx.stopped = map[ingressmanifest.Proof]stoppedIngressSocket{
		b.Proof: {runtime: tx.Runtime, firewall: nft, handle: rule.Handle, chainHandle: 7, inode: inode},
	}
	return &partialRetireFixture{x: &OperationalExecutor{transaction: tx}, tx: tx, fake: fake, cmd: cmd, plan: plan, handle: rule.Handle, inode: inode}
}

func (f *partialRetireFixture) evidence(rows ...ingressinventory.ActiveTCPClaim) ingressinventory.CoverageEvidence {
	return ingressinventory.CoverageEvidence{Port: f.cmd.Binding.Port, CoverageComplete: true, ActiveTCP: rows}
}

func (f *partialRetireFixture) fence(collect func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error)) *effectLeaseFence {
	return &effectLeaseFence{
		command:   f.cmd,
		authority: func(context.Context) error { return nil },
		installed: func(context.Context) error { return nil },
		collect:   collect,
	}
}

func TestReconcileStoppedRetireAcceptsOnlyProcessLocalVerifiedStop(t *testing.T) {
	ctx := context.Background()
	f := newPartialRetireFixture(t)
	b := f.cmd.Binding
	// Live-socket preflight still refuses (socket is not live): the seam.
	if err := f.x.ownedRetireListener(ctx, b, f.evidence()); err == nil {
		t.Fatal("stopped socket was classified as live owned listener")
	}
	if err := f.x.reconcileStoppedRetire(ctx, b, f.evidence()); err != nil {
		t.Fatalf("partial retire with process-local stopped evidence not reconcilable: %v", err)
	}
	// A different, foreign LISTEN is neither adopted nor touched.
	foreign := ingressinventory.ActiveTCPClaim{Port: b.Port, Family: "tcp6", State: "0A", Inode: f.inode + 1}
	if err := f.x.reconcileStoppedRetire(ctx, b, f.evidence(foreign)); err != nil {
		t.Fatalf("foreign rebinding blocked cleanup of our own exact rule: %v", err)
	}
	// Our closed inode still present in any state: not verified.
	for _, state := range []string{"0A", "01", "06"} {
		still := ingressinventory.ActiveTCPClaim{Port: b.Port, Family: "tcp4", State: state, Inode: f.inode}
		if err := f.x.reconcileStoppedRetire(ctx, b, f.evidence(still)); !errors.Is(err, ErrUnverified) {
			t.Fatalf("closed inode still in kernel (state %s) accepted: %v", state, err)
		}
	}
	incomplete := f.evidence()
	incomplete.CoverageComplete = false
	if err := f.x.reconcileStoppedRetire(ctx, b, incomplete); !errors.Is(err, ingressinventory.ErrCoverageIncomplete) {
		t.Fatalf("incomplete coverage accepted: %v", err)
	}
	unknown := f.evidence()
	unknown.UnknownSources = []string{"/proc/net/tcp6: unavailable"}
	if err := f.x.reconcileStoppedRetire(ctx, b, unknown); !errors.Is(err, ingressinventory.ErrCoverageIncomplete) {
		t.Fatalf("unknown source accepted: %v", err)
	}
	stopped := f.evidence()
	stopped.Stopped = []ingressinventory.PersistentTCPPortClaim{{Port: b.Port, Unit: "foreign.socket"}}
	if err := f.x.reconcileStoppedRetire(ctx, b, stopped); !errors.Is(err, ErrConflict) {
		t.Fatalf("stopped foreign claim waived: %v", err)
	}
	other := f.evidence(ingressinventory.ActiveTCPClaim{Port: b.Port + 1, Family: "tcp4", State: "0A", Inode: 9})
	if err := f.x.reconcileStoppedRetire(ctx, b, other); !errors.Is(err, ErrUnverified) {
		t.Fatalf("evidence for another port accepted: %v", err)
	}
}

func TestReconcileStoppedRetireRefusesWithoutIndependentEvidence(t *testing.T) {
	ctx := context.Background()
	mutate := map[string]func(f *partialRetireFixture){
		"restart: no process-local stopped record": func(f *partialRetireFixture) { f.tx.stopped = nil },
		"unknown closed inode": func(f *partialRetireFixture) {
			s := f.tx.stopped[f.cmd.Binding.Proof]
			s.inode = 0
			f.tx.stopped[f.cmd.Binding.Proof] = s
		},
		"different runtime": func(f *partialRetireFixture) { f.tx.Runtime = NewRuntime() },
		"different firewall backend": func(f *partialRetireFixture) {
			f.tx.Firewall = &NFTLabBackend{Runner: f.fake, ExpectedChainHandle: 7}
		},
		"handle mismatch": func(f *partialRetireFixture) {
			s := f.tx.stopped[f.cmd.Binding.Proof]
			s.handle++
			f.tx.stopped[f.cmd.Binding.Proof] = s
		},
		"journal still Verified (no durable retire intent)": func(f *partialRetireFixture) {
			rec, _, _ := f.tx.Journal.Current(f.cmd.Binding.Proof)
			f.tx.Journal = &journalOverride{rec: rec, phase: PhaseVerified}
		},
		"live session present": func(f *partialRetireFixture) {
			f.tx.Runtime.sessions[f.cmd.Binding.ListenEndpoint] = &session{proof: f.cmd.Binding.Proof}
		},
	}
	for name, fn := range mutate {
		t.Run(name, func(t *testing.T) {
			f := newPartialRetireFixture(t)
			fn(f)
			if err := f.x.reconcileStoppedRetire(ctx, f.cmd.Binding, f.evidence()); err == nil {
				t.Fatal("reconciliation accepted without independent evidence")
			}
		})
	}
	// A different tunnel generation/proof is never adopted.
	f := newPartialRetireFixture(t)
	wrong := f.cmd.Binding
	wrong.Proof.Generation++
	if err := f.x.reconcileStoppedRetire(ctx, wrong, f.evidence()); err == nil {
		t.Fatal("forged generation reconciled")
	}
}

// journalOverride reports a fixed current record; it never mutates anything.
type journalOverride struct {
	rec   MutationRecord
	phase MutationPhase
}

func (j *journalOverride) Advance(MutationRecord) error { return errors.New("read-only test journal") }
func (j *journalOverride) Current(ingressmanifest.Proof) (MutationRecord, bool, error) {
	r := j.rec
	r.Phase = j.phase
	return r, true, nil
}

// Every nft Remove must be immediately preceded by a fresh numeric-port
// inventory: a stale initial admission alone cannot authorize deleting.
func TestFencedNFTRemoveRechecksKernelSocketsAtEffect(t *testing.T) {
	ctx := context.Background()
	type tc struct {
		name       string
		collect    func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error)
		wantRemove bool
		wantErr    error
	}
	ret := func(e ingressinventory.CoverageEvidence, err error) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
		return func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) { return e, err }
	}
	cases := []tc{
		{"clean table", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			return ret(f.evidence(), nil)
		}, true, nil},
		{"foreign sibling listener elsewhere on port is untouched", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			return ret(f.evidence(ingressinventory.ActiveTCPClaim{Port: f.cmd.Binding.Port, Family: "tcp6", State: "0A", Inode: f.inode + 9}), ingressinventory.ErrClaimConflict)
		}, true, nil},
		{"closed inode reappears in kernel table", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			return ret(f.evidence(ingressinventory.ActiveTCPClaim{Port: f.cmd.Binding.Port, Family: "tcp4", State: "0A", Inode: f.inode}), ingressinventory.ErrClaimConflict)
		}, false, ErrUnverified},
		{"tcp6 table unavailable (incomplete coverage)", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			e := f.evidence()
			e.CoverageComplete = false
			e.UnknownSources = []string{"/proc/net/tcp6: unavailable"}
			return ret(e, ingressinventory.ErrCoverageIncomplete)
		}, false, ingressinventory.ErrCoverageIncomplete},
		{"collector failure", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			return ret(ingressinventory.CoverageEvidence{}, errors.New("policy drift"))
		}, false, nil},
		{"stopped foreign claim appears", func(f *partialRetireFixture) func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			e := f.evidence()
			e.Stopped = []ingressinventory.PersistentTCPPortClaim{{Port: f.cmd.Binding.Port, Unit: "foreign.socket"}}
			return ret(e, ingressinventory.ErrClaimConflict)
		}, false, ErrConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newPartialRetireFixture(t)
			b := f.cmd.Binding
			fence := f.fence(c.collect(f))
			before := len(f.fake.commands)
			err := f.tx.retireLocked(ctx, b.Spec, b.Proof, fence)
			removed := len(f.fake.commands) > before
			if c.wantRemove {
				if err != nil || !removed || f.fake.ruleHandle != 0 {
					t.Fatalf("expected exact-owned removal: err=%v removed=%v rule=%d", err, removed, f.fake.ruleHandle)
				}
				cur, _, _ := f.tx.Journal.Current(b.Proof)
				if cur.Phase != PhaseRetired {
					t.Fatalf("journal not finalized: %+v", cur)
				}
				return
			}
			if err == nil || removed || f.fake.ruleHandle == 0 {
				t.Fatalf("nft Remove ran without fresh socket evidence: err=%v removed=%v rule=%d", err, removed, f.fake.ruleHandle)
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("wrong refusal: %v", err)
			}
			cur, _, _ := f.tx.Journal.Current(b.Proof)
			if cur.Phase != PhaseRetireIntent {
				t.Fatalf("refusal lost uncertainty: %+v", cur)
			}
		})
	}
}

func TestFencedNFTRemoveRefusesUnknownClosedInode(t *testing.T) {
	f := newPartialRetireFixture(t)
	b := f.cmd.Binding
	s := f.tx.stopped[b.Proof]
	s.inode = 0
	f.tx.stopped[b.Proof] = s
	called := false
	fence := f.fence(func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
		called = true
		return f.evidence(), nil
	})
	if err := f.tx.retireLocked(context.Background(), b.Spec, b.Proof, fence); !errors.Is(err, ErrUnverified) {
		t.Fatalf("remove without known retired inode: %v", err)
	}
	if len(f.fake.commands) != 1 || f.fake.ruleHandle == 0 || called {
		t.Fatalf("effect ran: commands=%v rule=%d collected=%v", f.fake.commands, f.fake.ruleHandle, called)
	}
}

// The seam closes end to end: the second Retire after a failed nft Remove is
// accepted by the executor evidence gate, retireLocked finishes under fence.
func TestPartialRetireContinuesAfterFailedRemoveWithFreshEvidence(t *testing.T) {
	ctx := context.Background()
	f := newPartialRetireFixture(t)
	b := f.cmd.Binding
	f.fake.failCommand = true
	fence := f.fence(func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
		return f.evidence(), nil
	})
	if err := f.tx.retireLocked(ctx, b.Spec, b.Proof, fence); err == nil {
		t.Fatal("failed nft delete reported success")
	}
	cur, _, _ := f.tx.Journal.Current(b.Proof)
	if cur.Phase != PhaseRetireIntent || f.fake.ruleHandle == 0 {
		t.Fatalf("lost partial state: %+v rule=%d", cur, f.fake.ruleHandle)
	}
	f.fake.failCommand = false
	if err := f.x.reconcileStoppedRetire(ctx, b, f.evidence()); err != nil {
		t.Fatalf("gate refused reconcile after failed Remove: %v", err)
	}
	if err := f.tx.retireLocked(ctx, b.Spec, b.Proof, fence); err != nil {
		t.Fatalf("reconcile did not finish: %v", err)
	}
	cur, _, _ = f.tx.Journal.Current(b.Proof)
	if cur.Phase != PhaseRetired || f.fake.ruleHandle != 0 {
		t.Fatalf("not finalized: %+v rule=%d", cur, f.fake.ruleHandle)
	}
}

// Fresh socket evidence is required before the nft Install effect too; this
// needs a real Runtime listener and therefore the IPv4+IPv6 kernel tables.
func TestFencedNFTInstallRechecksKernelSocketsAtEffect(t *testing.T) {
	if _, err := os.Stat("/proc/net/tcp6"); err != nil {
		t.Skip("genuine prerequisite missing: /proc/net/tcp6 (no IPv6); Runtime admission fails closed")
	}
	ctx := context.Background()
	spec := testSpec(t, "install-effect")
	proof := mustPreview(t, spec)
	for _, name := range []string{"foreign-sibling", "unknown-coverage", "clean"} {
		t.Run(name, func(t *testing.T) {
			fake := &nftLabFake{chainHandle: 7}
			tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: new(LabJournal)}
			cmd := executorCommandV1(t)
			cmd.Binding.Spec, cmd.Binding.Proof = spec, proof
			cmd.Binding.ListenEndpoint = spec.Config.Listen
			port := portOf(t, spec.Config.Listen)
			cmd.Binding.Port, cmd.Binding.ConfigPort = port, port
			fence := &effectLeaseFence{command: cmd,
				authority: func(context.Context) error { return nil },
				installed: func(context.Context) error { return nil }}
			fence.collect = func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
				inode := listenerInode(t, tx, spec.Config.Listen)
				e := ingressinventory.CoverageEvidence{Port: port, CoverageComplete: true,
					ActiveTCP: []ingressinventory.ActiveTCPClaim{{Port: port, Family: "tcp4", State: "0A", Inode: inode}}}
				switch name {
				case "foreign-sibling":
					e.ActiveTCP = append(e.ActiveTCP, ingressinventory.ActiveTCPClaim{Port: port, Family: "tcp6", State: "0A", Inode: inode + 77})
				case "unknown-coverage":
					e.CoverageComplete = false
				}
				return e, ingressinventory.ErrClaimConflict
			}
			err := tx.apply(ctx, spec, proof, fence)
			defer tx.Runtime.Retire(spec.Config.Listen, proof)
			if name == "clean" {
				if err != nil || fake.ruleHandle == 0 {
					t.Fatalf("clean fenced install failed: %v", err)
				}
				return
			}
			if err == nil || fake.ruleHandle != 0 || strings.Contains(strings.Join(fake.commands, ";"), "add rule") {
				t.Fatalf("nft Install ran without fresh socket evidence: err=%v commands=%v", err, fake.commands)
			}
		})
	}
}
