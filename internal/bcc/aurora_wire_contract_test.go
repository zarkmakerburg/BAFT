package bcc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These fixtures are selected wire fields, not a complete response or a new API
// version. Extra object fields are allowed; consumed values and array scope are
// checked against the real in-process handlers before frontend decoding in Node.
func auroraWireJSON(t *testing.T, raw []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func auroraWireSubset(t *testing.T, want, got any, path string) {
	t.Helper()
	switch expected := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected object, got %T", path, got)
		}
		for key, value := range expected {
			field, exists := actual[key]
			if !exists {
				t.Fatalf("%s.%s: consumed wire field missing", path, key)
			}
			auroraWireSubset(t, value, field, path+"."+key)
		}
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(expected) {
			t.Fatalf("%s: expected array of length %d, got %T", path, len(expected), got)
		}
		for i, value := range expected {
			auroraWireSubset(t, value, actual[i], path+"[]")
		}
	default:
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%s: got %v, want %v", path, got, want)
		}
	}
}

func TestAuroraReadWireContract(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "synthetic-contract-admin")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.January, 1, 12, 0, 0, 123456789, time.UTC)
	app.now = func() time.Time { return at }
	handler := app.Handler()
	check := func(endpoint, fixture string) {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		request.Header.Set("Authorization", "Bearer synthetic-contract-admin")
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: status=%d content-type=%q", endpoint, response.Code, response.Header().Get("Content-Type"))
		}
		golden, err := os.ReadFile(filepath.Join("..", "..", "web", "aurora", "tests", "wire", fixture))
		if err != nil {
			t.Fatal(err)
		}
		auroraWireSubset(t, auroraWireJSON(t, golden), auroraWireJSON(t, response.Body.Bytes()), endpoint)
	}
	check("/api/overview", "empty-overview.json")
	check("/api/tunnels", "empty-tunnels.json")

	// Synthetic state only. No deployment, job creation or external connection.
	store.mu.Lock()
	store.st.Nodes["demo-ir-1"] = Node{ID: "demo-ir-1", Alias: "DEMO Iran", Role: "worker", Country: "IR", Health: "up", LastChecked: at, AgentSeen: at}
	store.st.Nodes["demo-ex-1"] = Node{ID: "demo-ex-1", Alias: "DEMO Exit", Role: "foreign", Country: "DE", Revoked: true}
	store.st.Telemetry["demo-ir-1"] = TelemetryCursor{NodeID: "demo-ir-1", LastTelemetry: at}
	store.st.History["demo-ir-1"] = []HistoryPoint{{Timestamp: at.Add(-time.Minute)}, {Timestamp: at}}
	store.st.Tunnels = map[string]Tunnel{}
	store.st.Tunnels["demo-scoped"] = Tunnel{ID: "demo-scoped", InstanceID: "demo-slot-1", IRNode: "demo-ir-1", EXNode: "demo-ex-1", Phase: TunnelActive, Port: 18443, CreatedAt: at.Add(-time.Minute), UpdatedAt: at}
	store.st.Tunnels["demo-legacy"] = Tunnel{ID: "demo-legacy", IRNode: "demo-ir-1", EXNode: "demo-ex-1", Phase: TunnelActive, Port: 19443, CreatedAt: at.Add(-2 * time.Minute), UpdatedAt: at}
	store.mu.Unlock()
	before := store.ListTunnels()
	check("/api/overview", "overview.json")
	check("/api/tunnels", "tunnels.json")
	if !reflect.DeepEqual(before, store.ListTunnels()) || len(store.ListJobs()) != 0 {
		t.Fatal("read handlers changed tunnel intent or queued operational jobs")
	}
	for _, endpoint := range []string{"/api/overview", "/api/tunnels"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "demo-ir-1") {
			t.Fatalf("%s: unauthenticated read status=%d", endpoint, response.Code)
		}
	}
}
