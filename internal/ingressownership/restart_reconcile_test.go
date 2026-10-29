//go:build unix

package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// multiRuleNFT is a fake kernel chain holding any number of rules (siblings,
// foreign rules) so "nothing else is touched" is observable.
type multiRuleNFT struct {
	mu          sync.Mutex
	chainHandle uint64
	layoutBad   bool
	rules       map[uint64]string // handle -> raw rule JSON
	next        uint64
	commands    []string
	failDelete  bool
	failRead    bool
}

func newMultiNFT() *multiRuleNFT {
	return &multiRuleNFT{chainHandle: 7, rules: map[uint64]string{}, next: 100}
}

func ruleJSON(handle uint64, tag string, port int) string {
	return fmt.Sprintf(`{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":%d,"comment":"%s","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":%d}},{"accept":null}]}}`, handle, tag, port)
}

func (f *multiRuleNFT) addOwned(tag string, port int) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.rules[f.next] = ruleJSON(f.next, tag, port)
	return f.next
}

func (f *multiRuleNFT) ReadRuleset(context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return nil, errors.New("read failed")
	}
	policy := "accept"
	if f.layoutBad {
		policy = "drop"
	}
	items := []string{fmt.Sprintf(`{"chain":{"family":"inet","table":"baft","name":"ingress","handle":%d,"type":"filter","hook":"input","prio":-100,"policy":"%s"}}`, f.chainHandle, policy)}
	var hs []uint64
	for h := range f.rules {
		hs = append(hs, h)
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i] < hs[j] })
	for _, h := range hs {
		items = append(items, f.rules[h])
	}
	return []byte(`{"nftables":[` + strings.Join(items, ",") + `]}`), nil
}

func (f *multiRuleNFT) Run(_ context.Context, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, strings.Join(args, " "))
	if len(args) == 7 && args[0] == "delete" {
		if f.failDelete {
			return errors.New("injected delete failure")
		}
		h, _ := strconv.ParseUint(args[6], 10, 64)
		delete(f.rules, h)
		return nil
	}
	return errors.New("the reconciler may only delete by handle")
}

func (f *multiRuleNFT) snapshot() string {
	raw, _ := f.ReadRuleset(context.Background())
	return string(raw)
}

type fakeAuth struct {
	b      ingressauthority.BindingV1
	dir    ingressauthority.RestartDirection
	cmd    ingressauthority.CommandV1
	lost   bool
	checks int
	// Simulates a host change during slow guard validation, after the outer
	// collector snapshot but before entering the guarded leaf effect.
	beforeEffect func()
}

func (a *fakeAuth) Binding() ingressauthority.BindingV1          { return a.b }
func (a *fakeAuth) Direction() ingressauthority.RestartDirection { return a.dir }
func (a *fakeAuth) Command() ingressauthority.CommandV1          { return a.cmd }
func (a *fakeAuth) Check(context.Context) error {
	a.checks++
	if a.lost {
		return ingressauthority.ErrUnverified
	}
	return nil
}

func (a *fakeAuth) WithLockedReconciliation(ctx context.Context, effect func() error) error {
	if err := a.Check(ctx); err != nil {
		return err
	}
	if a.beforeEffect != nil {
		a.beforeEffect()
	}
	err := effect()
	return errors.Join(err, a.Check(ctx))
}

type restartRig struct {
	t       *testing.T
	nft     *multiRuleNFT
	journal *LabJournal
	wrapped MutationJournal
	auth    *fakeAuth
	plan    RulePlan
	spec    ingressmanifest.Spec
	proof   ingressmanifest.Proof
	rows    []ingressinventory.ActiveTCPClaim
	stopped []ingressinventory.PersistentTCPPortClaim
	cov     ingressinventory.CoverageEvidence
	covErr  error
	collect int
	rec     *RestartReconciler
}

func newRestartRig(t *testing.T, dir ingressauthority.RestartDirection) *restartRig {
	t.Helper()
	spec := testSpec(t, "restart-rig")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	_, plan, err := nftTransactionRecord(spec, proof)
	if err != nil {
		t.Fatal(err)
	}
	b := ingressauthority.BindingV1{NodeID: "ir-1", Port: plan.Port, ListenEndpoint: spec.Config.Listen, Spec: spec, Proof: proof}
	r := &restartRig{t: t, nft: newMultiNFT(), journal: new(LabJournal), plan: plan, spec: spec, proof: proof,
		auth: &fakeAuth{b: b, dir: dir, cmd: ingressauthority.CommandV1{JobID: "job-1", Action: "public_ingress_retire", Binding: b}}}
	r.wrapped = r.journal
	r.cov = ingressinventory.CoverageEvidence{CoverageComplete: true}
	r.build()
	return r
}

