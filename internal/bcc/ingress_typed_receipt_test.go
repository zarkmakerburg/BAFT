package bcc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

// This is authenticated queue/receipt admission only: candidate evidence is
// data, not a claim that this test created a physical ingress or nft rule.
func TestTypedIngressReceiptTLSAdmissionKeepsWizardPending(t *testing.T) {
	s, spec, c, token := stagedIngressBoundary(t)
	now := time.Now().UTC()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.enqueueReviewedIngress(spec.TunnelID, now, ingressAudit()); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	app.now = func() time.Time { return now }
	endpoint := httptest.NewTLSServer(app.Handler())
	defer endpoint.Close()
	candidate := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: c.JobID, Action: c.Action, Binding: c.Binding, Phase: "applied", OwnershipVerified: true, RollbackStatus: "not_needed", ListenerEvidence: ingressauthority.ListenerEvidenceV1{Endpoint: c.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:456"}, FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{Family: "inet", Table: "baft", Chain: "ingress", ChainHandle: 4, RuleHandle: 5, OwnerTag: ingressauthority.FirewallOwnerTagV1(c.Binding.Port, c.Binding.Proof)}}}
	send := func(r ingressauthority.ReceiptV1) (int, string) {
		t.Helper()
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, endpoint.URL+"/api/ingress/operational-receipt", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := endpoint.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, string(body)
	}
	if code, _ := send(candidate); code != http.StatusConflict {
		t.Fatal("undispatched job accepted positive receipt")
	}
	if _, err := s.pullJobs(c.Binding.NodeID, token, func(j []Job) error { _, err := app.signAgentJobs(j); return err }); err != nil {
		t.Fatal(err)
	}
	invalid := candidate
	invalid.ListenerEvidence.SocketIdentity = "inode:0"
	if code, _ := send(invalid); code != http.StatusConflict {
		t.Fatal("unobserved zero inode accepted")
	}
	if code, body := send(candidate); code != http.StatusAccepted || !strings.Contains(body, `"verified":true`) || !strings.Contains(body, `"ready_to_apply":false`) || !strings.Contains(body, `"reconciliation_pending":false`) {
		t.Fatalf("typed receipt acknowledgement changed: %d %s", code, body)
	}
	// An already recorded exact receipt can recover its lost HTTPS ACK after
	// job expiry; it cannot mutate or execute again, or promote the Wizard.
	app.now = func() time.Time { return c.ExpiresAt.Add(time.Second) }
	if code, body := send(candidate); code != http.StatusAccepted || !strings.Contains(body, `"verified":true`) {
		t.Fatalf("lost acknowledgement not retryable: %d %s", code, body)
	}
	conflicting := candidate
	conflicting.ListenerEvidence.SocketIdentity = "inode:789"
	if code, _ := send(conflicting); code != http.StatusConflict {
		t.Fatal("conflicting receipt replaced durable outcome")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, _ := reopened.GetPublicIngressOperation(spec.TunnelID)
	if op.Status != PublicIngressPending || op.Receipt == nil || op.Receipt.ListenerEvidence != candidate.ListenerEvidence {
		t.Fatal("receipt admission falsely activated config or lost exact evidence")
	}
}
