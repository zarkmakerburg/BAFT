//go:build linux

package operational

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// ---------- shared helpers for the negative scenarios ----------

// deliver makes the real BCC-facing poll return exactly this signed job (signed
// with the BCC key: the strongest forger, a compromised signer or a replaying
// network attacker) and runs the real Agent once.
func (f *fullPath) deliver(t *testing.T, j agentjob.Job) []agent.Result {
	t.Helper()
	env := f.b.sign(j)
	f.tap.setHook(serve(signedJob{jobIDOf(env), env}))
	defer f.tap.setHook(nil)
	return f.a.runOnce()
}

// legit performs the full honest apply and waits for the verified card.
func (f *fullPath) legit(t *testing.T) {
	t.Helper()
	f.apply(t)
	if v := f.waitReady(t); !v.Ready || v.Card == nil {
		t.Fatalf("honest apply did not reach a verified card: %+v probe=%+v", v, v.Probe)
	}
	if err := roundTrip(t, fmt.Sprintf("127.0.0.1:%d", f.port), []byte("honest")); err != nil {
		t.Fatalf("honest ingress not serving: %v", err)
	}
}

// snap captures everything a refused request must leave unchanged.
func (f *fullPath) snap(t *testing.T) string {
	t.Helper()
	op := f.b.op(f.tn.ID)
	raw, _ := json.Marshal(struct {
		Rules    string
		Receipt  any
		Dispatch any
		Retire   any
	}{f.h.chainJSON(), op.Receipt, op.Dispatch, op.RetireCommand})
	left, err := f.a.Store.InspectFencedLeftovers()
	if err != nil {
		t.Fatal(err)
	}
	l, _ := json.Marshal(left)
	return string(raw) + string(l)
}

func (f *fullPath) mustStillServe(t *testing.T) {
	t.Helper()
	if err := roundTrip(t, fmt.Sprintf("127.0.0.1:%d", f.port), []byte("still-serving")); err != nil {
		t.Fatalf("the honest ingress was disturbed: %v", err)
	}
	if v := f.b.readiness(f.tn.ID); !v.Ready || v.Card == nil {
		// readiness is cached 10s; wait once for a fresh probe.
		if v = f.waitReady(t); !v.Ready {
			t.Fatalf("verified card lost: %+v", v)
		}
	}
}

// noEffects asserts a refusal happened before any host or BCC effect.
func (f *fullPath) noEffects(t *testing.T) {
	t.Helper()
	if n := f.h.baftRules(); n != 0 {
		t.Fatalf("refused request left %d owned nft rules: %s", n, f.h.chainJSON())
	}
	if !portFree(f.port) {
		t.Fatalf("refused request left a listener on %d", f.port)
	}
	left, err := f.a.Store.InspectFencedLeftovers()
	if err != nil || len(left) != 0 {
		t.Fatalf("refused request left reservation state: %+v %v", left, err)
	}
	if op := f.b.op(f.tn.ID); op.Receipt != nil {
		t.Fatalf("BCC recorded a receipt for a refused request: %+v", op.Receipt)
	}
	if v := f.b.readiness(f.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card for a refused request: %+v", v)
	}
}

func (b *bccRig) postReceipt(token string, r ingressauthority.ReceiptV1) int {
	b.t.Helper()
	raw, _ := json.Marshal(r)
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/ingress/operational-receipt", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.cl.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func isFailure(res []agent.Result) bool {
	return len(res) == 1 && (res[0].Status == "failed" || res[0].Status == "refused")
}

// ---------- wrong node ----------

