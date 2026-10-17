package bcc

import (
	"encoding/json"
	"github.com/zarkmakerburg/baft/internal/tunnelengine"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTunnelEnginesAPIReadOnlyFailClosed(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/db.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "very-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := app.Handler()
	call := func(method, token string) *httptest.ResponseRecorder {
		t.Helper()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, authReq(method, "/api/tunnel-engines", token, nil))
		return rr
	}
	if r := call(http.MethodPost, "very-secret"); r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", r.Code)
	}
	if r := call(http.MethodGet, "wrong-token"); r.Code == http.StatusOK {
		t.Fatal("catalog exposed to unauthenticated request")
	}
	rr := call(http.MethodGet, "very-secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	var caps []tunnelengine.Capability
	if err := json.Unmarshal(rr.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if len(caps) < 7 {
		t.Fatalf("catalog unexpectedly small: %d", len(caps))
	}
	names := map[string]bool{}
	for _, cap := range caps {
		if names[cap.Name] || cap.Name == "" || cap.Family == "" || cap.Direction == "" || cap.Label == "" {
			t.Fatalf("incomplete/duplicate engine: %+v", cap)
		}
		names[cap.Name] = true
		if cap.Name == "baft" {
			if !cap.Deployable || cap.Family != "direct" || cap.Direction != "ir_to_ex" || cap.ContractVersion != tunnelengine.DriverContractVersion {
				t.Fatalf("native capability incorrect: %+v", cap)
			}
		} else {
			if cap.ContractVersion != 0 {
				t.Fatalf("unimplemented engine advertises a driver contract: %+v", cap)
			}
			if cap.Deployable {
				t.Fatalf("unimplemented engine claimed deployable: %+v", cap)
			}
			if cap.Reason == "" || len(cap.Requirements) == 0 {
				t.Fatalf("planned engine lacks admission requirements: %+v", cap)
			}
			if _, err := tunnelengine.RequireDeployable(cap.Name); err == nil {
				t.Fatalf("catalog/admission mismatch for %s", cap.Name)
			}
		}
	}
	if !names["frp"] || !names["rathole"] || !names["backhaul"] || !names["wireguard"] {
		t.Fatalf("core driver roadmap missing: %v", names)
	}
}
