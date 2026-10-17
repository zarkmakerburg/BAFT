package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

const (
	commitA = "4545a784cb6c043ac1f97f88c810c422144d7823"
	commitB = "879ade5af7cb05f3a64bb04c530567e09e838f85"
)

var installerBody = []byte("#!/usr/bin/env bash\necho installer\n")

// rig is a signed release served over TLS, plus the host's trust state.
type rig struct {
	t         *testing.T
	now       time.Time
	rootPub   ed25519.PublicKey
	rootKey   ed25519.PrivateKey
	relKey    ed25519.PrivateKey
	cert      release.Envelope
	files     map[string][]byte // served under /dl/ and /revocations.json
	srv       *httptest.Server
	statePath string
	hits      []string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), files: map[string][]byte{}}
	var err error
	if r.rootPub, r.rootKey, err = release.GenerateKey(); err != nil {
		t.Fatal(err)
	}
	var relPub ed25519.PublicKey
	if relPub, r.relKey, err = release.GenerateKey(); err != nil {
		t.Fatal(err)
	}
	if r.cert, err = release.Certify(r.rootKey, relPub, r.now.Add(-time.Hour), r.now.Add(180*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits = append(r.hits, req.URL.Path)
		b, ok := r.files[req.URL.Path]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(r.srv.Close)
	r.statePath = filepath.Join(t.TempDir(), "release-state.json")
	r.publish("v0.3.4", commitA, 1, nil)
	return r
}

// publish signs a release and (re)serves it, with a revocation list of the
// given sequence revoking the given key ids.
func (r *rig) publish(version, commit string, seq uint64, revoked []string) {
	r.t.Helper()
	dir := r.t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, InstallerArtifact("amd64")), installerBody, 0o755); err != nil {
		r.t.Fatal(err)
	}
	if _, err := release.SignDir(release.SignInput{
		Dir: dir, Version: version, Commit: commit, CreatedAt: r.now,
		Provenance: release.Provenance{Builder: "test", GoVersion: "go1.27.1"},
		Key:        r.relKey, Cert: r.cert, Root: r.rootPub,
	}); err != nil {
		r.t.Fatal(err)
	}
	for _, name := range []string{release.ManifestFile, release.CertFile, release.SumsFile, InstallerArtifact("amd64")} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			r.t.Fatal(err)
		}
		r.files["/dl/"+name] = b
	}
	rev, err := release.SignRevocations(r.rootKey, seq, revoked, r.now.Add(-time.Hour), r.now.Add(30*24*time.Hour))
	if err != nil {
		r.t.Fatal(err)
	}
	p := filepath.Join(dir, "revocations.json")
	if err := release.WriteEnvelope(p, rev); err != nil {
		r.t.Fatal(err)
	}
	if r.files["/revocations.json"], err = os.ReadFile(p); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) installed(version, commit string, seq uint64) {
	r.t.Helper()
	if err := release.WriteState(r.statePath, release.TrustState{Version: version, Commit: commit, RevocationSequence: seq, UpdatedAt: r.now}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) cfg() Config {
	return Config{
		Root: r.rootPub,
		Source: Source{
			ReleaseURL:     r.srv.URL + "/dl",
			RevocationsURL: r.srv.URL + "/revocations.json",
			Client:         r.srv.Client(),
		},
		StatePath: r.statePath,
		Arch:      "amd64",
		Now:       func() time.Time { return r.now },
	}
}

func (r *rig) check(version string) Report {
	r.t.Helper()
	return Check(context.Background(), r.cfg(), version, r.t.TempDir())
}

func wantUnknown(t *testing.T, rep Report, reason string) {
	t.Helper()
	if rep.Status != Unknown {
		t.Fatalf("status = %q, want unknown (reason %q)", rep.Status, rep.Reason)
	}
	if !strings.Contains(rep.Reason, reason) {
		t.Fatalf("reason = %q, want one containing %q", rep.Reason, reason)
	}
	if rep.Installer != "" {
		t.Fatalf("an unverified result still offers an installer: %q", rep.Installer)
	}
}

func TestNewerSignedReleaseIsAvailableAndTheInstallerIsTheSignedOne(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)
	rep := r.check("")
	if rep.Status != UpdateAvailable || rep.Installed != "v0.3.3" || rep.Latest != "v0.3.4" || rep.Commit != commitA {
		t.Fatalf("report = %+v", rep)
	}
	if rep.SigningKeyID == "" || rep.RevocationSequence != 1 || rep.ReleasedAt.IsZero() || rep.Reason != "" {
		t.Fatalf("report lacks the verified facts: %+v", rep)
	}
	got, err := os.ReadFile(rep.Installer)
	if err != nil || !bytes.Equal(got, installerBody) {
		t.Fatalf("installer = %q, %v", got, err)
	}
}

func TestSameReleaseIsCurrent(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.4", commitA, 1)
	rep := r.check("")
	if rep.Status != Current || rep.Latest != "v0.3.4" || rep.Installer == "" {
		t.Fatalf("report = %+v", rep)
	}
}

func TestACheckNeverWritesTheTrustState(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)
	before, err := os.ReadFile(r.statePath)
	if err != nil {
		t.Fatal(err)
	}
	r.publish("v0.3.4", commitA, 5, nil)
	if rep := r.check(""); rep.Status != UpdateAvailable {
		t.Fatalf("report = %+v", rep)
	}
	after, err := os.ReadFile(r.statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("trust state changed by a check: %v", err)
	}
}