func TestOperationalIngressV1Gate_RefusesWrongNode(t *testing.T) {
	f := setupFullPath(t, 54711, "127.0.0.1:1443")
	f.cmd = f.b.stageFor(f.tn, "apply-wrong-node", 2*time.Minute)
	f.a.openGate()
	// A correctly signed job addressed to a different node.
	res := f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) { j.NodeID = "ir-other" }))
	if len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("mis-addressed job was not refused: %+v", res)
	}
	f.noEffects(t)
	// BCC never delivers one node's operational job to another node.
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatal(err)
	}
	if jobs := f.b.pull("ex-opv1"); len(jobs) != 0 {
		t.Fatalf("the IR job was delivered to the EX node: %+v", jobs)
	}
	// ...and the EX node's enrolled credential cannot speak for the IR binding.
	rec := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: f.cmd.JobID, Action: f.cmd.Action, Binding: f.cmd.Binding, Phase: "blocked",
		FailureCause: "x", RollbackStatus: "not_attempted"}
	if st := f.b.postReceipt(nodeToken("ex-opv1"), rec); st != http.StatusUnauthorized {
		t.Fatalf("another node's credential recorded a receipt: %d", st)
	}
	f.noEffects(t)
	// The honest job still applies afterwards: nothing was poisoned.
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("honest job after the refusals: %+v", res)
	}
	if v := f.waitReady(t); !v.Ready {
		t.Fatalf("not ready: %+v", v)
	}
}

// ---------- wrong plan hash ----------

func TestOperationalIngressV1Gate_RefusesWrongPlanHash(t *testing.T) {
	f := setupFullPath(t, 54712, "127.0.0.1:1443")
	f.legit(t)
	before := f.snap(t)
	other := strings.Repeat("e", 64)
	res := f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) { j.JobID = "forged-plan"; j.Params["plan_sha256"] = other }))
	if !isFailure(res) {
		t.Fatalf("a job with another plan hash was applied: %+v", res)
	}
	if after := f.snap(t); after != before {
		t.Fatalf("plan-hash drift changed host/BCC state:\nbefore=%s\nafter=%s", before, after)
	}
	f.mustStillServe(t)
	// BCC side: a drifted binding can neither be staged nor reserved, nor can
	// its receipt be recorded.
	drift := f.cmd
	drift.Binding.PlanSHA256 = other
	if err := f.b.store.StageReviewedIngressCommand(f.tn.ID, drift, time.Now()); err == nil {
		t.Fatal("BCC staged a command for another plan hash")
	}
	f.b.reserve(f.tn, other, f.port, http.StatusBadRequest)
	rec := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: f.cmd.JobID, Action: f.cmd.Action, Binding: drift.Binding, Phase: "blocked",
		FailureCause: "x", RollbackStatus: "not_attempted"}
	if st := f.b.postReceipt(nodeToken(irNode), rec); st == http.StatusAccepted {
		t.Fatal("BCC recorded a receipt bound to another plan hash")
	}
	if after := f.snap(t); after != before {
		t.Fatal("refused BCC calls changed state")
	}
}

// ---------- wrong port ----------

