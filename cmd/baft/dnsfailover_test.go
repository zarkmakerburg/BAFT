package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeZones(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9109}]}`))
			return
		}
		if r.URL.Query().Get("name") == "example.com" {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"zone123","name":"example.com"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDNSFailoverSetupWritesTokenConfigAndUnit(t *testing.T) {
	srv := fakeZones(t)
	dir, units := t.TempDir(), t.TempDir()
	var calls []string
	sysctl := func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }
	var out, errb bytes.Buffer
	code := runDNSFailoverSetup(context.Background(), []string{"--record", "t.example.com",
		"--target", "ir-a=192.0.2.1", "--target", "ir-b=192.0.2.2:8443", "--config-dir", dir, "--unit-dir", units},
		strings.NewReader("good-token\n"), &out, &errb, sysctl, srv.URL)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	fi, err := os.Stat(filepath.Join(dir, "cloudflare-dns.token"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file: %v %v", fi, err)
	}
	var cfg struct {
		ZoneID  string `json:"zone_id"`
		Targets []struct{ Address, Probe string }
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "dns-failover.json"))
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.ZoneID != "zone123" {
		t.Fatalf("config: %s %v", raw, err)
	}
	if cfg.Targets[0].Probe != "192.0.2.1:443" || cfg.Targets[1].Probe != "192.0.2.2:8443" {
		t.Fatalf("probes: %+v", cfg.Targets)
	}
	if strings.Contains(string(raw), "good-token") {
		t.Fatal("token leaked into the config")
	}
	unit, _ := os.ReadFile(filepath.Join(units, "baft-dns-failover.service"))
	if !strings.Contains(string(unit), "dns-failover --config "+filepath.Join(dir, "dns-failover.json")) {
		t.Fatalf("unit: %s", unit)
	}
	if strings.Join(calls, "|") != "daemon-reload|enable --now baft-dns-failover.service|restart baft-dns-failover.service" {
		t.Fatalf("systemctl calls: %v", calls)
	}
}

func TestDNSFailoverSetupRejectsBadTokenAndWritesNothing(t *testing.T) {
	srv := fakeZones(t)
	dir, units := t.TempDir(), t.TempDir()
	var out, errb bytes.Buffer
	code := runDNSFailoverSetup(context.Background(), []string{"--record", "t.example.com",
		"--target", "ir-a=192.0.2.1", "--target", "ir-b=192.0.2.2", "--config-dir", dir, "--unit-dir", units},
		strings.NewReader("bad\n"), &out, &errb, func(...string) error { t.Fatal("systemctl called"); return nil }, srv.URL)
	if code == 0 || !strings.Contains(errb.String(), "9109") {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	for _, d := range []string{dir, units} {
		if ents, _ := os.ReadDir(d); len(ents) != 0 {
			t.Fatalf("%s not empty after a failed setup: %v", d, ents)
		}
	}
}