func TestRefusedReleasesAreUnknownNeverCurrent(t *testing.T) {
	t.Run("downgrade", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.4.0", commitB, 1)
		wantUnknown(t, r.check(""), "downgrade refused")
	})
	t.Run("same version rebuilt from another commit", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.4", commitB, 1)
		rep := r.check("")
		wantUnknown(t, rep, "already accepted as commit")
	})
	t.Run("revocation list older than one already accepted", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.3", commitB, 9)
		wantUnknown(t, r.check(""), "is older than")
	})
	t.Run("signing key revoked", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.3", commitB, 1)
		relPub := r.relKey.Public().(ed25519.PublicKey)
		r.publish("v0.3.4", commitA, 2, []string{release.KeyID(relPub)})
		wantUnknown(t, r.check(""), "is revoked")
	})
	t.Run("tampered installer", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.3", commitB, 1)
		r.files["/dl/"+InstallerArtifact("amd64")] = []byte("#!/bin/sh\nrm -rf /\n")
		wantUnknown(t, r.check(""), "did not verify")
	})
	t.Run("manifest signed by an unpinned root", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.3", commitB, 1)
		other, _, err := release.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		cfg := r.cfg()
		cfg.Root = other
		rep := Check(context.Background(), cfg, "", t.TempDir())
		wantUnknown(t, rep, "did not verify")
	})
	t.Run("release served under a tag it does not declare", func(t *testing.T) {
		r := newRig(t)
		r.installed("v0.3.3", commitB, 1)
		wantUnknown(t, r.check("v0.3.5"), "declares itself v0.3.4")
	})
}

func TestUnreachableReleaseIsUnknown(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)
	delete(r.files, "/dl/"+release.ManifestFile)
	wantUnknown(t, r.check(""), "HTTP 404")

	r2 := newRig(t)
	r2.installed("v0.3.3", commitB, 1)
	delete(r2.files, "/revocations.json")
	wantUnknown(t, r2.check(""), "revocation list")
}

func TestHostWithoutReleaseStateDoesNotGuess(t *testing.T) {
	r := newRig(t)
	rep := r.check("")
	if rep.Status != Unknown || rep.Latest != "v0.3.4" || rep.Installed != "" {
		t.Fatalf("report = %+v", rep)
	}
	if !strings.Contains(rep.Reason, "no release state") || rep.Installer != "" {
		t.Fatalf("report = %+v", rep)
	}
}

func TestCorruptReleaseStateIsUnknown(t *testing.T) {
	r := newRig(t)
	if err := os.WriteFile(r.statePath, []byte(`{"schema_version":1,"surprise":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	wantUnknown(t, r.check(""), "release state cannot be read")
}

func TestOnlyHTTPSAndBoundedDownloadsAreAccepted(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)

	cfg := r.cfg()
	cfg.ReleaseURL = strings.Replace(r.srv.URL, "https://", "http://", 1) + "/dl"
	wantUnknown(t, Check(context.Background(), cfg, "", t.TempDir()), "only https is accepted")
	cfg = r.cfg()
	cfg.RevocationsURL = "file:///etc/passwd"
	wantUnknown(t, Check(context.Background(), cfg, "", t.TempDir()), "only https is accepted")

	r.files["/dl/"+InstallerArtifact("amd64")] = bytes.Repeat([]byte("x"), maxInstallerBytes+1)
	wantUnknown(t, r.check(""), "larger than")
}

func TestRedirectToPlainHTTPIsRefused(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("x")) }))
	defer plain.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, plain.URL+req.URL.Path, http.StatusFound)
	}))
	defer redirector.Close()
	cfg := r.cfg()
	cfg.ReleaseURL = redirector.URL + "/dl"
	cfg.Client = redirector.Client()
	wantUnknown(t, Check(context.Background(), cfg, "", t.TempDir()), "only https is accepted")
}

func TestInputsAreValidatedBeforeAnyRequest(t *testing.T) {
	r := newRig(t)
	r.installed("v0.3.3", commitB, 1)
	cases := []struct {
		name    string
		mutate  func(*Config, *string)
		wantMsg string
	}{
		{"arch", func(c *Config, _ *string) { c.Arch = "riscv64" }, "unsupported architecture"},
		{"no root", func(c *Config, _ *string) { c.Root = nil }, "no pinned release root key"},
		{"version", func(_ *Config, v *string) { *v = "../../etc/passwd" }, "not a release version"},
		{"repo", func(c *Config, _ *string) { c.Source = Source{Repo: "a/b/c"} }, "not a repository"},
	}
	for _, c := range cases {
		cfg, version := r.cfg(), ""
		c.mutate(&cfg, &version)
		wantUnknown(t, Check(context.Background(), cfg, version, t.TempDir()), c.wantMsg)
	}
	if len(r.hits) != 0 {
		t.Fatalf("requests were made for invalid input: %v", r.hits)
	}
}

func TestDefaultLocationsAreGitHub(t *testing.T) {
	base, rev, err := (Source{}).urls("")
	if err != nil || base != "https://github.com/zarkmakerburg/baft/releases/latest/download" ||
		rev != "https://raw.githubusercontent.com/zarkmakerburg/baft/main/release/keys/revocations.json" {
		t.Fatalf("latest: %q %q %v", base, rev, err)
	}
	base, _, err = (Source{Repo: "o/r"}).urls("v1.2.3-rc1")
	if err != nil || base != "https://github.com/o/r/releases/download/v1.2.3-rc1" {
		t.Fatalf("tagged: %q %v", base, err)
	}
}
