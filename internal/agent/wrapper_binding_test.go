package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestSignedJobWrapperIDMustMatchBeforeExecution(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	job := agentjob.Job{
		SchemaVersion: agentjob.SchemaVersion, JobID: "signed-job", NodeID: "node-a",
		Action: agentjob.ActionRestart, IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	envelope, err := agentjob.Sign(key, job)
	if err != nil {
		t.Fatal(err)
	}
	wrapperID := "unrelated-job"
	var acknowledgements []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agent/jobs":
			_ = json.NewEncoder(w).Encode([]pulledJob{{ID: wrapperID, Signed: &envelope}})
		case "/api/agent/ack":
			var ack struct{ JobID string }
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Errorf("decode ACK: %v", err)
			}
			acknowledgements = append(acknowledgements, ack.JobID)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	system := &fakeSystem{}
	a, err := New(Config{BCCURL: srv.URL, NodeID: job.NodeID, Token: "test-token", BCCJobKey: pub, StateDir: t.TempDir(), System: system})
	if err != nil {
		t.Fatal(err)
	}
	results, err := a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "refused" || !strings.Contains(results[0].Detail, "wrapper ID") {
		t.Fatalf("mismatched wrapper: %+v %v", results, err)
	}
	if a.isSeen(job.JobID) || a.isSeen(wrapperID) || len(system.calls) != 0 || len(acknowledgements) != 0 {
		t.Fatalf("mismatched wrapper changed state: seen=%v/%v calls=%v ACKs=%v", a.isSeen(job.JobID), a.isSeen(wrapperID), system.calls, acknowledgements)
	}

	// The same signed payload must still be runnable when the transport
	// wrapper is corrected; refusal must not consume its replay allowance.
	wrapperID = job.JobID
	results, err = a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" {
		t.Fatalf("corrected wrapper: %+v %v", results, err)
	}
	if !a.isSeen(job.JobID) || len(system.calls) == 0 || len(acknowledgements) != 1 || acknowledgements[0] != job.JobID {
		t.Fatalf("corrected wrapper did not execute exactly once: calls=%v ACKs=%v", system.calls, acknowledgements)
	}
}
