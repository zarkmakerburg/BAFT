//go:build linux

package operational

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

func tagOf(t *testing.T, s *site) string {
	t.Helper()
	plan, err := ingressownership.PlanIngressRule(uint16(s.port), s.cmd.Binding.Proof)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Tag
}

// enqueueApply stages and queues a site's Apply (the Agent must already have
// declared the capability) without running the Agent.
func (s *site) enqueueApply(t *testing.T) {
	t.Helper()
	s.stage(t)
	s.f.a.openGate()
	if err := s.f.b.store.EnqueueOperationalIngress(s.tn.ID); err != nil {
		t.Fatalf("enqueue %s: %v", s.tn.ID, err)
	}
}

// enqueueRetire queues a NEW signed Retire job (gate re-checked by a fresh
// capability poll).
func (s *site) enqueueRetire(t *testing.T) {
	t.Helper()
	s.f.a.openGate()
	if err := s.f.b.store.EnqueueOperationalIngressRetire(s.tn.ID); err != nil {
		t.Fatalf("enqueue retire %s: %v", s.tn.ID, err)
	}
}

func (f *fullPath) journalPhase(t *testing.T, s *site) (ingressownership.MutationPhase, bool) {
	t.Helper()
	rec, found, err := f.a.J.Current(s.cmd.Binding.Proof)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Phase, found
}

func (f *fullPath) leftoverState(t *testing.T, s *site) (ingressauthority.RecoveryState, bool) {
	t.Helper()
	left, err := f.a.Store.InspectFencedLeftovers()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range left {
		if int(l.Binding.Port) == s.port {
			return l.State, true
		}
	}
	return "", false
}

// A kernel add that the nft process reports as failed AFTER the kernel took
// the rule must roll back exactly the rule handle this transaction owns:
// a sibling tunnel's rule, its listener and unrelated tables stay untouched.
func TestOperationalIngressV1Gate_PartialNFTAddRollsBackOwnedHandlesOnly(t *testing.T) {
	f := setupFullPath(t, 54761, "127.0.0.1:1443")
	keep := f.h.addKeep()
	keepBefore := keep()
	f.legit(t)
	aChain := f.h.chainJSON()

	b := f.addSite(t, "ex-opv2", "second", 8444, 54762, "127.0.0.1:1444")
	b.enqueueApply(t)
	f.a.Runner.failAdd = true
	res := f.a.runOnce()
	if !isFailure(res) {
		t.Fatalf("a partial nft add was reported as success: %+v", res)
	}
	if got := f.h.chainJSON(); got != aChain {
		t.Fatalf("rollback touched more (or less) than its own handle:\nbefore=%s\nafter=%s", aChain, got)
	}
	if strings.Contains(f.h.chainJSON(), tagOf(t, b)) {
		t.Fatal("the failed transaction's own rule survived")
	}
	waitPortFree(t, b.port)
	if keep() != keepBefore {
		t.Fatal("unrelated nft table changed")
	}
	if err := roundTrip(t, f.addr(), []byte("sibling-ok")); err != nil {
		t.Fatalf("sibling tunnel disturbed by the failed install: %v", err)
	}
	if ph, found := f.journalPhase(t, b); found && ph == ingressownership.PhaseVerified {
		t.Fatalf("journal claims a verified owner after rollback: %s", ph)
	}
	if st, ok := f.leftoverState(t, b); !ok || st == "" {
		t.Fatalf("the uncertain install did not stay fenced for reconciliation (state=%q present=%v)", st, ok)
	}
	opb := f.b.op(b.tn.ID)
	if opb.Receipt == nil || opb.Receipt.Phase == "applied" || opb.Receipt.OwnershipVerified {
		t.Fatalf("BCC holds an applied receipt for the rolled-back install: %+v", opb.Receipt)
	}
	if v := f.b.readiness(b.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card for the failed install: %+v", v)
	}
	if v := f.waitReady(t); !v.Ready {
		t.Fatalf("sibling card lost: %+v", v)
	}

	// Worst case: the rollback's delete ALSO fails. The kernel rule of this
	// transaction (and only it) is retained as evidence, the listener is
	// stopped, and the sibling is still untouched.
	c := f.addSite(t, "ex-opv3", "third", 8445, 54763, "127.0.0.1:1445")
	c.enqueueApply(t)
	f.a.Runner.failAdd, f.a.Runner.failDelete = true, true
	if res := f.a.runOnce(); !isFailure(res) {
		t.Fatalf("partial add with failed rollback reported success: %+v", res)
	}
	f.a.Runner.failDelete = false
	chain := f.h.chainJSON()
	if !strings.Contains(chain, tagOf(t, c)) || !strings.Contains(chain, tagOf(t, f.site)) || strings.Contains(chain, tagOf(t, b)) {
		t.Fatalf("retained/rolled-back rule set is wrong: %s", chain)
	}
	waitPortFree(t, c.port)
	if err := roundTrip(t, f.addr(), []byte("sibling-ok-2")); err != nil {
		t.Fatalf("sibling tunnel disturbed: %v", err)
	}
	if v := f.b.readiness(c.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card for an unverifiable install: %+v", v)
	}
	if keep() != keepBefore {
		t.Fatal("unrelated nft table changed")
	}
}

