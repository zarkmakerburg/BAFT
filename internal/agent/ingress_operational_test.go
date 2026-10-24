package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
)

type fakeOperationalExecutor struct{ apply, retire int }

func (f *fakeOperationalExecutor) Apply(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (ingressownership.ObservedIngressV1, error) {
	f.apply++
	return ingressownership.ObservedIngressV1{}, ingressownership.ErrExecutorTrustUnavailable
}

func (f *fakeOperationalExecutor) Retire(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (ingressownership.RetiredIngressEvidence, error) {
	f.retire++
	return ingressownership.RetiredIngressEvidence{}, ingressownership.ErrExecutorTrustUnavailable
}

type operationalHarness struct {
	agent    *Agent
	exec     *fakeOperationalExecutor
	store    *ingressauthority.ReservationStore
	job      agentjob.Job
	mu       sync.Mutex
	receipts []ingressauthority.ReceiptV1
	ack      string
	envelope release.Envelope
}

func newOperationalHarness(t *testing.T, action string, withConfig bool) *operationalHarness {
	t.Helper()
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	h := &operationalHarness{exec: &fakeOperationalExecutor{}, job: validSignedIngressV1(t)}
	h.job.Action = action
	h.envelope, err = agentjob.Sign(key, h.job)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		switch r.URL.Path {
		case "/api/agent/jobs":
			_ = json.NewEncoder(w).Encode([]pulledJob{{ID: h.job.JobID, Signed: &h.envelope}})
		case "/api/ingress/operational-receipt":
			var receipt ingressauthority.ReceiptV1
			if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			h.receipts = append(h.receipts, receipt)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`))
		case "/api/agent/ack":
			var ack map[string]string
			_ = json.NewDecoder(r.Body).Decode(&ack)
			h.ack = ack["Status"]
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := Config{BCCURL: srv.URL, NodeID: h.job.NodeID, Token: "tok", BCCJobKey: pub, StateDir: t.TempDir(), HTTP: srv.Client(), IngressLABExecutor: &fakeIngressLAB{}}
	if withConfig {
		dir := filepath.Join(t.TempDir(), "authority")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		h.store, err = ingressauthority.OpenReservationStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg.IngressOperational = &IngressOperational{Store: h.store, Executor: h.exec}
	}
	h.agent, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Without a provisioned, pinned trusted host policy the operational path must
// stay release-closed: BLOCKED receipt, executor never invoked, no LAB.
func TestOperationalIngressWithoutTrustedPolicyIsBlockedAndNeverExecutes(t *testing.T) {
	for _, action := range []string{agentjob.ActionPublicIngressApply, agentjob.ActionPublicIngressRetire} {
		t.Run(action, func(t *testing.T) {
			h := newOperationalHarness(t, action, true)
			wantCause := "trusted host policy unavailable"
			if action == agentjob.ActionPublicIngressRetire {
				wantCause = "unverified" // unknown owner: no pending claim to hold
			}
			results, err := h.agent.RunOnce(context.Background())
			if err != nil || len(results) != 1 || results[0].Status != "failed" {
				t.Fatalf("expected failed job: %+v %v", results, err)
			}
			if h.exec.apply != 0 || h.exec.retire != 0 {
				t.Fatal("executor reached without trusted policy")
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.receipts) != 1 || h.receipts[0].Phase != "blocked" || h.receipts[0].OwnershipVerified || h.ack != "failed" || !strings.Contains(h.receipts[0].FailureCause, wantCause) {
				t.Fatalf("missing blocked receipt: %+v ack=%s", h.receipts, h.ack)
			}
			cmd, _ := ingressV1Command(h.job)
			if state, err := h.store.InspectRecovery(cmd.Binding); err == nil && state == ingressauthority.StateApplying {
				t.Fatalf("applying recorded without policy: %v", state)
			}
		})
	}
}

// executeFull has no signed envelope, so it can never mint a lease.
func TestOperationalIngressWithoutEnvelopeIsRefused(t *testing.T) {
	h := newOperationalHarness(t, agentjob.ActionPublicIngressApply, true)
	if _, _, err := h.agent.executeFull(context.Background(), h.job); err == nil {
		t.Fatal("v1 job without signed envelope executed")
	}
	if h.exec.apply != 0 {
		t.Fatal("executor invoked without envelope")
	}
}

func TestOutboundIngressReceiptShapes(t *testing.T) {
	base := blockedReceiptFixture(t)
	if !validOutboundIngressReceipt(base) {
		t.Fatal("blocked receipt rejected")
	}
	applied := base
	applied.Phase, applied.FailureCause, applied.RollbackStatus, applied.OwnershipVerified = "applied", "", "", true
	if validOutboundIngressReceipt(applied) {
		t.Fatal("applied without evidence accepted")
	}
	applied.ListenerEvidence = ingressauthority.ListenerEvidenceV1{Endpoint: applied.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:1"}
	applied.FirewallEvidence = []ingressauthority.FirewallEvidenceV1{{Family: "inet", Table: "t", Chain: "c", ChainHandle: 1, RuleHandle: 2, OwnerTag: "x"}}
	if !validOutboundIngressReceipt(applied) {
		t.Fatal("complete applied receipt rejected")
	}
	failed := base
	failed.Phase, failed.RollbackStatus = "failed", "not_attempted"
	if validOutboundIngressReceipt(failed) {
		t.Fatal("failed receipt must declare reconciliation_required")
	}
	retired := base
	retired.Action, retired.Phase, retired.FailureCause, retired.RollbackStatus = agentjob.ActionPublicIngressRetire, "retired", "", ""
	if !validOutboundIngressReceipt(retired) {
		t.Fatal("retired receipt rejected")
	}
	retired.OwnershipVerified = true
	if validOutboundIngressReceipt(retired) {
		t.Fatal("retired may not claim ownership")
	}
}

// Positive wiring (policy pinned, lease minted, executor invoked, applied
// receipt) needs the fixed installed policy, so it only runs in the explicit
// disposable policy LAB, like TestMintIngressLeaseLABPositiveSingleUse.
func TestOperationalIngressLABAppliedFailsClosedAfterExecutorError(t *testing.T) {
	if os.Getenv("BAFT_ISOLATED_POLICY_LAB") != "1" {
		t.Skip("requires disposable Linux root with explicit LAB opt-in")
	}
	sentinel, err := os.ReadFile("/baft-policy-lab")
	if os.Geteuid() != 0 || err != nil || string(sentinel) != "isolated-policy-lab" {
		t.Fatal("refusing outside explicitly prepared disposable root")
	}
	h := newOperationalHarness(t, agentjob.ActionPublicIngressApply, true)
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(context.Background(), h.job.NodeID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll("/etc/baft") })
	results, err := h.agent.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("executor refusal must fail the job: %+v %v", results, err)
	}
	if h.exec.apply != 1 {
		t.Fatalf("executor not invoked exactly once: %d", h.exec.apply)
	}
	cmd, _ := ingressV1Command(h.job)
	if state, err := h.store.InspectRecovery(cmd.Binding); err != nil || state != ingressauthority.StateNeedsReconciliation {
		t.Fatalf("uncertain state not recorded: %v %v", state, err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.receipts) != 1 || h.receipts[0].OwnershipVerified {
		t.Fatalf("bad receipt: %+v", h.receipts)
	}
}
