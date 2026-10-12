package bcc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocationOnlyUsesPublicLiteralIPs(t *testing.T) {
	for _, value := range []string{"127.0.0.1:22", "10.0.0.1:22", "172.16.0.1", "192.168.1.1", "100.87.219.44:22", "0.1.2.3", "169.254.1.2", "192.0.2.1", "198.51.100.2", "203.0.113.3", "198.18.1.1", "240.0.0.1", "224.0.0.1", "[::1]:22", "fc00::1", "fe80::1%eth0", "2001:db8::1", "example.com:22", "https://8.8.8.8"} {
		if ip := locationIP(value); ip != "" {
			t.Errorf("%q leaked as %q", value, ip)
		}
	}
	for value, want := range map[string]string{"8.8.8.8:22": "8.8.8.8", "::ffff:8.8.8.8": "8.8.8.8", "[2001:4860:4860::8888]:2222": "2001:4860:4860::8888"} {
		if got := locationIP(value); got != want {
			t.Errorf("%s: got %s want %s", value, got, want)
		}
	}
	if got := nodeLocationIP(Node{Address: "100.87.219.44:22", PathIPv4: "8.8.8.8"}); got != "8.8.8.8" {
		t.Fatal(got)
	}
	if got := nodeLocationIP(Node{Address: "1.1.1.1:22", PathIPv4: "8.8.8.8"}); got != "1.1.1.1" {
		t.Fatal("entered public IP must take precedence", got)
	}
}

func TestCountryServiceValidatesResponseAndSendsOnlyIP(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"valid", `{"ip":"8.8.8.8","country":"de"}`, 200, "DE"},
		{"different IP", `{"ip":"1.1.1.1","country":"DE"}`, 200, ""},
		{"missing country", `{"ip":"8.8.8.8"}`, 200, ""},
		{"unsupported country", `{"ip":"8.8.8.8","country":"ZZ"}`, 200, ""},
		{"invalid json", "<html>error</html>", 200, ""},
		{"oversized", strings.Repeat(" ", 4097), 200, ""},
		{"rate limited", "", 429, ""},
		{"redirect", "", 302, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/8.8.8.8" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			code, err := countryLookup(server.Client(), server.URL)(context.Background(), "8.8.8.8")
			if code != tc.want || (tc.want == "") != (err != nil) {
				t.Fatalf("country=%q err=%v", code, err)
			}
		})
	}
}

func locationRig(t *testing.T) (*Server, *Store) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpsertNode(Node{ID: "n", Alias: "server", Address: "8.8.8.8:22", Role: "foreign"}, ""); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return app, store
}

