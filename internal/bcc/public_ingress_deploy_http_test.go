package bcc

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

func operationalDeployFixture(t *testing.T) (*Server, *Store, PublicIngressSpec, string, ed25519.PublicKey) {
	t.Helper()
	s, spec := reservationStore(t)
	now := time.Now().UTC()
	token := "operational-deploy-enrolled-agent"
	s.mu.Lock()
	tunnel := s.st.Tunnels[spec.TunnelID]
	tunnel.Phase = TunnelActive
	tunnel.PlanHash = strings.Repeat("a", 64)
	tunnel.ClientPort = spec.PublicPort
	tunnel.RouteListen = spec.LocalTarget
	if tunnel.InstanceID == "" {
		tunnel.InstanceID = "instance-operational-deploy"
	}
	s.st.Tunnels[tunnel.ID] = tunnel
	node := s.st.Nodes[spec.IRNode]
	node.AgentTokenHash = tokenHash(token)
	node.AgentSeen = now
	s.st.Nodes[node.ID] = node
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	return app, s, spec, token, pub
}

func postIngressOperation(t *testing.T, endpoint *httptest.Server, action string, spec PublicIngressSpec, planHash, adminToken string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"tunnel_id": spec.TunnelID, "plan_hash": planHash})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/ingress/deploy"
	if action == "retire" {
		path = "/api/ingress/retire"
	}
	req, err := http.NewRequest(http.MethodPost, endpoint.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := endpoint.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPublicIngressDeployAPIQueuesExactSignedReviewedCommand(t *testing.T) {
	app, s, spec, _, pub := operationalDeployFixture(t)
	endpoint := httptest.NewTLSServer(app.Handler())
	defer endpoint.Close()
	tunnel, ok := s.GetTunnel(spec.TunnelID)
	if !ok {
		t.Fatal("active tunnel fixture missing")
	}

	resp := postIngressOperation(t, endpoint, "deploy", spec, tunnel.PlanHash, "admin")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("deploy status=%d", resp.StatusCode)
	}
	var first publicIngressOperationResponse
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if first.TunnelID != spec.TunnelID || first.OperationID == "" || first.JobID == "" ||
		first.Action != agentjob.ActionPublicIngressApply || first.DispatchPhase != "queued" ||
		first.ReadyToApply || !first.ReconciliationPending {
		t.Fatalf("unsafe or incomplete deploy response: %+v", first)
	}
	op, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != "queued" || op.Receipt != nil {
		t.Fatalf("reviewed command not durably queued without false readiness: %+v", op)
	}
	s.mu.Lock()
	job, exists := s.st.Jobs[first.JobID]
	s.mu.Unlock()
	if !exists || job.Status != "queued" || job.IngressCommand == nil || *job.IngressCommand != *op.Command {
		t.Fatalf("queue changed signed command identity: %+v", job)
	}
	signed, err := app.signAgentJobs([]Job{job})
	if err != nil || len(signed) != 1 || signed[0].Signed == nil {
		t.Fatalf("could not sign operational command: %+v %v", signed, err)
	}
	decoded, err := (agentjob.Verifier{BCCKey: pub, NodeID: spec.IRNode}).Verify(*signed[0].Signed, app.now())
	if err != nil {
		t.Fatal(err)
	}
	command, err := ingressauthority.CommandFromJob(decoded)
	if err != nil || command != *op.Command {
		t.Fatalf("signed poll changed reviewed command: %+v %v", command, err)
	}

	// An exact retry returns the same operation and JobID, without duplicate queue rows.
	resp2 := postIngressOperation(t, endpoint, "deploy", spec, tunnel.PlanHash, "admin")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("idempotent deploy status=%d", resp2.StatusCode)
	}
	var second publicIngressOperationResponse
	if err := json.NewDecoder(resp2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.OperationID != first.OperationID || second.JobID != first.JobID {
		t.Fatalf("retry replaced immutable operation: first=%+v second=%+v", first, second)
	}
	s.mu.Lock()
	jobCount := len(s.st.Jobs)
	s.mu.Unlock()
	if jobCount != 1 {
		t.Fatalf("retry duplicated queued job: %d", jobCount)
	}
}

func TestPublicIngressDeployAPIRejectsMalformedAndUnreviewedPlan(t *testing.T) {
	app, s, spec, _, _ := operationalDeployFixture(t)
	endpoint := httptest.NewTLSServer(app.Handler())
	defer endpoint.Close()
	tunnel, _ := s.GetTunnel(spec.TunnelID)
	before := stateJSONForTest(t, s)

	for _, body := range []string{
		`{"tunnel_id":"` + spec.TunnelID + `","plan_hash":"` + tunnel.PlanHash + `","surprise":true}`,
		`{"tunnel_id":"` + spec.TunnelID + `","tunnel_id":"` + spec.TunnelID + `","plan_hash":"` + tunnel.PlanHash + `"}`,
		`{"tunnel_id":"` + spec.TunnelID + `","plan_hash":"wrong"}`,
	} {
		req, err := http.NewRequest(http.MethodPost, endpoint.URL+"/api/ingress/deploy", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin")
		resp, err := endpoint.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("malformed/unreviewed request accepted: status=%d body=%s", resp.StatusCode, body)
		}
	}
	if after := stateJSONForTest(t, s); string(after) != string(before) {
		t.Fatal("invalid deploy request mutated durable state")
	}
}
