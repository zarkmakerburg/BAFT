package bcc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
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