func TestAutomaticCountryPreservesOverridesAcrossUpdatesAndRestart(t *testing.T) {
	app, store := locationRig(t)
	now := time.Now().UTC()
	if changed, err := store.SetNodeCountryFromIP("n", "8.8.8.8", "DE", now); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	n := app.Overview(now, time.Hour).Nodes[0]
	if n.Country != "DE" || n.CountrySource != "ip" || n.Lat == nil || n.Lon == nil || !n.LocationApproximate {
		t.Fatalf("missing map location: %+v", n)
	}
	if _, err := store.SetNodeCountry("n", "NL", now); err != nil {
		t.Fatal(err)
	}
	if changed, err := store.SetNodeCountryFromIP("n", "8.8.8.8", "DE", now); err != nil || changed {
		t.Fatalf("overwrote manual choice: changed=%v err=%v", changed, err)
	}
	if _, err := store.UpsertNode(Node{ID: "n", Address: "1.1.1.1:22", Role: "foreign"}, ""); err != nil {
		t.Fatal(err)
	}
	manual, _ := store.GetNode("n")
	if manual.Country != "NL" || manual.CountrySource != "manual" {
		t.Fatalf("lost override: %+v", manual)
	}
	if _, err := store.SetNodeCountry("n", "", now); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	hidden, _ := reopened.GetNode("n")
	if automaticCountry(hidden) {
		t.Fatal("explicit no-location became automatic after restart")
	}
	if _, err := reopened.ResetNodeCountry("n", now, AuditEntry{Actor: "admin"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := reopened.SetNodeCountryFromIP("n", "1.1.1.1", "DE", now); err != nil || !changed {
		t.Fatalf("automatic reset: changed=%v err=%v", changed, err)
	}
	if _, err := reopened.UpsertNode(Node{ID: "n", Address: "8.8.4.4:22", Role: "foreign"}, ""); err != nil {
		t.Fatal(err)
	}
	changedNode, _ := reopened.GetNode("n")
	if changedNode.Country != "" || !automaticCountry(changedNode) {
		t.Fatalf("stale inferred location: %+v", changedNode)
	}
	if changed, err := reopened.SetNodeCountryFromIP("n", "1.1.1.1", "DE", now); err != nil || changed {
		t.Fatalf("accepted stale IP: changed=%v err=%v", changed, err)
	}
}

func TestLegacyCountryRemainsManual(t *testing.T) {
	_, store := locationRig(t)
	if _, err := store.UpsertNode(Node{ID: "n", Address: "8.8.8.8:22", Role: "foreign", Country: "IR"}, ""); err != nil {
		t.Fatal(err)
	}
	if changed, err := store.SetNodeCountryFromIP("n", "8.8.8.8", "DE", time.Now()); err != nil || changed {
		t.Fatalf("legacy country overwritten: %v %v", changed, err)
	}
}

func TestCountryWriteFailureRollsBackLocationAndAudit(t *testing.T) {
	_, store := locationRig(t)
	path := store.Path()
	blocked := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	store.path = blocked
	if changed, err := store.SetNodeCountryFromIP("n", "8.8.8.8", "DE", time.Now()); err == nil || changed {
		t.Fatal("failed write reported success")
	}
	n, _ := store.GetNode("n")
	if n.Country != "" || len(store.PendingSecurityAuditIntents()) != 0 {
		t.Fatal("failed write leaked country or audit intent")
	}
	store.path = path
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	n, _ = reopened.GetNode("n")
	if n.Country != "" {
		t.Fatal("failed write reached disk")
	}
}

func TestLocationWorkerDoesNotOverwriteEditDuringLookup(t *testing.T) {
	app, store := locationRig(t)
	started, release := make(chan struct{}), make(chan struct{})
	app.locationLookup = func(ctx context.Context, ip string) (string, error) {
		close(started)
		select {
		case <-release:
			return "DE", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { app.StartLocationLoop(ctx); close(done) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("lookup did not start")
	}
	if _, err := store.SetNodeCountry("n", "NL", time.Now()); err != nil {
		t.Fatal(err)
	}
	close(release)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	n, _ := store.GetNode("n")
	if n.Country != "NL" || n.CountrySource != "manual" {
		t.Fatalf("late response overwrote edit: %+v", n)
	}
}

func TestLocationFailureKeepsNodeUnknownAndLoopCancellable(t *testing.T) {
	app, store := locationRig(t)
	called := make(chan struct{})
	app.locationLookup = func(context.Context, string) (string, error) { close(called); return "", errors.New("unavailable") }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { app.StartLocationLoop(ctx); close(done) }()
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("lookup did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	n, _ := store.GetNode("n")
	if n.Country != "" || len(store.ListNodes()) != 1 {
		t.Fatal("failed location changed node enrollment")
	}
}

func TestLocationAPIRequiresAuthAndSupportsReturningToAutomatic(t *testing.T) {
	app, store := locationRig(t)
	now := time.Now()
	if _, err := store.SetNodeCountry("n", "DE", now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		body  map[string]any
		want  int
	}{
		{"", map[string]any{"node_id": "n", "automatic": true}, 401},
		{"admin", map[string]any{"node_id": "n", "automatic": true, "country": "NL"}, 400},
		{"admin", map[string]any{"node_id": "n", "automatic": true}, 200},
	} {
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, authReq("POST", "/api/nodes/location", tc.token, tc.body))
		if w.Code != tc.want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
		}
	}
	n, _ := store.GetNode("n")
	if n.Country != "" || !automaticCountry(n) {
		t.Fatal("reset did not return to automatic")
	}
}
