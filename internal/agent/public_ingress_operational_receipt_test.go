package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestOperationalIngressSignedPollReportsBlockedReceiptAndNeverUsesLAB(t *testing.T) {
	j := validSignedIngressV1(t)
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := agentjob.Sign(key, j)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var receipts []ingressauthority.ReceiptV1
	var ackStatus string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer enrolled-test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/agent/jobs":
			_ = json.NewEncoder(w).Encode([]pulledJob{{ID: j.JobID, Signed: &envelope}})
		case "/api/ingress/operational-receipt":
			if r.Method != http.MethodPost || r.TLS == nil {
				t.Error("receipt transport is not real HTTPS POST")
			}
			var receipt ingressauthority.ReceiptV1
			if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			receipts = append(receipts, receipt)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`))
		case "/api/agent/ack":
			var ack map[string]string
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Error(err)
			}
			ackStatus = ack["Status"]
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	lab := &fakeIngressLAB{}
	cfg := Config{BCCURL: srv.URL, NodeID: j.NodeID, Token: "enrolled-test-token", BCCJobKey: pub, StateDir: t.TempDir(), HTTP: srv.Client(), IngressLABExecutor: lab}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	results, err := a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("unsigned success or failed polling: %+v %v", results, err)
	}
	command, err := ingressV1Command(j)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := len(receipts)
	if count != 1 {
		mu.Unlock()
		t.Fatalf("normal signed Agent did not report one blocked receipt: %d", count)
	}
	receipt := receipts[0]
	status := ackStatus
	mu.Unlock()
	if status != "failed" || receipt.SchemaVersion != 1 || receipt.JobID != j.JobID || receipt.Action != j.Action || receipt.Binding != command.Binding || receipt.Phase != "blocked" || receipt.OwnershipVerified || receipt.FailureCause == "" || len(receipt.FirewallEvidence) != 0 || receipt.ListenerEvidence != (ingressauthority.ListenerEvidenceV1{}) {
		t.Fatalf("refusal misreported as operational evidence: %+v ack=%s", receipt, status)
	}
	if lab.apply != 0 || lab.retire != 0 {
		t.Fatal("operational v1 reached LAB executor")
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	again, err := restarted.RunOnce(context.Background())
	if err != nil || len(again) != 1 || again[0].Status != "refused" {
		t.Fatalf("replayed blocked job was not fenced: %+v %v", again, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(receipts) != 1 {
		t.Fatal("Agent restart replayed the blocked operational receipt")
	}
}

func blockedReceiptFixture(t *testing.T) ingressauthority.ReceiptV1 {
	t.Helper()
	j := validSignedIngressV1(t)
	cmd, err := ingressV1Command(j)
	if err != nil {
		t.Fatal(err)
	}
	return ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: j.JobID, Action: j.Action, Binding: cmd.Binding, Phase: "blocked", FailureCause: "authority unavailable", RollbackStatus: "not_attempted"}
}

type ingressReceiptTestTransport func(*http.Request) (*http.Response, error)

func (f ingressReceiptTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBlockedIngressReceiptRefusesUntrustedTransportsBeforeSendingToken(t *testing.T) {
	for _, kind := range []string{"plaintext", "insecure-certificate", "custom-roundtripper", "untrusted-certificate"} {
		t.Run(kind, func(t *testing.T) {
			var requests atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusAccepted) })
			var srv *httptest.Server
			if kind == "plaintext" {
				srv = httptest.NewServer(handler)
			} else {
				srv = httptest.NewTLSServer(handler)
			}
			defer srv.Close()
			client := srv.Client()
			if kind == "insecure-certificate" {
				transport := client.Transport.(*http.Transport).Clone()
				transport.TLSClientConfig.InsecureSkipVerify = true
				client = &http.Client{Transport: transport}
			}
			if kind == "custom-roundtripper" {
				client = &http.Client{Transport: ingressReceiptTestTransport(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					return nil, errors.New("token reached untrusted transport")
				})}
			}
			if kind == "untrusted-certificate" {
				client = &http.Client{Timeout: time.Second}
			}
			receipt := blockedReceiptFixture(t)
			a := &Agent{cfg: Config{BCCURL: srv.URL, NodeID: receipt.Binding.NodeID, Token: "private-agent-token", HTTP: client}}
			if err := a.postBlockedIngressReceipt(context.Background(), receipt); err == nil {
				t.Fatal("untrusted receipt transport accepted")
			}
			if requests.Load() != 0 {
				t.Fatal("credential sent through a rejected transport")
			}
		})
	}
}

func TestBlockedIngressReceiptNeverFollowsRedirect(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	receipt := blockedReceiptFixture(t)
	a := &Agent{cfg: Config{BCCURL: origin.URL, NodeID: receipt.Binding.NodeID, Token: "private-agent-token", HTTP: origin.Client()}}
	if err := a.postBlockedIngressReceipt(context.Background(), receipt); err == nil || redirected.Load() != 0 {
		t.Fatalf("receipt followed a redirect: err=%v requests=%d", err, redirected.Load())
	}
}

func TestBlockedIngressReceiptAckRequiresExactUnverifiedPendingObject(t *testing.T) {
	good := `{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`
	if err := validateBlockedIngressReceiptAck([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		good + ` {}`, `null`, strings.Replace(good, `"verified":false`, `"verified":true`, 1),
		strings.Replace(good, `"ready_to_apply":false`, `"ready_to_apply":true`, 1),
		strings.Replace(good, `"verified":false,`, "", 1), strings.Replace(good, `"verified":false`, `"verified":null`, 1),
		strings.Replace(good, `"verified":false`, `"verified":false,"verified":false`, 1),
		strings.Replace(good, `"verified":false`, `"Verified":false`, 1),
		strings.Replace(good, `"recorded":true`, `"recorded":false`, 1),
		strings.Replace(good, `"reconciliation_pending":true`, `"reconciliation_pending":false`, 1),
	} {
		if err := validateBlockedIngressReceiptAck([]byte(bad)); err == nil {
			t.Fatalf("ambiguous or promoted receipt acknowledgement accepted: %s", bad)
		}
	}
}

func TestBlockedIngressReceiptAckReadsNPlusOneBytes(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(strconv.Itoa(extra), func(t *testing.T) {
			ack := `{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`
			ack += strings.Repeat(" ", maxIngressReceiptBytes-len(ack)+extra)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, ack)
			}))
			defer srv.Close()
			receipt := blockedReceiptFixture(t)
			a := &Agent{cfg: Config{BCCURL: srv.URL, NodeID: receipt.Binding.NodeID, Token: "enrolled-token", HTTP: srv.Client()}}
			err := a.postBlockedIngressReceipt(context.Background(), receipt)
			if (err != nil) != (extra == 1) {
				t.Fatalf("receipt ACK bound failure extra=%d err=%v", extra, err)
			}
		})
	}
}
