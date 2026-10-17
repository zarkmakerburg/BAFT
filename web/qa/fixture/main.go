// Browser QA only: a real authenticated BCC on loopback with disposable state.
// No agent, health, tunnel, bootstrap or discovery loop runs in this fixture.
// It is outside webui's embedded assets and is never installed as a service.
package main

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "browser fixture:", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "baft-browser-qa-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	store, err := bcc.OpenStore(filepath.Join(dir, "state.json"))
	if err != nil {
		return err
	}
	var agents []struct{ id, token string }
	for i, spec := range []struct{ id, role, country string }{{"qa-ir", "worker", "IR"}, {"qa-ex", "foreign", "DE"}} {
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		agentToken := make([]byte, 32)
		if _, err := rand.Read(agentToken); err != nil {
			return err
		}
		agents = append(agents, struct{ id, token string }{spec.id, base64.StdEncoding.EncodeToString(agentToken)})
		_, err = store.UpsertNode(bcc.Node{
			ID: spec.id, Alias: "Browser QA " + spec.id, Role: spec.role,
			Address: fmt.Sprintf("127.0.0.1:%d", 23001+i), PathIPv4: fmt.Sprintf("192.0.2.%d", i+10),
			PublicKey: base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), Country: spec.country, CountryManual: true,
		}, base64.StdEncoding.EncodeToString(agentToken))
		if err != nil {
			return err
		}
	}
	admin := make([]byte, 32)
	if _, err := rand.Read(admin); err != nil {
		return err
	}
	app, err := bcc.NewServer(store, base64.StdEncoding.EncodeToString(admin))
	if err != nil {
		return err
	}
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	app.ConfigureJobSigning(signing)
	app.SetVersion("browser-qa")
	backupKey := make([]byte, 32)
	if _, err := rand.Read(backupKey); err != nil {
		return err
	}
	if err := app.ConfigureBackupAdmin(filepath.Join(dir, "backups"), backupKey); err != nil {
		return err
	}
	accessFile := filepath.Join(dir, "access.json")
	creds, err := bcc.RotateAccess(accessFile, true, time.Now())
	if err != nil {
		return err
	}
	if err := app.ConfigureAccess(accessFile, false); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	// Simulate contact through the actual agent endpoint so a ready plan can be
	// reviewed in the browser. No job is executed and no agent process exists.
	client := &http.Client{Timeout: 2 * time.Second}
	for _, agent := range agents {
		req, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/api/agent/jobs?node_id="+agent.id, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+agent.token)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fixture agent contact: HTTP %d", resp.StatusCode)
		}
	}
	// The parent captures this pipe in memory. Never upload it as an artifact.
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
		"base":     "http://" + listener.Addr().String() + "/" + creds.SecretPath + "/",
		"username": creds.Username, "password": creds.Password,
	}); err != nil {
		_ = server.Close()
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-done:
		return err
	}
}