func TestOperationalIngressV1Gate_RefusesWrongPort(t *testing.T) {
	f := setupFullPath(t, 54713, "127.0.0.1:1443")
	f.cmd = f.b.stageFor(f.tn, "apply-wrong-port", 2*time.Minute)
	f.a.openGate()
	for name, mutate := range map[string]func(j *agentjob.Job){
		"public_port differs from listen_endpoint": func(j *agentjob.Job) { j.JobID = "wp-1"; j.Params["public_port"] = "54999" },
		"SSH port 22": func(j *agentjob.Job) {
			j.JobID = "wp-2"
			j.Params["public_port"], j.Params["listen_endpoint"] = "22", "0.0.0.0:22"
			reDigest(j, f.cmd.Binding.Spec.InstanceID)
		},
		"listen on a different port than the digests": func(j *agentjob.Job) {
			j.JobID = "wp-3"
			j.Params["listen_endpoint"] = "0.0.0.0:54998"
			j.Params["public_port"] = "54998"
		},
		"non-canonical endpoint": func(j *agentjob.Job) { j.JobID = "wp-4"; j.Params["listen_endpoint"] = "0.0.0.0:054713" },
	} {
		if res := f.deliver(t, forgedJob(f.cmd, mutate)); !isFailure(res) {
			t.Fatalf("%s: applied: %+v", name, res)
		}
		f.noEffects(t)
		for _, p := range []int{22, 54999, 54998} {
			if listening(fmt.Sprintf("127.0.0.1:%d", p)) {
				t.Fatalf("%s: a listener appeared on %d", name, p)
			}
		}
	}
	// BCC refuses the same drift: command, reservation and receipt.
	drift := f.cmd
	drift.Binding.Port++
	if err := f.b.store.StageReviewedIngressCommand(f.tn.ID, drift, time.Now()); err == nil {
		t.Fatal("BCC staged a command whose public port differs from the reviewed client port")
	}
	drift = f.cmd
	drift.Binding.ConfigPort++
	if err := f.b.store.StageReviewedIngressCommand(f.tn.ID, drift, time.Now()); err == nil {
		t.Fatal("BCC staged a command whose config port differs from its public port")
	}
	f.b.reserve(f.tn, f.plan.Hash, f.port+1, http.StatusBadRequest)
	f.b.reserve(f.tn, f.plan.Hash, 22, http.StatusBadRequest)
	f.noEffects(t)
	// The reviewed signed job carries exactly the client port, and applies.
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatal(err)
	}
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("reviewed job: %+v", res)
	}
	if v := f.waitReady(t); !v.Ready || int(v.Card.PublicPort) != f.port || int(v.Card.ConfigPort) != f.port {
		t.Fatalf("card port: %+v", v)
	}
}

// ---------- wrong lease ----------

func TestOperationalIngressV1Gate_RefusesWrongLease(t *testing.T) {
	f := setupFullPath(t, 54714, "127.0.0.1:1443")
	f.legit(t)
	before := f.snap(t)
	res := f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) { j.JobID = "forged-lease"; j.Params["lease_id"] = "lease-forged" }))
	if !isFailure(res) {
		t.Fatalf("a job with another lease id was applied: %+v", res)
	}
	if after := f.snap(t); after != before {
		t.Fatalf("lease drift changed state:\nbefore=%s\nafter=%s", before, after)
	}
	// The executor itself is unusable without the live held lease of THIS
	// exact command: no lease, a zero lease, and no lease for another binding.
	ctx := context.Background()
	if _, err := f.a.Exec.Apply(ctx, f.cmd, nil); err == nil {
		t.Fatal("executor applied without a lease")
	}
	if _, err := f.a.Exec.Apply(ctx, f.cmd, &ingressauthority.Lease{}); err == nil {
		t.Fatal("executor applied with a zero lease")
	}
	retire := f.cmd
	retire.Action, retire.JobID = agentjob.ActionPublicIngressRetire, "retire-no-lease"
	if _, err := f.a.Exec.Retire(ctx, retire, nil); err == nil {
		t.Fatal("executor retired without a lease")
	}
	if _, err := f.a.Exec.Retire(ctx, retire, &ingressauthority.Lease{}); err == nil {
		t.Fatal("executor retired with a zero lease")
	}
	// BCC: a receipt for another lease is not recorded.
	drift := f.cmd
	drift.Binding.LeaseID = "lease-forged"
	rec := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: f.cmd.JobID, Action: f.cmd.Action, Binding: drift.Binding, Phase: "blocked",
		FailureCause: "x", RollbackStatus: "not_attempted"}
	if st := f.b.postReceipt(nodeToken(irNode), rec); st == http.StatusAccepted {
		t.Fatal("BCC recorded a receipt for another lease")
	}
	if err := f.b.store.StageReviewedIngressCommand(f.tn.ID, drift, time.Now()); err == nil {
		t.Fatal("BCC staged a command for another lease")
	}
	if after := f.snap(t); after != before {
		t.Fatal("refused calls changed state")
	}
	f.mustStillServe(t)
}

