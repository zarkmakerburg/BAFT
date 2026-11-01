package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

// ADR-001 acceptance case: "Logs/receipts include crafted secret values in
// errors → secrets redacted in all paths". These tests craft secrets into the
// diagnostic text the Agent produces and assert that none of them reaches BCC
// (ACK body, operational receipt) or the returned Result used for logs.

const redactionTestToken = "agent-bearer-0123456789abcdef"

const redactionTestPEM = "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIBc8leakedKeyMaterial\n-----END PRIVATE KEY-----"

var redactionTestSecrets = []string{redactionTestToken, "BCIEIBc8leakedKeyMaterial", "BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA", "hunter22pass"}

func craftedSecretText() string {
	return "doctor failed; token=" + redactionTestToken + "\n" + redactionTestPEM +
		"\npair BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA via https://ops:hunter22pass@bcc.example/api"
}

type redactionTestSystem struct{}

func (redactionTestSystem) Systemctl(context.Context, ...string) (string, error) {
	return "", errors.New("systemctl unavailable in test")
}

func (redactionTestSystem) Run(_ context.Context, name string, args ...string) (string, error) {
	if filepath.Base(name) == "baft" && len(args) > 0 && args[0] == "doctor" {
		return craftedSecretText(), errors.New("exit status 1")
	}
	return "", errors.New("not available in test")
}

func assertNoRedactionTestSecret(t *testing.T, where, s string) {
	t.Helper()
	for _, secret := range redactionTestSecrets {
		if strings.Contains(s, secret) {
			t.Fatalf("%s leaked %q: %q", where, secret, s)
		}
	}
}

func TestFailedJobACKAndResultAreRedacted(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	env, err := agentjob.Sign(key, agentjob.Job{JobID: "job-redact-1", NodeID: "node-redact", Action: agentjob.ActionHealth, IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var acks []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agent/jobs":
			_ = json.NewEncoder(w).Encode([]pulledJob{{ID: "job-redact-1", Signed: &env}})
		case "/api/agent/ack":
			var ack map[string]string
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Error(err)
			}
			mu.Lock()
			acks = append(acks, ack)
			mu.Unlock()
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a, err := New(Config{BCCURL: srv.URL, NodeID: "node-redact", Token: redactionTestToken, BCCJobKey: pub,
		StateDir: t.TempDir(), BinDir: t.TempDir(), HTTP: srv.Client(), System: redactionTestSystem{}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	results, err := a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("health job did not fail as crafted: %+v %v", results, err)
	}
	assertNoRedactionTestSecret(t, "Result.Detail", results[0].Detail)
	if !strings.Contains(results[0].Detail, "baft doctor") || !strings.Contains(results[0].Detail, "[redacted") {
		t.Fatalf("diagnostic lost its non-secret context: %q", results[0].Detail)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(acks) != 1 || acks[0]["Status"] != "failed" {
		t.Fatalf("expected one failed ACK, got %+v", acks)
	}
	for field, value := range acks[0] {
		assertNoRedactionTestSecret(t, "ACK."+field, value)
	}
	if len(acks[0]["Message"]) > maxFailureMessage {
		t.Fatalf("failure ACK exceeds bound: %d", len(acks[0]["Message"]))
	}
}

// Regression for CI run 41229286650 on e156fd7f: redacting the ACK Output
// field stripped the pairing/reply code BCC needs to pair IR with EX, so
// tunnels stalled in preparing_ir. Output is the authenticated one-time
// channel for those codes and must reach BCC byte-for-byte on success.
func TestSucceededACKOutputCarriesPairingCodeUnchanged(t *testing.T) {
	const code = "BAFTPAIR1:QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo"
	var mu sync.Mutex
	var acks []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ack map[string]string
		if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
			t.Error(err)
		}
		mu.Lock()
		acks = append(acks, ack)
		mu.Unlock()
	}))
	defer srv.Close()
	a := &Agent{cfg: Config{BCCURL: srv.URL, NodeID: "ex-1", Token: redactionTestToken, HTTP: srv.Client()}}
	if err := a.ack(context.Background(), "job-pair", "succeeded", "pairing code issued", code); err != nil {
		t.Fatal(err)
	}
	if err := a.ack(context.Background(), "job-fail", "failed", "baft-pair failed near "+code+" token="+redactionTestToken, ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(acks) != 2 {
		t.Fatalf("acks: %d", len(acks))
	}
	if acks[0]["Output"] != code || acks[0]["Message"] != "pairing code issued" || acks[0]["Status"] != "succeeded" {
		t.Fatalf("succeeded ACK altered the pairing channel: %+v", acks[0])
	}
	if strings.Contains(acks[1]["Message"], code) || strings.Contains(acks[1]["Message"], redactionTestToken) || acks[1]["Output"] != "" {
		t.Fatalf("failure ACK leaked a secret: %+v", acks[1])
	}
}

func TestOperationalReceiptFailureCauseIsRedactedAndDeliverable(t *testing.T) {
	// A long executor error used to push the receipt past
	// maxIngressReceiptBytes, so BCC received no failure evidence at all.
	receipt := blockedReceiptFixture(t)
	receipt.FailureCause = "operational ingress refused before host effects: " + craftedSecretText() + strings.Repeat(" nft: rule dump", 2000)
	var mu sync.Mutex
	var got []ingressauthority.ReceiptV1
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ingress/operational-receipt" {
			http.NotFound(w, r)
			return
		}
		var in ingressauthority.ReceiptV1
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		got = append(got, in)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`))
	}))
	defer srv.Close()
	a := &Agent{cfg: Config{BCCURL: srv.URL, NodeID: receipt.Binding.NodeID, Token: redactionTestToken, HTTP: srv.Client()}}
	if err := a.postBlockedIngressReceipt(context.Background(), receipt); err != nil {
		t.Fatalf("redacted, bounded failure receipt was not delivered: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("receipts delivered: %d", len(got))
	}
	cause := got[0].FailureCause
	assertNoRedactionTestSecret(t, "receipt.FailureCause", cause)
	if cause == "" || len(cause) > maxIngressFailureCause || got[0].Phase != "blocked" || got[0].RollbackStatus != "not_attempted" {
		t.Fatalf("receipt shape changed: phase=%q rollback=%q causeLen=%d", got[0].Phase, got[0].RollbackStatus, len(cause))
	}
	if !strings.HasPrefix(cause, "operational ingress refused before host effects") {
		t.Fatalf("failure context lost: %q", cause)
	}
}
