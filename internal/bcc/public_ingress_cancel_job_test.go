package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestIngressCancelIsSignedAndFencesLatePrepareAcknowledgement(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	prepare, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePublicIngressCancel(spec.TunnelID, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.PullJobs(spec.IRNode, "tok-src")
	if err != nil || len(jobs) != 2 {
		t.Fatalf("pull: %+v %v", jobs, err)
	}
	var cancel Job
	for _, j := range jobs {
		if j.Type == agentjob.ActionPublicIngressCancel {
			cancel = j
		}
	}
	if cancel.ID == "" {
		t.Fatal("cancel job not dispatched")
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
	signed, err := app.signAgentJobs([]Job{cancel})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: spec.IRNode}).Verify(*signed[0].Signed, time.Now())
	if err != nil || verified.Action != agentjob.ActionPublicIngressCancel || verified.Params["manifest_digest"] != proof.ManifestDigest {
		t.Fatalf("signed cancellation: %+v %v", verified, err)
	}
	if !verified.ExpiresAt.Equal(publicIngressPreflightDeadline(cancel).UTC().Truncate(time.Second)) {
		t.Fatalf("cancel signed lifetime escaped persisted deadline: %s", verified.ExpiresAt)
	}
	manifestSpec := publicIngressManifestSpec(spec, "", 1)
	tombstone, err := ingressmanifest.PreviewCancellation(manifestSpec)
	if err != nil {
		t.Fatal(err)
	}
	receipt := publicIngressCancelEvidence{JobID: cancel.ID, NodeID: cancel.NodeID, Proof: tombstone, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), tombstone.ManifestSHA256, strings.Repeat("a", 64), 1)
	if err := s.AckJobOutput(spec.IRNode, "tok-src", cancel.ID, "succeeded", "cancelled", bad); err == nil {
		t.Fatal("wrong tombstone receipt accepted")
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", cancel.ID, "succeeded", "cancelled", string(raw)); err != nil {
		t.Fatal(err)
	}
	prepared, err := ingressmanifest.Preview(manifestSpec)
	if err != nil {
		t.Fatal(err)
	}
	prepareReceipt, err := json.Marshal(publicIngressPrepareEvidence{JobID: prepare.ID, NodeID: prepare.NodeID, Proof: prepared, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID, PreparedBytesMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", prepare.ID, "succeeded", "late", string(prepareReceipt)); err == nil {
		t.Fatal("late prepare ACK revived cancelled generation")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.PublicIngressCancelReceipt(cancel.ID)
	if err != nil || got.Status != "succeeded" || !got.ManifestOnly || got.ReadyToApply || string(got.Evidence) != string(raw) {
		t.Fatalf("cancel receipt: %+v %v", got, err)
	}
}

func TestIngressCancelAPIRequiresAdminAndIssuedPrepare(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
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
	call := func(token, body string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/ingress/cancel", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, r)
		return rr.Code
	}
	if code := call("tok-src", `{"tunnel_id":"tun-test"}`); code != 401 {
		t.Fatalf("agent issued cancellation: %d", code)
	}
	if code := call("admin", `{"tunnel_id":"tun-test"}`); code != 400 {
		t.Fatalf("unissued prepare cancelled: %d", code)
	}
	if _, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	if code := call("admin", `{"tunnel_id":"tun-test","apply":true}`); code != 400 {
		t.Fatalf("unsafe payload accepted: %d", code)
	}
	if code := call("admin", `{"tunnel_id":"tun-test"}`); code != 202 {
		t.Fatalf("valid cancellation refused: %d", code)
	}
	if code := call("admin", `{"tunnel_id":"tun-test"}`); code != 400 {
		t.Fatalf("duplicate cancellation issued: %d", code)
	}
}

func TestIngressCancelExpiredDispatchAndLateAckFailClosed(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	job, err := s.CreatePublicIngressCancel(spec.TunnelID, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	job.CreatedAt = time.Now().Add(-6 * time.Minute)
	s.st.Jobs[job.ID] = job
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := s.PullJobs(spec.IRNode, "tok-src")
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ID == job.ID {
			t.Fatal("expired cancellation dispatched")
		}
	}
	if s.st.Jobs[job.ID].Status != "failed" {
		t.Fatal("expired cancellation remained live")
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", job.ID, "succeeded", "late", "{}"); err == nil {
		t.Fatal("late cancellation ACK accepted")
	}
	found := false
	for _, intent := range s.PendingSecurityAuditIntents() {
		if intent.Action == "public_ingress.cancel.expire" && intent.Details["job_id"] == job.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expired cancellation lacks audit")
	}
}
