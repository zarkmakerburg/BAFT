package bcc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

func inspectOperationStore(t *testing.T) (*Store, PublicIngressSpec, PublicIngressExpectedProof) {
	t.Helper()
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 7)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	return s, spec, proof
}

func TestPublicIngressInspectQueuesSignedCursorProofAfterRestart(t *testing.T) {
	s, spec, proof := inspectOperationStore(t)
	s, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.CreatePublicIngressInspect(spec.TunnelID, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if job.Type != agentjob.ActionPublicIngressInspect || job.Params["manifest_digest"] != proof.ManifestDigest || job.Params["generation"] != "7" || job.Params["listener_id"] != proof.ListenerID || job.Params["firewall_id"] != proof.FirewallID {
		t.Fatalf("job escaped cursor: %+v", job)
	}
	if _, err := s.CreatePublicIngressInspect(spec.TunnelID, time.Now(), ingressAudit()); err == nil {
		t.Fatal("duplicate active inspection accepted")
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
	if err != nil || verified.Action != agentjob.ActionPublicIngressInspect || verified.Params["manifest_digest"] != proof.ManifestDigest {
		t.Fatalf("signed intent: %+v %v", verified, err)
	}
	receipt, err := s.PublicIngressInspectReceipt(job.ID)
	if err != nil || !receipt.ReadOnly || receipt.ReadyToApply || len(receipt.Evidence) != 0 {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
}

func TestPublicIngressInspectAPIAuthAndStrictRequest(t *testing.T) {
	s, spec, _ := inspectOperationStore(t)
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
		status      int
	}{
		{"tok-src", `{"tunnel_id":"tun-test"}`, key, 401},
		{"admin", `{"tunnel_id":"tun-test"}`, nil, 503},
		{"admin", `{"tunnel_id":"tun-test","tunnel_id":"tun-test"}`, key, 400},
		{"admin", `{"tunnel_id":"tun-test","apply":true}`, key, 400},
		{"admin", `{"tunnel_id":"tun-test"} {}`, key, 400},
		{"admin", `{"tunnel_id":"tun-test"}`, key, 202},
	} {
		app.ConfigureJobSigning(tc.key)
		r := httptest.NewRequest(http.MethodPost, "/api/ingress/inspect", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, r)
		if rr.Code != tc.status {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.status, rr.Body.String())
		}
		if tc.status == 202 && (!strings.Contains(rr.Body.String(), `"read_only":true`) || !strings.Contains(rr.Body.String(), `"ready_to_apply":false`)) {
			t.Fatal("inspect response claimed apply authority")
		}
	}
	if len(s.ListJobs()) != 1 {
		t.Fatal("rejected requests queued jobs")
	}
	job := s.ListJobs()[0]
	r := httptest.NewRequest(http.MethodGet, "/api/ingress/inspect?id="+job.ID, nil)
	r.Header.Set("Authorization", "Bearer admin")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, r)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"read_only":true`) {
		t.Fatalf("inspect read: %d %s", rr.Code, rr.Body.String())
	}
	if job.Params["tunnel_id"] != spec.TunnelID {
		t.Fatal("wrong tunnel queued")
	}
}

func TestPublicIngressInspectReplacesExpiredJobAtomically(t *testing.T) {
	s, spec, _ := inspectOperationStore(t)
	now := time.Now().UTC()
	old, err := s.CreatePublicIngressInspect(spec.TunnelID, now, ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	old.CreatedAt = now.Add(-6 * time.Minute)
	s.st.Jobs[old.ID] = old
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	path := s.path
	s.path = t.TempDir()
	if _, err := s.CreatePublicIngressInspect(spec.TunnelID, now, ingressAudit()); err == nil {
		t.Fatal("failed replacement save accepted")
	}
	s.path = path
	if s.st.Jobs[old.ID].Status != "queued" {
		t.Fatal("failed save expired old job in memory")
	}
	replacement, err := s.CreatePublicIngressInspect(spec.TunnelID, now, ingressAudit())
	if err != nil || replacement.ID == old.ID {
		t.Fatalf("replacement: %+v %v", replacement, err)
	}
	if err := s.AckJobOutput(spec.IRNode, "tok-src", old.ID, "failed", "late", ""); err == nil {
		t.Fatal("late ACK revived expired inspection")
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.st.Jobs[old.ID].Status != "failed" || s.st.Jobs[replacement.ID].Status != "queued" {
		t.Fatal("replacement did not survive restart")
	}
	found := false
	for _, intent := range s.PendingSecurityAuditIntents() {
		if intent.Action == "public_ingress.inspect.expire" && intent.Details["job_id"] == old.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("replacement has no expiry audit")
	}
}

func TestPublicIngressInspectSessionRequiresCSRF(t *testing.T) {
	rig := newAccessRig(t)
	cookie, _ := rig.session()
	req := httptest.NewRequest(http.MethodPost, rig.base()+"api/ingress/inspect", strings.NewReader(`{"tunnel_id":"tun-test"}`))
	req.AddCookie(cookie)
	if rr := rig.do(req); rr.Code != http.StatusUnauthorized {
		t.Fatalf("write without CSRF status %d", rr.Code)
	}
}