func (r *restartRig) build() {
	r.rec = &RestartReconciler{
		Journal:  r.wrapped,
		Firewall: &NFTLabBackend{Runner: r.nft, ExpectedChainHandle: 7},
		AgentUID: 0, UIDSet: true,
		Collect: func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
			r.collect++
			c := r.cov
			c.ActiveTCP, c.Stopped = r.rows, r.stopped
			return c, r.covErr
		},
	}
}

func (r *restartRig) journalAt(phase MutationPhase, handle uint64) {
	r.t.Helper()
	steps := []MutationRecord{{Port: r.plan.Port, Owner: r.proof, Phase: PhaseIntent}}
	add := func(p MutationPhase, h uint64) {
		steps = append(steps, MutationRecord{Port: r.plan.Port, Owner: r.proof, Phase: p, FirewallHandle: h})
	}
	switch phase {
	case PhaseListenerBound:
		add(PhaseListenerBound, 0)
	case PhaseFirewallInstalled:
		add(PhaseListenerBound, 0)
		add(PhaseFirewallInstalled, handle)
	case PhaseVerified:
		add(PhaseListenerBound, 0)
		add(PhaseFirewallInstalled, handle)
		add(PhaseVerified, handle)
	case PhaseRetireIntent:
		add(PhaseListenerBound, 0)
		add(PhaseFirewallInstalled, handle)
		add(PhaseVerified, handle)
		add(PhaseRetireIntent, handle)
	}
	for _, s := range steps {
		if err := r.journal.Advance(s); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *restartRig) phase() MutationPhase {
	rec, found, err := r.journal.Current(r.proof)
	if err != nil || !found {
		r.t.Fatalf("journal: %v %v", found, err)
	}
	return rec.Phase
}

func (r *restartRig) run() (RestartReport, error) {
	return r.rec.reconcile(context.Background(), r.auth)
}

func (r *restartRig) mustFence(why string) RestartReport {
	r.t.Helper()
	before, ph := r.nft.snapshot(), r.phase0()
	rep, err := r.run()
	if !errors.Is(err, ErrRestartFenced) || rep.Outcome != OutcomeFenced {
		r.t.Fatalf("%s: not fenced: %+v %v", why, rep, err)
	}
	if len(r.nft.commands) != 0 || r.nft.snapshot() != before || r.phase0() != ph {
		r.t.Fatalf("%s: a fenced decision changed state (commands=%v phase %q->%q)", why, r.nft.commands, ph, r.phase0())
	}
	return rep
}

func (r *restartRig) phase0() MutationPhase {
	rec, found, _ := r.journal.Current(r.proof)
	if !found {
		return ""
	}
	return rec.Phase
}

func row(family, state string, inode uint64, uid uint32, port uint16) ingressinventory.ActiveTCPClaim {
	return ingressinventory.ActiveTCPClaim{Port: port, Family: family, State: state, Inode: inode, Uid: uid}
}

// ---------------------------------------------------------------- positive

func TestRestartRetireCompletesInterruptedRetireByExactHandle(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	sibling := r.nft.addOwned("baft-ingress-00000000000000000000000000000001", 40001)
	foreign := r.nft.next + 1
	r.nft.next = foreign
	r.nft.rules[foreign] = `{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":` + strconv.Itoa(int(foreign)) + `,"comment":"baft-ingress-ffffffffffffffffffffffffffffffff","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":40002}},{"accept":null}]}}`
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, h)
	rep, err := r.run()
	if err != nil || rep.Outcome != OutcomeRetired || rep.RuleHandle != h || !rep.RulePresent {
		t.Fatalf("report: %+v %v", rep, err)
	}
	want := fmt.Sprintf("delete rule inet baft ingress handle %d", h)
	if len(r.nft.commands) != 1 || r.nft.commands[0] != want {
		t.Fatalf("commands %v, want exactly [%s]", r.nft.commands, want)
	}
	if _, ok := r.nft.rules[sibling]; !ok {
		t.Fatal("sibling rule removed")
	}
	if _, ok := r.nft.rules[foreign]; !ok {
		t.Fatal("other rule removed")
	}
	if _, ok := r.nft.rules[h]; ok {
		t.Fatal("owner rule survived")
	}
	if r.phase() != PhaseRetired {
		t.Fatalf("journal %s", r.phase())
	}
	// Idempotent: the next start finds retired + absent and writes nothing.
	r.nft.commands = nil
	rep, err = r.run()
	if err != nil || rep.Outcome != OutcomeAlreadyRetired || len(r.nft.commands) != 0 {
		t.Fatalf("rerun: %+v %v %v", rep, err, r.nft.commands)
	}
}

