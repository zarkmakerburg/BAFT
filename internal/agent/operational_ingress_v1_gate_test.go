//go:build linux

package agent

// Release gate operational-ingress-v1: the NORMAL signed Agent path
// (RunOnce: pull, verify, replay-ledger, hold, pin policy, record intent,
// durable APPLYING, mint the single-use lease) admits a reviewed Apply and
// hands the executor a live lease, then reports the outcome over HTTPS. Host
// effects are provided by the executor, which the e2e gate drives for real;
// here the executor is a recording double that also proves the lease is live.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

type gateExecutor struct {
	mu        sync.Mutex
	applies   int
	retires   int
	leaseLive []bool
}

func (g *gateExecutor) Apply(ctx context.Context, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) (ingressownership.ObservedIngressV1, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.applies++
	g.leaseLive = append(g.leaseLive, lease != nil && lease.CheckLockedIngress(ctx, cmd) == nil)
	plan, err := ingressownership.PlanIngressRule(cmd.Binding.Port, cmd.Binding.Proof)
	if err != nil {
		return ingressownership.ObservedIngressV1{}, err
	}
	return ingressownership.ObservedIngressV1{
		Listener: ingressauthority.ListenerEvidenceV1{Endpoint: cmd.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:424242"},
		Firewall: ingressauthority.FirewallEvidenceV1{Family: "inet", Table: "baft", Chain: "ingress", ChainHandle: 3, RuleHandle: 9, OwnerTag: plan.Tag},
		Chain:    3,
	}, nil
}

func (g *gateExecutor) Retire(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (ingressownership.RetiredIngressEvidence, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.retires++
	return ingressownership.RetiredIngressEvidence{}, ingressownership.ErrExecutorTrustUnavailable
}

type gateRig struct {
	t        *testing.T
	agent    *Agent
	exec     *gateExecutor
	store    *ingressauthority.ReservationStore
	job      agentjob.Job
	key      ed25519.PrivateKey
	srv      *httptest.Server
	mu       sync.Mutex
	serve    []pulledJob
	receipts []ingressauthority.ReceiptV1
	acks     map[string]string
}

func newGateRig(t *testing.T) *gateRig {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateRig{t: t, exec: &gateExecutor{}, job: validSignedIngressV1(t), key: key, acks: map[string]string{}}
	g.job.NodeID = "ir-opv1"
	g.job.Params["public_port"] = "443"
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch r.URL.Path {
		case "/api/agent/jobs":
			if r.Header.Get(CapabilityHeader) == "" {
				t.Errorf("the Agent polled without declaring %s although fully provisioned", CapabilityOperationalV1)
			}
			_ = json.NewEncoder(w).Encode(g.serve)
		case "/api/ingress/operational-receipt":
			var rc ingressauthority.ReceiptV1
			if err := json.NewDecoder(r.Body).Decode(&rc); err != nil || r.TLS == nil || r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			g.receipts = append(g.receipts, rc)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`))
		case "/api/agent/ack":
			var ack map[string]string
			_ = json.NewDecoder(r.Body).Decode(&ack)
			g.acks[ack["JobID"]] = ack["Status"]
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.srv.Close)
	dir := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	g.store, err = ingressauthority.OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	g.agent, err = New(Config{BCCURL: g.srv.URL, NodeID: g.job.NodeID, Token: "tok", BCCJobKey: pub, StateDir: t.TempDir(), HTTP: g.srv.Client(),
		IngressOperational: &IngressOperational{Store: g.store, Executor: g.exec}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func (g *gateRig) deliver(env release.Envelope, id string) []Result {
	g.t.Helper()
	g.mu.Lock()
	g.serve = []pulledJob{{ID: id, Signed: &env}}
	g.mu.Unlock()
	res, err := g.agent.RunOnce(context.Background())
	if err != nil {
		g.t.Fatalf("RunOnce: %v", err)
	}
	return res
}

func (g *gateRig) sign(k ed25519.PrivateKey, j agentjob.Job) release.Envelope {
	g.t.Helper()
	env, err := agentjob.Sign(k, j)
	if err != nil {
		g.t.Fatal(err)
	}
	return env
}

func TestOperationalIngressV1Gate_NormalSignedAgentAdmitsReviewedApply(t *testing.T) {
	oplab.Require(t)
	g := newGateRig(t)
	ctx := context.Background()
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, g.job.NodeID); err != nil {
		t.Fatal(err)
	}
	g.job.IssuedAt, g.job.ExpiresAt = time.Now().Add(-time.Second).UTC().Truncate(time.Second), time.Now().Add(time.Minute).UTC().Truncate(time.Second)

	// Everything the Agent must refuse BEFORE the executor is ever called.
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	refuse := func(why string, env release.Envelope, id string) {
		t.Helper()
		res := g.deliver(env, id)
		if len(res) != 1 || res[0].Status == "succeeded" {
			t.Fatalf("%s: admitted: %+v", why, res)
		}
		g.exec.mu.Lock()
		defer g.exec.mu.Unlock()
		if g.exec.applies != 0 || g.exec.retires != 0 {
			t.Fatalf("%s: the executor was reached", why)
		}
	}
	other := g.job
	other.JobID, other.NodeID = "apply-other-node", "ir-other"
	refuse("job for another node", g.sign(g.key, other), other.JobID)
	forged := g.job
	forged.JobID = "apply-forged"
	refuse("job signed by a key the Agent is not enrolled with", g.sign(otherKey, forged), forged.JobID)
	expired := g.job
	expired.JobID, expired.IssuedAt, expired.ExpiresAt = "apply-expired", time.Now().Add(-time.Hour), time.Now().Add(-30*time.Minute)
	refuse("expired job", g.sign(g.key, expired), expired.JobID)
	tampered := g.sign(g.key, g.job)
	tampered.Payload = strings.Replace(tampered.Payload, tampered.Payload[10:14], "AAAA", 1)
	refuse("tampered envelope", tampered, g.job.JobID)
	refuse("wrapper id differing from the signed id", g.sign(g.key, g.job), "some-other-id")
	g.mu.Lock()
	g.serve = []pulledJob{{ID: g.job.JobID}}
	g.mu.Unlock()
	if res, err := g.agent.RunOnce(ctx); err != nil || len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("unsigned job: %+v %v", res, err)
	}
	if left, err := g.store.InspectFencedLeftovers(); err != nil || len(left) != 0 {
		t.Fatalf("refusals reserved state: %+v %v", left, err)
	}

	// The reviewed signed Apply is admitted through the normal poll.
	env := g.sign(g.key, g.job)
	res := g.deliver(env, g.job.JobID)
	if len(res) != 1 || res[0].Status != "succeeded" || res[0].Action != agentjob.ActionPublicIngressApply {
		t.Fatalf("reviewed signed Apply not admitted: %+v", res)
	}
	g.exec.mu.Lock()
	applies, live := g.exec.applies, append([]bool(nil), g.exec.leaseLive...)
	g.exec.mu.Unlock()
	if applies != 1 || len(live) != 1 || !live[0] {
		t.Fatalf("the executor must be called exactly once with a LIVE held lease: applies=%d live=%v", applies, live)
	}
	command, err := ingressV1Command(g.job)
	if err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	receipts, ack := append([]ingressauthority.ReceiptV1(nil), g.receipts...), g.acks[g.job.JobID]
	g.mu.Unlock()
	if ack != "succeeded" || len(receipts) != 1 {
		t.Fatalf("ack=%q receipts=%d", ack, len(receipts))
	}
	r := receipts[0]
	if r.Phase != "applied" || !r.OwnershipVerified || r.JobID != g.job.JobID || r.Binding != command.Binding || r.FailureCause != "" || len(r.FirewallEvidence) != 1 {
		t.Fatalf("applied receipt: %+v", r)
	}
	state, err := g.store.InspectRecovery(command.Binding)
	if err != nil || state != ingressauthority.StateApplying {
		t.Fatalf("durable lifecycle after admission: %v %v", state, err)
	}
	// Single use, durably: the replay is refused and does not reach the executor.
	res = g.deliver(env, g.job.JobID)
	if len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("replay admitted: %+v", res)
	}
	g.exec.mu.Lock()
	defer g.exec.mu.Unlock()
	if g.exec.applies != 1 {
		t.Fatal("replay reached the executor")
	}
}
