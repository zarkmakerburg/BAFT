package node

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/verification"
)

func TestVerificationEndpointReportsDeliveryWithoutPayloadOrAuthorityFromCounters(t *testing.T) {
	r := NewRuntime()
	r.verificationDigest, r.verificationSupported = strings.Repeat("a", 64), true
	r.ingressBytes.Add(512)
	r.observeDelivery(256)
	w := httptest.NewRecorder()
	r.managementMetricsHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/verification", nil))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("proof endpoint response is cacheable or unavailable")
	}
	var p verification.RuntimeReport
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.ReceivedBytes != 512 || p.DeliveredBytes != 256 || p.LastDeliveredAt.IsZero() || p.AuthenticatedPeers != 0 {
		t.Fatal("counters falsely claimed current authenticated authority")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"version": true, "config_digest": true, "boot_id": true, "supported": true, "generated_at": true, "authenticated_peers": true, "received_bytes": true, "delivered_bytes": true, "last_delivered_at": true}
	for field := range fields {
		if !allowed[field] {
			t.Fatalf("unexpected proof field %s", field)
		}
	}
	w = httptest.NewRecorder()
	r.managementMetricsHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/verification", nil))
	if w.Code == http.StatusOK {
		t.Fatal("mutation method accepted by proof endpoint")
	}
}