func TestRestartApplyRollsBackEveryInterruptedPhase(t *testing.T) {
	for _, tc := range []struct {
		name    string
		phase   MutationPhase
		rule    bool // rule exists in the kernel
		journal bool // handle journaled
	}{
		{"listener-bound-rule-installed-but-not-journaled", PhaseListenerBound, true, false},
		{"listener-bound-no-rule", PhaseListenerBound, false, false},
		{"intent-only", PhaseIntent, false, false},
		{"firewall-installed", PhaseFirewallInstalled, true, true},
		{"verified-listener-died-with-the-process", PhaseVerified, true, true},
		{"verified-rule-lost-in-reboot", PhaseVerified, false, true},
		{"retire-intent-rule-already-deleted", PhaseRetireIntent, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRestartRig(t, ingressauthority.RestartApply)
			keep := r.nft.addOwned("baft-ingress-00000000000000000000000000000002", 40003)
			var h uint64
			if tc.rule {
				h = r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
			} else {
				h = 55
			}
			jh := uint64(0)
			if tc.journal {
				jh = h
			}
			r.journalAt(tc.phase, jh)
			rep, err := r.run()
			if err != nil || rep.Outcome != OutcomeRolledBack {
				t.Fatalf("report: %+v %v", rep, err)
			}
			if r.phase() != PhaseRetired {
				t.Fatalf("journal %s", r.phase())
			}
			if _, ok := r.nft.rules[keep]; !ok {
				t.Fatal("sibling rule removed")
			}
			if tc.rule {
				if len(r.nft.commands) != 1 || !strings.HasSuffix(r.nft.commands[0], fmt.Sprintf("handle %d", h)) {
					t.Fatalf("commands %v", r.nft.commands)
				}
			} else if len(r.nft.commands) != 0 {
				t.Fatalf("nothing to delete but commands ran: %v", r.nft.commands)
			}
		})
	}
}

func TestRestartDryRunWritesNothing(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, h)
	r.rec.DryRun = true
	before := r.nft.snapshot()
	rep, err := r.run()
	if err != nil || rep.Outcome != OutcomeWouldReconcile {
		t.Fatalf("report: %+v %v", rep, err)
	}
	if r.nft.snapshot() != before || len(r.nft.commands) != 0 || r.phase() != PhaseRetireIntent {
		t.Fatal("dry run changed state")
	}
}

func TestRestartIgnoresKernelRemnantsAndForeignNonListeningRowsButReportsThem(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, h)
	p := r.plan.Port
	r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "06", 0, 0, p), row("tcp6", "08", 0, 0, p), row("tcp4", "01", 777, 1000, p)}
	rep, err := r.run()
	if err != nil || rep.Outcome != OutcomeRetired {
		t.Fatalf("report: %+v %v", rep, err)
	}
	classes := map[string]int{}
	for _, s := range rep.Sockets {
		classes[s.Class]++
	}
	if classes["kernel-remnant"] != 2 || classes["foreign"] != 1 {
		t.Fatalf("sockets not reported by class: %+v", rep.Sockets)
	}
}

// ---------------------------------------------------------------- fail closed

func TestRestartFencesOnSocketEvidence(t *testing.T) {
	p := func(r *restartRig) uint16 { return r.plan.Port }
	for name, mutate := range map[string]func(r *restartRig){
		"foreign-v4-listener":          func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "0A", 11, 33, p(r))} },
		"foreign-v6-listener":          func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp6", "0A", 12, 33, p(r))} },
		"listener-owned-by-agent-uid":  func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "0A", 13, 0, p(r))} },
		"agent-uid-established-socket": func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "01", 14, 0, p(r))} },
		"agent-uid-close-wait-socket":  func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp6", "08", 15, 0, p(r))} },
		"row-for-another-port":         func(r *restartRig) { r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "06", 0, 0, p(r)+1)} },
		"coverage-incomplete":          func(r *restartRig) { r.cov.CoverageComplete = false },
		"unknown-source":               func(r *restartRig) { r.cov.UnknownSources = []string{"/proc/net/tcp6: unavailable"} },
		"collector-error":              func(r *restartRig) { r.covErr = errors.New("collector exploded") },
		"coverage-error":               func(r *restartRig) { r.covErr = ingressinventory.ErrCoverageIncomplete },
		"stopped-systemd-claim-on-port": func(r *restartRig) {
			r.stopped = []ingressinventory.PersistentTCPPortClaim{{Port: p(r), Unit: "x.socket"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRestartRig(t, ingressauthority.RestartRetire)
			h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
			r.journalAt(PhaseRetireIntent, h)
			mutate(r)
			r.build()
			rep := r.mustFence(name)
			t.Logf("%s -> %s", name, rep.Reason)
			if rep.Outcome != OutcomeFenced {
				t.Fatalf("outcome %s", rep.Outcome)
			}
		})
	}
}