// A Retire whose exact-handle nft delete fails stops the listener but keeps
// the rule, the fence and the "not retired" truth; a NEW signed Retire job
// resumes and completes it.
func TestOperationalIngressV1Gate_PartialNFTRemoveResumesRetire(t *testing.T) {
	f := setupFullPath(t, 54765, "127.0.0.1:1443")
	sib := f.addSite(t, "ex-opv2", "second", 8444, 54766, "127.0.0.1:1444")
	f.legit(t)
	sib.apply(t)
	if v := sib.waitReady(t); !v.Ready {
		t.Fatalf("sibling not ready: %+v", v)
	}
	sibChain := tagOf(t, sib)

	f.enqueueRetire(t)
	f.a.Runner.failDelete = true
	res := f.a.runOnce()
	if !isFailure(res) {
		t.Fatalf("Retire with a failed kernel delete reported success: %+v", res)
	}
	f.a.Runner.failDelete = false
	chain := f.h.chainJSON()
	if !strings.Contains(chain, tagOf(t, f.site)) {
		t.Fatalf("the owned rule vanished although its delete failed: %s", chain)
	}
	if op := f.b.op(f.tn.ID); op.RetireReceipt != nil && op.RetireReceipt.Phase == "retired" {
		t.Fatalf("BCC recorded retired before cleanup was observed: %+v", op.RetireReceipt)
	}
	if v := f.b.readiness(f.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card shown while retire is unresolved: %+v", v)
	}
	if ph, found := f.journalPhase(t, f.site); !found || ph != ingressownership.PhaseRetireIntent {
		t.Fatalf("journal lost the retire intent: %v %v", ph, found)
	}
	if err := roundTrip(t, sib.addr(), []byte("sibling")); err != nil {
		t.Fatalf("sibling disturbed by the failed retire: %v", err)
	}
	// Resume with a NEW signed Retire job.
	f.enqueueRetire(t)
	res = f.a.runOnce()
	if len(res) != 1 || res[0].Status != "succeeded" || res[0].Action != agentjob.ActionPublicIngressRetire {
		t.Fatalf("resumed Retire: %+v", res)
	}
	if strings.Contains(f.h.chainJSON(), tagOf(t, f.site)) {
		t.Fatal("rule survived a successful Retire")
	}
	if !strings.Contains(f.h.chainJSON(), sibChain) {
		t.Fatal("sibling's rule was removed")
	}
	if ph, _ := f.journalPhase(t, f.site); ph != ingressownership.PhaseRetired {
		t.Fatalf("journal phase after retire: %s", ph)
	}
	if op := f.b.op(f.tn.ID); op.RetireReceipt == nil || op.RetireReceipt.Phase != "retired" {
		t.Fatalf("BCC retire receipt: %+v", op.RetireReceipt)
	}
	waitPortFree(t, f.port)
	if err := roundTrip(t, sib.addr(), []byte("sibling-after")); err != nil {
		t.Fatalf("sibling disturbed by the resumed retire: %v", err)
	}
}

