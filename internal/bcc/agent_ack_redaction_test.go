package bcc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ADR-001 §5 defense in depth: a failure ACK from an agent that does not
// redact (pre-v0.5.0) must not persist secrets in BCC's job store, while a
// succeeded ACK keeps its message and Output (the pairing channel) intact.
func TestAgentFailureACKIsRedactedBeforeStorage(t *testing.T) {
	s := newPathProbeStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	p := startOnePathProbe(t, s, 22001)
	secrets := []string{"s3cret-value-123", "abcdefgh12345678", "MC4CAQAwBQYDK2VwBCIEIleak", "BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA"}
	msg := "path probe listen failed: password=" + secrets[0] + " Authorization: Bearer " + secrets[1] +
		"\n-----BEGIN PRIVATE KEY-----\n" + secrets[2] + "\n-----END PRIVATE KEY-----\n" + secrets[3]
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, authReq(http.MethodPost, "/api/agent/ack", "tok-dst", map[string]string{
		"NodeID": "dst", "JobID": p.JobID, "Status": "failed", "Message": msg,
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("failure ack=%d %s", rec.Code, rec.Body.String())
	}
	var stored *Job
	for _, j := range s.ListJobs() {
		if j.ID == p.JobID {
			j := j
			stored = &j
		}
	}
	if stored == nil || stored.Status != "failed" {
		t.Fatalf("job not recorded as failed: %+v", stored)
	}
	for _, secret := range secrets {
		if strings.Contains(stored.Message, secret) {
			t.Fatalf("BCC stored secret %q in failure message: %q", secret, stored.Message)
		}
	}
	if !strings.Contains(stored.Message, "path probe listen failed") {
		t.Fatalf("redaction removed the diagnostic context: %q", stored.Message)
	}
}