func TestRestartForeignListenerIsReportedNeverAdopted(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartApply)
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseVerified, h)
	r.rows = []ingressinventory.ActiveTCPClaim{row("tcp4", "0A", 4242, 998, r.plan.Port)}
	rep := r.mustFence("foreign listener")
	if len(rep.Sockets) != 1 || rep.Sockets[0].Class != "foreign-listener" || rep.Sockets[0].Inode != 4242 || rep.Sockets[0].Uid != 998 {
		t.Fatalf("foreign socket not reported with its exact evidence: %+v", rep.Sockets)
	}
}

func TestRestartFencesOnFirewallEvidence(t *testing.T) {
	cases := map[string]func(r *restartRig, h uint64){
		"handle-differs-from-journal": func(r *restartRig, h uint64) {
			delete(r.nft.rules, h)
			r.nft.rules[h+50] = ruleJSON(h+50, r.plan.Tag, int(r.plan.Port))
		},
		"foreign-rule-on-the-same-dport": func(r *restartRig, h uint64) {
			r.nft.rules[h+60] = ruleJSON(h+60, "baft-ingress-0123456789abcdef0123456789abcdef", int(r.plan.Port))
		},
		"duplicate-owner-rule": func(r *restartRig, h uint64) {
			r.nft.rules[h+70] = ruleJSON(h+70, r.plan.Tag, int(r.plan.Port))
		},
		"chain-layout-drifted": func(r *restartRig, h uint64) { r.nft.layoutBad = true },
		"chain-handle-drifted": func(r *restartRig, h uint64) { r.nft.chainHandle = 9 },
		"ruleset-unreadable":   func(r *restartRig, h uint64) { r.nft.failRead = true },
		"opaque-extra-rule": func(r *restartRig, h uint64) {
			r.nft.rules[h+80] = `{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":` + strconv.Itoa(int(h+80)) + `,"comment":"foreign","expr":[{"accept":null}]}}`
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRestartRig(t, ingressauthority.RestartRetire)
			h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
			r.journalAt(PhaseRetireIntent, h)
			mutate(r, h)
			rep := r.mustFence(name)
			t.Logf("%s -> %s", name, rep.Reason)
		})
	}
}

func TestRestartFencesOnJournalAndAuthorityEvidence(t *testing.T) {
	t.Run("rule-without-journal-record", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartRetire)
		r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
		before := r.nft.snapshot()
		rep, err := r.run()
		if !errors.Is(err, ErrRestartFenced) || r.nft.snapshot() != before || len(r.nft.commands) != 0 {
			t.Fatalf("%+v %v", rep, err)
		}
	})
	t.Run("no-record-no-rule-is-no-effects", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartRetire)
		rep, err := r.run()
		if err != nil || rep.Outcome != OutcomeNoEffects || len(r.nft.commands) != 0 {
			t.Fatalf("%+v %v", rep, err)
		}
	})
	t.Run("journal-says-retired-but-rule-exists", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartRetire)
		h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
		r.journalAt(PhaseRetireIntent, h)
		if err := r.journal.Advance(MutationRecord{Port: r.plan.Port, Owner: r.proof, Phase: PhaseRetired, FirewallHandle: h}); err != nil {
			t.Fatal(err)
		}
		r.mustFence("retired journal with live rule")
	})
	t.Run("intent-phase-with-a-rule", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartApply)
		r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
		r.journalAt(PhaseIntent, 0)
		r.mustFence("rule before the listener was even bound")
	})
	t.Run("authority-lost", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartRetire)
		h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
		r.journalAt(PhaseRetireIntent, h)
		r.auth.lost = true
		r.mustFence("authority")
	})
	t.Run("wrong-journal-owner", func(t *testing.T) {
		r := newRestartRig(t, ingressauthority.RestartRetire)
		other := r.spec
		other.Generation++
		op, _ := ingressmanifest.Preview(other)
		h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
		// the journal only knows another generation of the same tunnel
		if err := r.journal.Advance(MutationRecord{Port: r.plan.Port, Owner: op, Phase: PhaseIntent}); err != nil {
			t.Fatal(err)
		}
		_ = h
		rep, err := r.run()
		if !errors.Is(err, ErrRestartFenced) {
			t.Fatalf("%+v %v", rep, err)
		}
	})
}