// ---------- wrong generation ----------

func TestOperationalIngressV1Gate_RefusesWrongGeneration(t *testing.T) {
	f := setupFullPath(t, 54715, "127.0.0.1:1443")
	f.legit(t)
	before := f.snap(t)
	// Internally inconsistent generation (digests still those of generation 1).
	res := f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) { j.JobID = "gen-1"; j.Params["generation"] = "2" }))
	if !isFailure(res) {
		t.Fatalf("inconsistent generation applied: %+v", res)
	}
	// Fully consistent generation 2 (digests recomputed): only the fenced
	// owner identity can refuse it.
	res = f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) {
		j.JobID = "gen-2"
		j.Params["generation"] = "2"
		reDigest(j, f.cmd.Binding.Spec.InstanceID)
	}))
	if !isFailure(res) {
		t.Fatalf("a newer generation replaced the fenced owner: %+v", res)
	}
	// Generation 0 and a rollback are invalid too.
	for i, g := range []string{"0", "18446744073709551616", "x"} {
		res = f.deliver(t, forgedJob(f.cmd, func(j *agentjob.Job) { j.JobID = fmt.Sprintf("gen-bad-%d", i); j.Params["generation"] = g }))
		if !isFailure(res) {
			t.Fatalf("generation %q applied: %+v", g, res)
		}
	}
	if after := f.snap(t); after != before {
		t.Fatalf("generation drift changed state:\nbefore=%s\nafter=%s", before, after)
	}
	drift := f.cmd
	drift.Binding.Spec.Generation = 2
	if err := f.b.store.StageReviewedIngressCommand(f.tn.ID, drift, time.Now()); err == nil {
		t.Fatal("BCC staged a command for another generation")
	}
	f.mustStillServe(t)
}

// ---------- wrong enrollment ----------

func TestOperationalIngressV1Gate_RefusesWrongEnrollment(t *testing.T) {
	f := setupFullPath(t, 54716, "127.0.0.1:1443")
	f.cmd = f.b.stageFor(f.tn, "apply-wrong-enrollment", 2*time.Minute)
	closed := func(what string) {
		t.Helper()
		if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); !errors.Is(err, bcc.ErrIngressAuthorityUnavailable) {
			t.Fatalf("%s: BCC opened the enqueue gate: %v", what, err)
		}
		for _, j := range f.b.store.ListJobs() {
			if j.Type == agentjob.ActionPublicIngressApply {
				t.Fatalf("%s: an operational job was queued: %+v", what, j)
			}
		}
	}
	closed("never contacted")
	// 1. wrong credential / another node's credential / unknown node: the
	//    poll is refused so the capability is never recorded.
	for name, o := range map[string]*agentOpt{
		"wrong token":     {token: "not-the-enrolled-token", client: f.b.cl},
		"EX node's token": {token: nodeToken("ex-opv1"), client: f.b.cl},
		"unenrolled node": {node: "ir-unenrolled", token: "tok-ir-unenrolled", client: f.b.cl},
	} {
		w := startAgent(t, f.b, f.h, t.TempDir(), o)
		if _, err := w.Agent.RunOnce(w.proc); err == nil {
			t.Fatalf("%s: poll succeeded", name)
		}
		closed(name)
		w.kill()
	}
	// 2. an enrolled Agent opens the gate; revoking the node closes it again,
	//    stops job delivery and receipt acceptance.
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatalf("enrolled capable agent did not open the gate: %v", err)
	}
	if _, err := f.b.store.RevokeNode(irNode, "gate test", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Agent.RunOnce(f.a.proc); err == nil {
		t.Fatal("revoked node could still poll")
	}
	rec := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: f.cmd.JobID, Action: f.cmd.Action, Binding: f.cmd.Binding, Phase: "blocked",
		FailureCause: "x", RollbackStatus: "not_attempted"}
	if st := f.b.postReceipt(nodeToken(irNode), rec); st != http.StatusUnauthorized {
		t.Fatalf("revoked node recorded a receipt: %d", st)
	}
	f.noEffects(t)
}

