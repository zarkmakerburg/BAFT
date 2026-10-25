package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A succeeded job's structured report (tunnel_discover with a few units is
// over 2000 bytes) must reach BCC whole; a failure message stays capped.
func TestAckKeepsLargeSucceededReportWhole(t *testing.T) {
	var got struct{ Message string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	a := &Agent{cfg: Config{BCCURL: srv.URL, NodeID: "n", Token: "t", HTTP: srv.Client()}}
	report := `{"version":1,"pad":"` + strings.Repeat("x", 5000) + `"}`
	if err := a.ack(context.Background(), "job-1", "succeeded", report, ""); err != nil {
		t.Fatal(err)
	}
	if got.Message != report || !json.Valid([]byte(got.Message)) {
		t.Fatalf("succeeded report was cut: %d of %d bytes", len(got.Message), len(report))
	}
	if err := a.ack(context.Background(), "job-2", "failed", strings.Repeat("e", 5000), ""); err != nil {
		t.Fatal(err)
	}
	if len(got.Message) != maxFailureMessage {
		t.Fatalf("failure message not capped: %d", len(got.Message))
	}
}