// A cancelled context, an unconfigured reconciler and a nil authority all deny.
func TestRestartReconcilerDeniesWithoutTrust(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.rec.reconcile(ctx, r.auth); err == nil || len(r.nft.commands) != 0 {
		t.Fatalf("cancelled context: %v %v", err, r.nft.commands)
	}
	if _, err := r.rec.Reconcile(context.Background(), nil); !errors.Is(err, ErrRestartFenced) {
		t.Fatalf("nil authority: %v", err)
	}
	for _, bad := range []*RestartReconciler{nil, {}, {Journal: r.journal}, {Journal: r.journal, Firewall: &NFTLabBackend{Runner: r.nft}}} {
		if _, err := bad.reconcile(context.Background(), r.auth); !errors.Is(err, ErrRestartFenced) {
			t.Fatalf("unconfigured reconciler accepted: %v", err)
		}
	}
	if len(r.nft.commands) != 0 {
		t.Fatalf("denied reconciliation ran commands: %v", r.nft.commands)
	}
}

// ---------------------------------------------------------------- crash safety

type faultJournal struct {
	MutationJournal
	failPhase MutationPhase
}

func (j *faultJournal) Advance(r MutationRecord) error {
	if j.failPhase != "" && r.Phase == j.failPhase {
		return errors.New("injected journal failure")
	}
	return j.MutationJournal.Advance(r)
}

// The reconciler may itself die or fail at any step; the next run finishes
// from the same evidence and never repeats a destructive step blindly.
func TestRestartIsResumableAtEveryStep(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failPhase  MutationPhase
		failDelete bool
		wantRule   bool // rule still present after the failed run
		wantPhase  MutationPhase
	}{
		{"journal-retire-intent-write-fails", PhaseRetireIntent, false, true, PhaseVerified},
		{"exact-delete-fails", "", true, true, PhaseRetireIntent},
		{"final-journal-write-fails-after-delete", PhaseRetired, false, false, PhaseRetireIntent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRestartRig(t, ingressauthority.RestartApply)
			h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
			r.journalAt(PhaseVerified, h)
			fj := &faultJournal{MutationJournal: r.journal, failPhase: tc.failPhase}
			r.wrapped = fj
			r.build()
			r.nft.failDelete = tc.failDelete
			if _, err := r.run(); !errors.Is(err, ErrRestartFenced) {
				t.Fatalf("injected failure not reported: %v", err)
			}
			if _, ok := r.nft.rules[h]; ok != tc.wantRule {
				t.Fatalf("rule present=%v want %v", ok, tc.wantRule)
			}
			if r.phase() != tc.wantPhase {
				t.Fatalf("phase %s want %s", r.phase(), tc.wantPhase)
			}
			// restart again with the fault cleared
			fj.failPhase, r.nft.failDelete = "", false
			r.nft.commands = nil
			rep, err := r.run()
			if err != nil || rep.Outcome != OutcomeRolledBack || r.phase() != PhaseRetired {
				t.Fatalf("resume: %+v %v phase=%s", rep, err, r.phase())
			}
			if _, ok := r.nft.rules[h]; ok {
				t.Fatal("rule survived the resumed run")
			}
			if tc.wantRule && len(r.nft.commands) != 1 {
				t.Fatalf("resume commands %v", r.nft.commands)
			}
			if !tc.wantRule && len(r.nft.commands) != 0 {
				t.Fatalf("resume repeated the delete: %v", r.nft.commands)
			}
		})
	}
}

// The socket inventory is taken again immediately before the delete: a
// listener that appears between the first check and the effect denies it.
func TestRestartRechecksSocketsRightBeforeTheDelete(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	h := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, h)
	p := r.plan.Port
	n := 0
	r.rec.Collect = func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
		n++
		c := r.cov
		if n >= 2 {
			c.ActiveTCP = []ingressinventory.ActiveTCPClaim{row("tcp4", "0A", 99, 0, p)}
		}
		return c, nil
	}
	rep, err := r.run()
	if !errors.Is(err, ErrRestartFenced) || len(r.nft.commands) != 0 {
		t.Fatalf("late listener ignored: %+v %v %v", rep, err, r.nft.commands)
	}
	if _, ok := r.nft.rules[h]; !ok {
		t.Fatal("rule removed despite a late listener")
	}
}
