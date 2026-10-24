package bcc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

// Exercise the real BCC receiver across a certificate-validated TLS socket.
// This proves authenticated receipt admission only, never operational Apply.
func TestIngressReceiptRealTLSReceiverKeepsUnverifiedEvidencePending(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewTLSServer(app.Handler())
	defer secure.Close()
	plain := httptest.NewServer(app.Handler())
	defer plain.Close()
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "unverified", FailureCause: "held authority unavailable"}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	send := func(client *http.Client, endpoint, credential string) (*http.Response, []byte, error) {
		req, err := http.NewRequest(http.MethodPost, endpoint+"/api/ingress/operational-receipt", bytes.NewReader(raw))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+credential)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-Proto", "https")
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
		return resp, body, err
	}
	before, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	ordinary := &http.Client{Timeout: 5 * time.Second}
	if resp, body, err := send(ordinary, plain.URL, token); err != nil || resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("real plaintext socket trusted forwarded HTTPS: %v %v %s", resp, err, body)
	}
	if _, _, err := send(ordinary, secure.URL, token); err == nil {
		t.Fatal("untrusted BCC certificate accepted")
	}
	client := secure.Client()
	client.Timeout = 5 * time.Second
	if resp, body, err := send(client, secure.URL, "wrong-agent-token"); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("real TLS wrong credential admitted: %v %v %s", resp, err, body)
	}
	refused, err := s.snapshotState()
	if err != nil || !reflect.DeepEqual(before, refused) {
		t.Fatal("refused transport/authentication changed persisted state")
	}
	resp, body, err := send(client, secure.URL, token)
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("enrolled receipt admission failed: %v %v %s", resp, err, body)
	}
	if resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 {
		t.Fatal("positive receiver test did not validate the BCC certificate")
	}
	var result struct {
		Recorded bool `json:"recorded"`
		Verified bool `json:"verified"`
		Ready    bool `json:"ready_to_apply"`
		Pending  bool `json:"reconciliation_pending"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.Recorded || result.Verified || result.Ready || !result.Pending {
		t.Fatalf("inert receipt promoted readiness: %s %v", body, err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, ok := reopened.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Status != PublicIngressPending || op.Command == nil || *op.Command != cmd || op.Receipt == nil || !reflect.DeepEqual(*op.Receipt, receipt) || op.ReviewedPlanHash != cmd.Binding.PlanSHA256 || op.Spec.PublicPort != int(cmd.Binding.ConfigPort) {
		t.Fatal("authenticated receipt lost exact durable reviewed binding or changed readiness")
	}
}

// The job delivery fixture supplies a real signed envelope because normal
// BCC operational enqueue is still disabled. Agent execution and receipt POST
// use production code; this is a refusal-path bridge, not successful Apply.
func TestSignedAgentRefusalReachesActualBCCReceiptReceiver(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	b := cmd.Binding
	params := map[string]string{
		"ingress_protocol": cmd.IngressProtocol, "tunnel_id": b.Spec.TunnelID, "generation": strconv.FormatUint(b.Spec.Generation, 10),
		"public_port": strconv.Itoa(int(b.Port)), "local_target": b.LocalTarget, "manifest_digest": b.Proof.ManifestSHA256,
		"config_sha256": b.Proof.ConfigSHA256, "listen_endpoint": b.ListenEndpoint, "operation_id": b.OperationID,
		"lease_id": b.LeaseID, "plan_sha256": b.PlanSHA256, "transport": b.Transport, "listener_id": b.ListenerID, "firewall_id": b.FirewallID,
	}
	if b.Spec.InstanceID != "" {
		params["instance_id"] = b.Spec.InstanceID
	}
	signed, err := agentjob.Sign(key, agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: cmd.JobID, NodeID: b.NodeID, Action: cmd.Action, IssuedAt: cmd.IssuedAt, ExpiresAt: cmd.ExpiresAt, Params: params})
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	actual := app.Handler()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/jobs" {
			if r.Header.Get("Authorization") != "Bearer "+token || r.URL.Query().Get("node_id") != b.NodeID {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode([]AgentJob{{Job: Job{ID: cmd.JobID, NodeID: b.NodeID}, Signed: &signed}})
			return
		}
		if r.URL.Path == "/api/ingress/operational-receipt" {
			posts.Add(1)
		}
		actual.ServeHTTP(w, r)
	}))
	defer srv.Close()
	cfg := agent.Config{BCCURL: srv.URL, NodeID: b.NodeID, Token: token, BCCJobKey: pub, StateDir: t.TempDir(), HTTP: srv.Client()}
	run := func(want string) {
		t.Helper()
		worker, err := agent.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		results, err := worker.RunOnce(context.Background())
		if err != nil || len(results) != 1 || results[0].Status != want {
			t.Fatalf("real signed Agent result: %+v %v want=%s", results, err, want)
		}
	}
	run("failed")
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, ok := reopened.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Status != PublicIngressPending || op.Receipt == nil || op.Receipt.Binding != cmd.Binding || op.Receipt.JobID != cmd.JobID || op.Receipt.Phase != "blocked" || op.Receipt.OwnershipVerified || op.Receipt.FailureCause == "" || op.Receipt.RollbackStatus != "not_attempted" || len(op.Receipt.FirewallEvidence) != 0 || op.Receipt.ListenerEvidence != (ingressauthority.ListenerEvidenceV1{}) {
		t.Fatalf("actual BCC did not retain a negative exact-bound receipt: %+v", op)
	}
	run("refused")
	if posts.Load() != 1 {
		t.Fatalf("restart replay reissued an operational receipt: %d", posts.Load())
	}
}