func TestOperationalIngressV1Gate_RefusesWrongEnrollmentPinnedSigner(t *testing.T) {
	// (Not in the manifest; kept separate so a failure names the cause.)
	f := setupFullPath(t, 54717, "127.0.0.1:1443")
	f.cmd = f.b.stageFor(f.tn, "apply-wrong-signer", 2*time.Minute)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	imposter := startAgent(t, f.b, f.h, t.TempDir(), &agentOpt{key: otherPub, url: f.tap.srv.URL, client: f.b.cl})
	imposter.openGate()
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatal(err)
	}
	res := imposter.runOnce()
	if len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("an Agent pinned to another BCC key ran the job: %+v", res)
	}
	f.noEffects(t)
}

// ---------- wrong TLS identity ----------

func TestOperationalIngressV1Gate_RefusesWrongTLSIdentity(t *testing.T) {
	f := setupFullPath(t, 54718, "127.0.0.1:1443")
	f.cmd = f.b.stageFor(f.tn, "apply-wrong-tls", 2*time.Minute)
	// 1. An Agent that does not trust BCC's CA cannot poll, so BCC never
	//    learns a capability and the gate stays closed.
	noTrust := startAgent(t, f.b, f.h, t.TempDir(), &agentOpt{client: &http.Client{Timeout: 5 * time.Second}})
	if _, err := noTrust.Agent.RunOnce(noTrust.proc); err == nil {
		t.Fatal("poll succeeded without trusting the BCC certificate")
	}
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); !errors.Is(err, bcc.ErrIngressAuthorityUnavailable) {
		t.Fatalf("gate opened without an authenticated poll: %v", err)
	}
	// 2. An impostor presenting a certificate from another CA at the
	//    configured address: the handshake fails and the enrolled credential
	//    is never transmitted.
	var hits atomic.Int32
	evil := newPKI(t, "evil-ca")
	impostor := evil.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	for _, url := range []string{impostor.URL} {
		a := startAgent(t, f.b, f.h, t.TempDir(), &agentOpt{url: url, client: f.b.cl})
		if _, err := a.Agent.RunOnce(a.proc); err == nil {
			t.Fatal("agent accepted an impostor BCC certificate")
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("the enrolled credential reached an untrusted TLS peer (%d requests)", hits.Load())
	}
	// 3. A certificate that is valid for the CA but not for the name used
	//    (localhost vs the 127.0.0.1 IP SAN) is refused too.
	host := strings.Replace(f.b.srv.URL, "127.0.0.1", "localhost", 1)
	byName := startAgent(t, f.b, f.h, t.TempDir(), &agentOpt{url: host, client: f.b.cl})
	if _, err := byName.Agent.RunOnce(byName.proc); err == nil {
		t.Fatal("agent accepted a certificate that does not match the BCC name")
	}
	// 4. The BCC receiver itself refuses non-TLS delivery of a receipt even
	//    when it claims to be behind an HTTPS proxy.
	plain := httptest.NewServer(f.b.app.Handler())
	defer plain.Close()
	rec := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: f.cmd.JobID, Action: f.cmd.Action, Binding: f.cmd.Binding, Phase: "blocked",
		FailureCause: "x", RollbackStatus: "not_attempted"}
	raw, _ := json.Marshal(rec)
	req, _ := http.NewRequest(http.MethodPost, plain.URL+"/api/ingress/operational-receipt", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+nodeToken(irNode))
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plaintext receipt not refused: %d", resp.StatusCode)
	}
	f.noEffects(t)
	// 5. Over the right identity everything works.
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatal(err)
	}
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("apply over the genuine BCC identity: %+v", res)
	}
}
