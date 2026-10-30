package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

// Exercise the real signed pull/dispatch/ACK and durable seen-job path. The
// executor is a test double: kernel ownership has its own namespace acceptance.
func TestPublicIngressLABPollingReplaySurvivesAgentRestart(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	manifestDir := t.TempDir()
	if err := os.Chmod(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var queued []pulledJob
	var acknowledgements []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer polling-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/agent/jobs":
			if r.Method != http.MethodGet || r.URL.Query().Get("node_id") != j.NodeID {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(queued)
		case "/api/agent/ack":
			var ack map[string]string
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Errorf("ACK decode: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			acknowledgements = append(acknowledgements, ack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	executor := &fakeIngressLAB{}
	cfg := Config{BCCURL: srv.URL, NodeID: j.NodeID, Token: "polling-token", BCCJobKey: pub, StateDir: t.TempDir(), IngressManifestDir: manifestDir, IngressLABExecutor: executor}
	newAgent := func() *Agent {
		t.Helper()
		a, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	poll := func(a *Agent, job agentjob.Job, expected string) {
		t.Helper()
		envelope, err := agentjob.Sign(key, job)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		queued = []pulledJob{{ID: job.JobID, Signed: &envelope}}
		mu.Unlock()
		results, err := a.RunOnce(context.Background())
		if err != nil || len(results) != 1 || results[0].Status != expected || results[0].JobID != job.JobID {
			t.Fatalf("%s poll got=%+v err=%v want=%s", job.Action, results, err, expected)
		}
	}
	a := newAgent()
	standby := newAgent() // Another process can start before any job is seen.
	poll(a, j, "succeeded")
	if executor.apply != 0 || executor.retire != 0 {
		t.Fatal("prepare mutated live executor")
	}
	manifest, err := ingressmanifest.Open(manifestDir)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := manifest.Inspect(proof)
	_ = manifest.Close()
	if err != nil || !inspection.PreparedBytesMatch || inspection.Manifest.Phase != ingressmanifest.PhasePrepared {
		t.Fatalf("prepared manifest: %+v %v", inspection, err)
	}
	j.Action, j.JobID = agentjob.ActionPublicIngressApply, "polling-apply"
	poll(a, j, "succeeded")
	mu.Lock()
	var receipt publicIngressLABReceipt
	err = json.Unmarshal([]byte(acknowledgements[len(acknowledgements)-1]["Output"]), &receipt)
	mu.Unlock()
	if err != nil || receipt.JobID != j.JobID || receipt.Proof != proof || !receipt.LiveOwnershipVerified {
		t.Fatalf("apply ACK: %+v %v", receipt, err)
	}
	poll(a, j, "refused")
	poll(standby, j, "refused")
	a = newAgent() // Fresh Agent object must reload the actual seen-jobs file.
	poll(a, j, "refused")
	if executor.apply != 1 {
		t.Fatalf("replayed Apply invoked executor %d times", executor.apply)
	}
	j.Action, j.JobID = agentjob.ActionPublicIngressRetire, "polling-retire"
	poll(a, j, "succeeded")
	poll(newAgent(), j, "refused")
	if executor.retire != 1 {
		t.Fatalf("replayed Retire invoked executor %d times", executor.retire)
	}
	if !a.isSeen("polling-apply") || !a.isSeen("polling-retire") {
		t.Fatal("durable seen jobs were lost")
	}
	var stored map[string]json.RawMessage
	raw, err := os.ReadFile(a.seenPath())
	if err != nil || json.Unmarshal(raw, &stored) != nil || len(stored) != 3 {
		t.Fatalf("durable replay journal: %s %v", raw, err)
	}
}