// A restarted Agent finds the previous owner's kernel rule, reservation and
// journal intact and fenced. It never guesses ownership: it neither re-binds,
// retires nor releases anything on its own.
func TestOperationalIngressV1Gate_AgentRestartKeepsFencedOwnership(t *testing.T) {
	f := setupFullPath(t, 54771, "127.0.0.1:1443")
	f.legit(t)
	rules := f.h.chainJSON()
	leftBefore, _ := f.a.Store.InspectFencedLeftovers()
	lb, _ := json.Marshal(leftBefore)
	dir := f.a.dir
	f.a.kill()
	waitPortFree(t, f.port)
	f.a = startAgent(t, f.b, f.h, dir, &agentOpt{url: f.tap.srv.URL, client: f.b.cl})

	if got := f.h.chainJSON(); got != rules {
		t.Fatalf("restart changed the kernel rule set:\nbefore=%s\nafter=%s", rules, got)
	}
	leftAfter, err := f.a.Store.InspectFencedLeftovers()
	if err != nil {
		t.Fatal(err)
	}
	la, _ := json.Marshal(leftAfter)
	if string(la) != string(lb) || len(leftAfter) != 1 {
		t.Fatalf("the reservation did not survive the restart intact:\nbefore=%s\nafter=%s", lb, la)
	}
	rec, err := f.a.D.Recover()
	if err != nil || len(rec.Records) != 1 {
		t.Fatalf("journal recovery: %+v %v", rec, err)
	}
	if !portFree(f.port) {
		t.Fatal("something listens on the port after restart")
	}
	// Fresh, correctly signed Apply for the same binding: nothing is
	// re-adopted by guess. Whatever the verdict, the port must not be
	// re-bound by this Agent and the single fenced owner must remain.
	res := f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) {
		j.JobID = "apply-after-restart"
		j.IssuedAt = time.Now().Add(-time.Second)
		j.ExpiresAt = time.Now().Add(time.Minute)
	}))
	if len(res) == 1 && res[0].Status == "succeeded" && !portFree(f.port) {
		t.Fatalf("a restarted Agent silently re-adopted fenced ownership: %+v", res)
	}
	if n := strings.Count(f.h.chainJSON(), "baft-ingress-"); n != 1 {
		t.Fatalf("restart duplicated or lost the owned rule (%d): %s", n, f.h.chainJSON())
	}
	// A new signed Retire after restart cannot remove what this process never
	// proved it stopped: the rule stays until an operator reconciles.
	f.enqueueRetire(t)
	res = f.a.runOnce()
	if !isFailure(res) {
		t.Fatalf("Retire after restart claimed cleanup without proof: %+v", res)
	}
	if n := strings.Count(f.h.chainJSON(), "baft-ingress-"); n != 1 {
		t.Fatalf("Retire after restart touched the rule set: %s", f.h.chainJSON())
	}
	if op := f.b.op(f.tn.ID); op.RetireReceipt != nil && op.RetireReceipt.Phase == "retired" {
		t.Fatalf("BCC believes the ingress retired: %+v", op.RetireReceipt)
	}
	left, _ := f.a.Store.InspectFencedLeftovers()
	if len(left) != 1 {
		t.Fatalf("the fence was released without observed cleanup: %+v", left)
	}
	// A foreign process may take the freed port, and keeps it.
	ln := foreignListen(t, "tcp4", fmt.Sprintf("0.0.0.0:%d", f.port))
	bannerServer(t, ln, "FOREIGN-AFTER-RESTART\n")
	if b, err := readBanner(ln.Addr().String()); err != nil || b != "FOREIGN-AFTER-RESTART\n" {
		t.Fatalf("foreign service not served: %q %v", b, err)
	}
}

// Retire is a NEW signed job; the port is released only after the listener,
// the exact kernel rule and the journal all show cleanup, and BCC then stops
// presenting a card.
func TestOperationalIngressV1Gate_NewSignedRetireJobReleasesOnlyAfterObservedCleanup(t *testing.T) {
	f := setupFullPath(t, 54775, "127.0.0.1:1443")
	// Retire before anything was applied is refused by BCC.
	f.stage(t)
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngressRetire(f.tn.ID); err == nil {
		t.Fatal("BCC queued a Retire without an applied, ownership-verified receipt")
	}
	f.b.store.EnqueueOperationalIngress(f.tn.ID)
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("apply: %+v", res)
	}
	if v := f.waitReady(t); !v.Ready {
		t.Fatalf("not ready: %+v", v)
	}
	applyJob := f.cmd.JobID
	body := f.tap.lastCaptured()

	f.enqueueRetire(t)
	op := f.b.op(f.tn.ID)
	if op.RetireCommand == nil || op.RetireCommand.JobID == applyJob || op.RetireCommand.Action != agentjob.ActionPublicIngressRetire {
		t.Fatalf("Retire must be a NEW signed job identity: %+v", op.RetireCommand)
	}
	res := f.a.runOnce()
	if len(res) != 1 || res[0].Status != "succeeded" || res[0].Action != agentjob.ActionPublicIngressRetire {
		t.Fatalf("Retire result: %+v", res)
	}
	// Observed cleanup: listener, rule, journal, BCC receipt.
	waitPortFree(t, f.port)
	if n := f.h.baftRules(); n != 0 {
		t.Fatalf("rules after retire: %s", f.h.chainJSON())
	}
	if ph, _ := f.journalPhase(t, f.site); ph != ingressownership.PhaseRetired {
		t.Fatalf("journal: %s", ph)
	}
	op = f.b.op(f.tn.ID)
	if op.RetireReceipt == nil || op.RetireReceipt.Phase != "retired" || op.RetireReceipt.OwnershipVerified {
		t.Fatalf("retire receipt: %+v", op.RetireReceipt)
	}
	time.Sleep(11 * time.Second)
	if v := f.b.readiness(f.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card after retire: %+v", v)
	}
	// The consumed Apply cannot be replayed to bring it back.
	f.tap.setHook(func([]byte) []byte { return body })
	res = f.a.runOnce()
	f.tap.setHook(nil)
	if len(res) != 1 || res[0].Status != "refused" || !portFree(f.port) {
		t.Fatalf("replayed Apply after Retire: %+v", res)
	}
	if err := f.b.store.EnqueueOperationalIngressRetire(f.tn.ID); err == nil {
		t.Fatal("BCC queued a second Retire after the first was answered")
	}
}
