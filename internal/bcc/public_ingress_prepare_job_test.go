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

func TestIngressPrepareJobIsSignedAndAckBoundToCursor(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	job, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit()); err == nil {
		t.Fatal("duplicate prepare queued")
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
	jobs, err := s.PullJobs(spec.IRNode, "tok-src")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pull: %+v %v", jobs, err)
	}
	signed, err := app.signAgentJobs(jobs)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: spec.IRNode}).Verify(*signed[0].Signed, time.Now())
	if err != nil || verified.Action != agentjob.ActionPublicIngressPrepare || verified.Params["manifest_digest"] != proof.ManifestDigest {
		t.Fatalf("signed prepare: %+v %v", verified, err)
	}
	if !verified.ExpiresAt.Equal(publicIngressPreflightDeadline(job).UTC().Truncate(time.Second)) {
		t.Fatalf("prepare signed lifetime escaped persisted deadline: %s", verified.ExpiresAt)
	}
	manifest, err := ingressmanifest.Preview(publicIngressManifestSpec(spec, "", 1))
	if err != nil {
		t.Fatal(err)
	}
	receipt := publicIngressPrepareEvidence{JobID: job.ID, NodeID: job.NodeID, Proof: manifest, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID, PreparedBytesMatch: true}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"prepared_bytes_match":true`, `"prepared_bytes_match":false`, 1)
	if err := s.AckJobOutput(spec.IRNode, "tok-src", job.ID, "succeeded", "prepared", bad); err == nil {
		t.Fatal("unprepared receipt accepted")
	}
	if s.st.Jobs[job.ID].Status != "dispatched" {
		t.Fatal("invalid receipt completed job")
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", job.ID, "succeeded", "prepared", string(raw)); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil || reopened.st.Jobs[job.ID].Status != "succeeded" || reopened.st.Jobs[job.ID].Output != string(raw) {
		t.Fatalf("prepare proof not durable: %v", err)
	}
	if reopened.st.PublicIngressOperations[spec.TunnelID].Status != PublicIngressPending {
		t.Fatal("prepared bytes incorrectly marked live ingress applied")
	}
	result, err := reopened.PublicIngressPrepareReceipt(job.ID)
	if err != nil || !result.PreparedOnly || result.ReadyToApply || string(result.Evidence) != string(raw) {
		t.Fatalf("validated prepare receipt not readable: %+v %v", result, err)
	}
}

func TestIngressPrepareAPIRequiresAdminAndStrictRequest(t *testing.T) {
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
	for _, tc := range []struct {
		token, body string
		key         []byte
		want        int
	}{
		{"tok-src", `{"tunnel_id":"tun-test"}`, key, 401},
		{"admin", `{"tunnel_id":"tun-test"}`, nil, 503},
		{"admin", `{"tunnel_id":"tun-test","apply":true}`, key, 400},
		{"admin", `{"tunnel_id":"tun-test"}`, key, 202},
	} {
		app.ConfigureJobSigning(tc.key)
		r := httptest.NewRequest(http.MethodPost, "/api/ingress/prepare", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, r)
		if rr.Code != tc.want {
			t.Fatalf("prepare HTTP status %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
		}
		if tc.want == 202 && (!strings.Contains(rr.Body.String(), `"prepared_only":true`) || !strings.Contains(rr.Body.String(), `"ready_to_apply":false`)) {
			t.Fatal("prepare API claimed live readiness")
		}
	}
	if len(s.ListJobs()) != 1 {
		t.Fatal("rejected requests queued prepare jobs")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/ingress/prepare?id="+s.ListJobs()[0].ID, nil)
	r.Header.Set("Authorization", "Bearer admin")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, r)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"prepared_only":true`) {
		t.Fatalf("prepare receipt HTTP: %d %s", rr.Code, rr.Body.String())
	}
}

func TestIngressPrepareSessionRequiresCSRF(t *testing.T) {
	rig := newAccessRig(t)
	cookie, _ := rig.session()
	r := httptest.NewRequest(http.MethodPost, rig.base()+"api/ingress/prepare", strings.NewReader(`{"tunnel_id":"tun-test"}`))
	r.AddCookie(cookie)
	if rr := rig.do(r); rr.Code != http.StatusUnauthorized {
		t.Fatalf("prepare without CSRF: %d", rr.Code)
	}
}

func TestIngressPrepareExpiredPollAndAckFailClosed(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	job, err := s.CreatePublicIngressPrepare(spec.TunnelID, time.Now(), ingressAudit())
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
	if err != nil || len(jobs) != 0 || s.st.Jobs[job.ID].Status != "failed" {
		t.Fatalf("expired prepare delivered: %+v %v", jobs, err)
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", job.ID, "succeeded", "late", "{}"); err == nil {
		t.Fatal("late prepare ACK accepted")
	}
	found := false
	for _, intent := range s.PendingSecurityAuditIntents() {
		if intent.Action == "public_ingress.prepare.expire" && intent.Details["job_id"] == job.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expired prepare lacks audit")
	}
}
