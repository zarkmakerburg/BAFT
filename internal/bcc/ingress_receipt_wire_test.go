package bcc

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

func TestOperationalReceiptRejectsAmbiguousWireWithoutRecording(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) string
	}{
		{"duplicate-job", func(s string) string { return strings.Replace(s, `"job_id":`, `"job_id":"ignored","job_id":`, 1) }},
		{"escaped-duplicate-job", func(s string) string { return strings.Replace(s, `"job_id":`, `"job_\u0069d":"ignored","job_id":`, 1) }},
		{"case-alias-job", func(s string) string { return strings.Replace(s, `"job_id":`, `"Job_ID":`, 1) }},
		{"duplicate-config-port", func(s string) string {
			return strings.Replace(s, `"config_port":`, `"config_port":22,"config_port":`, 1)
		}},
		{"case-alias-node", func(s string) string { return strings.Replace(s, `"node_id":`, `"Node_ID":`, 1) }},
		{"null-ownership", func(s string) string {
			return strings.Replace(s, `"ownership_verified":false`, `"ownership_verified":null`, 1)
		}},
		{"null-listener", func(s string) string {
			return strings.Replace(s, `"listener_evidence":{"endpoint":"","family":"","socket_identity":""}`, `"listener_evidence":null`, 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, spec, cmd, token := stagedIngressBoundary(t)
			app, err := NewServer(s, "admin")
			if err != nil {
				t.Fatal(err)
			}
			receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "blocked", RollbackStatus: "not_attempted", FailureCause: "authority unavailable"}
			raw, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			body := tc.mutate(string(raw))
			if body == string(raw) {
				t.Fatal("test did not mutate wire")
			}
			req := httptest.NewRequest(http.MethodPost, "https://bcc.example/api/ingress/operational-receipt", strings.NewReader(body))
			req.TLS = &tls.ConnectionState{}
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()
			app.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("ambiguous receipt status=%d body=%s", rr.Code, rr.Body.String())
			}
			op, ok := s.GetPublicIngressOperation(spec.TunnelID)
			if !ok || op.Receipt != nil {
				t.Fatal("ambiguous receipt changed stored operation")
			}
		})
	}
}
