package bcc

import (
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestPublicIngressPreflightCannotRenewSignedLifetime(t *testing.T) {
	s, req := ingressPreflightStore(t)
	created := time.Now().UTC().Add(-4 * time.Minute)
	job, err := s.CreatePublicIngressPreflight(req, created, ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	now := time.Now().UTC()
	app.now = func() time.Time { return now }
	jobs, err := s.PullJobs(req.IRNode, "tok-src")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pull: %+v %v", jobs, err)
	}
	signed, err := app.signAgentJobs(jobs)
	if err != nil {
		t.Fatal(err)
	}
	verifier := agentjob.Verifier{BCCKey: pub, NodeID: req.IRNode}
	got, err := verifier.Verify(*signed[0].Signed, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExpiresAt.Equal(publicIngressPreflightDeadline(job).UTC().Truncate(time.Second)) {
		t.Fatalf("signed expiry %s differs from persisted deadline %s", got.ExpiresAt, publicIngressPreflightDeadline(job))
	}
	if _, err := verifier.Verify(*signed[0].Signed, publicIngressPreflightDeadline(job)); err == nil {
		t.Fatal("agent accepted preflight after its persisted lifetime")
	}
	app.now = func() time.Time { return publicIngressPreflightDeadline(job).Add(time.Second) }
	if _, err := app.signAgentJobs(jobs); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired job was re-signed: %v", err)
	}
}

func TestPublicIngressPreflightExpiresBeforeLatePullAndAck(t *testing.T) {
	s, req := ingressPreflightStore(t)
	old := time.Now().UTC().Add(-publicIngressPreflightPendingLifetime - time.Minute)
	job, err := s.CreatePublicIngressPreflight(req, time.Now().UTC(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	job.CreatedAt = old
	s.st.Jobs[job.ID] = job
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := s.PullJobs(req.IRNode, "tok-src")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("expired pull: %+v %v", jobs, err)
	}
	if got := s.st.Jobs[job.ID]; got.Status != "failed" || got.Message == "" {
		t.Fatalf("expired job state: %+v", got)
	}
	reopened, err := OpenStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.Jobs[job.ID].Status != "failed" {
		t.Fatal("expiration did not survive restart")
	}

	current, err := reopened.CreatePublicIngressPreflight(req, time.Now().UTC(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	// A delivered job can still be running when its immutable deadline passes.
	reopened.mu.Lock()
	j := reopened.st.Jobs[current.ID]
	j.Status = "dispatched"
	j.CreatedAt = old
	reopened.st.Jobs[j.ID] = j
	reopened.mu.Unlock()
	if err := reopened.AckJobOutput(req.IRNode, "tok-src", j.ID, "succeeded", "late", publicIngressReceiptFixture(req)); err == nil {
		t.Fatal("late success changed an expired preflight")
	}
	if reopened.st.Jobs[j.ID].Status != "dispatched" {
		t.Fatal("late acknowledgement changed job state")
	}
}
