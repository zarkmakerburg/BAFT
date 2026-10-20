package bcc

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/zarkmakerburg/baft/internal/release"
)

func TestAgentPullSigningFailureNeverDispatchesJob(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "bcc.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNode(Node{ID: "ir", Role: "worker", Address: "127.0.0.1:2222"}, "token-ir"); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.CreateDeployJobs([]string{"ir"}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	id := jobs[0].ID
	store.mu.Lock()
	broken := store.st.Jobs[id]
	broken.Type = "unsupported-action"
	store.st.Jobs[id] = broken
	err = store.saveLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	pull := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/agent/jobs?node_id=ir", nil)
		req.Header.Set("Authorization", "Bearer token-ir")
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, req)
		return response
	}
	if response := pull(); response.Code != http.StatusInternalServerError {
		t.Fatalf("failed signing response=%d %s", response.Code, response.Body.String())
	}
	if store.st.Jobs[id].Status != "queued" {
		t.Fatal("undelivered job was marked dispatched")
	}
	reopened, err := OpenStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.Jobs[id].Status != "queued" {
		t.Fatal("undelivered job was dispatched on disk")
	}
	store.mu.Lock()
	repaired := store.st.Jobs[id]
	repaired.Type = JobDeployBAFT
	store.st.Jobs[id] = repaired
	err = store.saveLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	path := store.path
	store.path = t.TempDir()
	if response := pull(); response.Code != http.StatusInternalServerError {
		t.Fatalf("failed persistence response=%d %s", response.Code, response.Body.String())
	}
	if store.st.Jobs[id].Status != "queued" {
		t.Fatal("failed persistence dispatched job")
	}
	store.path = path
	if response := pull(); response.Code != http.StatusOK {
		t.Fatalf("repaired job was not delivered: %d %s", response.Code, response.Body.String())
	}
	if store.st.Jobs[id].Status != "dispatched" {
		t.Fatal("delivered job remained queued")
	}
}

func TestAgentPullWithoutSigningKeyKeepsQueuedJob(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "bcc.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNode(Node{ID: "ir", Role: "worker", Address: "127.0.0.1:2222"}, "token-ir"); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.CreateDeployJobs([]string{"ir"}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agent/jobs?node_id=ir", nil)
	req.Header.Set("Authorization", "Bearer token-ir")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || store.st.Jobs[jobs[0].ID].Status != "queued" {
		t.Fatalf("unsigned job escaped or stranded: status=%d job=%+v", response.Code, store.st.Jobs[jobs[0].ID])
	}
	reopened, err := OpenStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.Jobs[jobs[0].ID].Status != "queued" {
		t.Fatal("missing signing key dispatched job on disk")
	}
	_, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK || store.st.Jobs[jobs[0].ID].Status != "dispatched" {
		t.Fatalf("restored signing key did not deliver job: %d %s", response.Code, response.Body.String())
	}
}
