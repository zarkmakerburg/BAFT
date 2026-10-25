package bcc

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

// ---- fixtures: throwaway-root signed releases, built with the real signer ----

type ucFixture struct {
	t        *testing.T
	rootPub  ed25519.PublicKey
	rootPriv ed25519.PrivateKey
	relPub   ed25519.PublicKey
	relPriv  ed25519.PrivateKey
	cert     release.Envelope
	now      time.Time
}

func newUCFixture(t *testing.T) *ucFixture {
	t.Helper()
	f := &ucFixture{t: t, now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	var err error
	if f.rootPub, f.rootPriv, err = release.GenerateKey(); err != nil {
		t.Fatal(err)
	}
	if f.relPub, f.relPriv, err = release.GenerateKey(); err != nil {
		t.Fatal(err)
	}
	if f.cert, err = release.Certify(f.rootPriv, f.relPub, f.now.Add(-48*time.Hour), f.now.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return f
}

var ucArtifactNames = []string{"baft-linux-amd64", "baft-pair-linux-amd64", "baft-agent-linux-amd64", "baft-bcc-linux-amd64"}

func ucBinary(name, version string) []byte { return []byte("binary " + name + " " + version) }

func ucDigest(name, version string) string {
	s := sha256.Sum256(ucBinary(name, version))
	return hex.EncodeToString(s[:])
}

func (f *ucFixture) revocations(seq uint64, revoked ...string) release.Envelope {
	e, err := release.SignRevocations(f.rootPriv, seq, revoked, f.now.Add(-time.Hour), f.now.Add(60*24*time.Hour))
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func (f *ucFixture) expiredRevocations(seq uint64) release.Envelope {
	e, err := release.SignRevocations(f.rootPriv, seq, nil, f.now.Add(-72*time.Hour), f.now.Add(-time.Hour))
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

// bundle signs a release with release.SignDir and packs the signed files.
func (f *ucFixture) bundle(version string, commitChar string, rev release.Envelope) UpdateBundle {
	f.t.Helper()
	dir := f.t.TempDir()
	for _, n := range ucArtifactNames {
		if err := os.WriteFile(filepath.Join(dir, n), ucBinary(n, version), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
	_, err := release.SignDir(release.SignInput{
		Dir: dir, Version: version, Commit: strings.Repeat(commitChar, 40), CreatedAt: f.now.Add(-time.Hour),
		Provenance: release.Provenance{Builder: "test", GoVersion: "go1.27"}, Key: f.relPriv, Cert: f.cert, Root: f.rootPub,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	man, err := release.ReadEnvelope(filepath.Join(dir, release.ManifestFile))
	if err != nil {
		f.t.Fatal(err)
	}
	cert, err := release.ReadEnvelope(filepath.Join(dir, release.CertFile))
	if err != nil {
		f.t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(dir, release.SumsFile))
	if err != nil {
		f.t.Fatal(err)
	}
	return UpdateBundle{SchemaVersion: 1, Manifest: &man, Cert: &cert, Revocations: rev, SHA256SUMS: string(sums)}
}

func (f *ucFixture) relKeyID() string { return release.KeyID(f.relPub) }

// ---- server harness ----

type ucEnv struct {
	t     *testing.T
	f     *ucFixture
	store *Store
	app   *Server
	h     http.Handler
	path  string
}

func newUCEnv(t *testing.T, bccVersion string) *ucEnv {
	t.Helper()
	f := newUCFixture(t)
	path := filepath.Join(t.TempDir(), "bcc.sqlite")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	e := &ucEnv{t: t, f: f, store: store, path: path}
	e.app = e.newApp(store, bccVersion)
	e.h = e.app.Handler()
	return e
}

func (e *ucEnv) newApp(store *Store, bccVersion string) *Server {
	app, err := NewServer(store, "admin-secret")
	if err != nil {
		e.t.Fatal(err)
	}
	app.now = func() time.Time { return e.f.now }
	app.SetVersion(bccVersion)
	if err := app.ConfigureUpdateCenter(e.f.rootPub); err != nil {
		e.t.Fatal(err)
	}
	return app
}

func (e *ucEnv) do(method, path, token string, body any, hdr ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case nil:
	case []byte:
		buf.Write(b)
	default:
		if err := json.NewEncoder(&buf).Encode(b); err != nil {
			e.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &buf)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *ucEnv) imp(b UpdateBundle, hdr ...string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, "/api/updates/import", "admin-secret", b, hdr...)
}

// node enrolls a node with confirmed linux/amd64 platform and a fresh report.
func (e *ucEnv) node(id, installedVersion string) {
	e.t.Helper()
	if _, err := e.store.UpsertNode(Node{ID: id, Address: "127.0.0.1:2443", Role: "foreign"}, "agent-key-"+id); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.store.ConfirmNodePlatform(id, "linux", "amd64", e.f.now.Add(-time.Hour), AuditEntry{Actor: "admin"}); err != nil {
		e.t.Fatal(err)
	}
	e.report(id, installedVersion, e.f.now.Add(-time.Minute))
}

func (e *ucEnv) report(id, version string, accepted time.Time) {
	e.t.Helper()
	d := map[string]string{"transport": "unknown", "agent": "unknown", "bcc": "unknown"}
	if version != "" {
		d = map[string]string{
			"transport": ucDigest("baft-linux-amd64", version),
			"agent":     ucDigest("baft-agent-linux-amd64", version),
			"bcc":       ucDigest("baft-bcc-linux-amd64", version),
		}
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	if e.store.st.ComponentDigestReceipts == nil {
		e.store.st.ComponentDigestReceipts = map[string]ComponentDigestReceipt{}
	}
	e.store.st.ComponentDigestReceipts[id] = ComponentDigestReceipt{NodeID: id, ObservedAt: accepted, AcceptedAt: accepted, Digests: d}
	if err := e.store.saveLocked(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *ucEnv) view() UpdateView {
	e.t.Helper()
	w := e.do(http.MethodGet, "/api/updates", "admin-secret", nil)
	if w.Code != http.StatusOK {
		e.t.Fatalf("GET /api/updates = %d %s", w.Code, w.Body.String())
	}
	var v UpdateView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (v UpdateView) row(id string) UpdateNodeRow {
	for _, n := range v.Nodes {
		if n.NodeID == id {
			return n
		}
	}
	return UpdateNodeRow{}
}

func (e *ucEnv) digestOf(version string) string {
	for _, r := range e.view().Releases {
		if r.Version == version {
			return r.Digest
		}
	}
	e.t.Fatalf("release %s not imported", version)
	return ""
}

func mustCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body.String())
	}
}

func (e *ucEnv) auditActions() []string {
	_ = e.app.FlushSecurityAuditIntents()
	entries, err := e.app.audit.List(0)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, a := range entries {
		out = append(out, a.Action+":"+a.Outcome)
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---- tests ----

func TestUpdateCenterImportValidAndStatus(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", rev)), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", rev)), http.StatusCreated)
	e.node("ex-1", "v0.4.0")
	e.node("ex-2", "v0.4.1")
	e.node("ex-3", "v0.4.0")
	e.report("ex-3", "", e.f.now.Add(-time.Minute)) // unknown digests
	v := e.view()
	if !v.RootPinned || v.Floor != "v0.4.0" || v.Latest == nil || v.Latest.Version != "v0.4.1" || len(v.Releases) != 2 {
		t.Fatalf("view=%+v", v)
	}
	for _, r := range v.Releases {
		if r.SignatureStatus != "verified" || r.Revoked || r.Channel != "stable" {
			t.Fatalf("release=%+v", r)
		}
	}
	if r := v.row("ex-1"); r.Status != "update-available" || r.InstalledVersion != "v0.4.0" || r.LatestVersion != "v0.4.1" || r.NoticeCode != "update_available" || r.Notice == "" || r.LastReportAge == nil || *r.LastReportAge != 60 {
		t.Fatalf("ex-1=%+v", r)
	}
	if r := v.row("ex-2"); r.Status != "current" || r.InstalledVersion != "v0.4.1" {
		t.Fatalf("ex-2=%+v", r)
	}
	if r := v.row("ex-3"); r.Status != "unknown" || r.NoticeCode != "no_report" {
		t.Fatalf("ex-3=%+v", r)
	}
	if v.NoticeCount != 1 {
		t.Fatalf("notice count=%d", v.NoticeCount)
	}
	// Same bundle again is idempotent.
	w := e.imp(e.f.bundle("v0.4.1", "b", rev))
	mustCode(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `"unchanged"`) {
		t.Fatalf("body=%s", w.Body.String())
	}
	if len(e.view().Releases) != 2 {
		t.Fatal("idempotent import duplicated a release")
	}
}

func TestUpdateCenterStatusMatrix(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", rev)), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", rev)), http.StatusCreated)

	// no platform confirmation
	if _, err := e.store.UpsertNode(Node{ID: "noplat", Address: "127.0.0.1:1", Role: "foreign"}, "k"); err != nil {
		t.Fatal(err)
	}
	// ahead of the catalog: a node running a build newer than anything imported
	e.node("ahead", "v0.4.1")
	// stale report
	e.node("stale", "v0.4.0")
	e.report("stale", "v0.4.0", e.f.now.Add(-time.Hour))
	// digest of an unknown build
	e.node("alien", "v0.9.9")
	// no report at all
	if _, err := e.store.UpsertNode(Node{ID: "silent", Address: "127.0.0.1:2", Role: "foreign"}, "k2"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ConfirmNodePlatform("silent", "linux", "amd64", e.f.now.Add(-time.Hour), AuditEntry{Actor: "admin"}); err != nil {
		t.Fatal(err)
	}
	v := e.view()
	want := map[string][2]string{
		"noplat": {"unknown", "no_platform"}, "ahead": {"current", "current"}, "stale": {"unknown", "stale_report"},
		"alien": {"unknown", "unmatched_digest"}, "silent": {"unknown", "no_report"},
	}
	for id, w := range want {
		if r := v.row(id); r.Status != w[0] || r.NoticeCode != w[1] {
			t.Fatalf("%s = %s/%s want %v", id, r.Status, r.NoticeCode, w)
		}
	}

	// make v0.4.1 rollback-only by importing it below a higher floor
	e3 := newUCEnv(t, "v0.4.5")
	rev3 := e3.f.revocations(1)
	mustCode(t, e3.imp(e3.f.bundle("v0.4.1", "b", rev3), "X-BAFT-Downgrade-Confirmation", "DOWNGRADE v0.4.1 "+strings.Repeat("b", 40)), http.StatusCreated)
	e3.node("n", "v0.4.1")
	if r := e3.view().row("n"); r.Status != "unknown" || r.NoticeCode != "no_release" {
		t.Fatalf("rollback-only must never be the latest: %+v", r)
	}
	if !e3.view().Releases[0].RollbackOnly {
		t.Fatal("release not marked rollback-only")
	}

	// downgrade-blocked: installed newer than latest eligible release.
	e4 := newUCEnv(t, "v0.4.0")
	rev4 := e4.f.revocations(1)
	mustCode(t, e4.imp(e4.f.bundle("v0.4.0", "a", rev4)), http.StatusCreated)
	mustCode(t, e4.imp(e4.f.bundle("v0.4.1", "b", rev4)), http.StatusCreated)
	e4.node("n", "v0.4.1")
	// forget the newer release by using a catalog that only has v0.4.0 eligible:
	e4.store.mu.Lock()
	for d, r := range e4.store.st.UpdateReleases {
		if r.Version == "v0.4.1" {
			r.RollbackOnly = true
			e4.store.st.UpdateReleases[d] = r
		}
	}
	_ = e4.store.saveLocked()
	e4.store.mu.Unlock()
	if r := e4.view().row("n"); r.Status != "downgrade-blocked" || r.NoticeCode != "ahead_of_catalog" {
		t.Fatalf("row=%+v", r)
	}
}

func TestUpdateCenterRevokedKeyAndRevocationOnlyBundle(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", e.f.revocations(1))), http.StatusCreated)
	e.node("ex-1", "v0.4.0")
	if e.view().row("ex-1").Status != "current" {
		t.Fatal("setup")
	}
	// The root revokes the release key; a revocation-only bundle teaches BCC.
	rv := UpdateBundle{SchemaVersion: 1, Revocations: e.f.revocations(2, e.f.relKeyID())}
	w := e.imp(rv)
	mustCode(t, w, http.StatusCreated)
	v := e.view()
	if v.RevocationSequence != 2 || !v.Releases[0].Revoked || v.Latest != nil {
		t.Fatalf("view=%+v", v)
	}
	if r := v.row("ex-1"); r.Status != "revoked" || r.NoticeCode != "revoked_installed" {
		t.Fatalf("row=%+v", r)
	}
	if v.NoticeCount != 1 {
		t.Fatalf("revoked servers must raise a notice, got %d", v.NoticeCount)
	}
	// A release signed with the revoked key is refused, and an older list cannot undo it.
	mustCode(t, e.imp(e.f.bundle("v0.4.2", "c", e.f.revocations(2, e.f.relKeyID()))), http.StatusConflict)
	mustCode(t, e.imp(UpdateBundle{SchemaVersion: 1, Revocations: e.f.revocations(1)}), http.StatusConflict)
	if e.view().RevocationSequence != 2 {
		t.Fatal("older revocation list replaced a newer one")
	}
}

func TestUpdateCenterImportRefusals(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	good := e.f.bundle("v0.4.1", "b", rev)

	clone := func() UpdateBundle {
		b := good
		m, c := *good.Manifest, *good.Cert
		b.Manifest, b.Cert = &m, &c
		return b
	}
	tamperedManifest := clone()
	pl := []byte(`{"tampered":true}`)
	tamperedManifest.Manifest.Payload = base64Std(pl)
	otherRoot := newUCFixture(t)
	foreign := otherRoot.bundle("v0.4.1", "b", otherRoot.revocations(1))
	foreignRev := clone()
	foreignRev.Revocations = otherRoot.revocations(1)
	badSums := clone()
	badSums.SHA256SUMS = strings.Replace(good.SHA256SUMS, ucDigest("baft-linux-amd64", "v0.4.1")[:8], "deadbeef", 1)
	noSums := clone()
	noSums.SHA256SUMS = ""
	unsigned := clone()
	unsigned.Manifest.Signatures = nil
	halfPair := clone()
	halfPair.Cert = nil
	expired := e.f.bundle("v0.4.1", "b", e.f.expiredRevocations(1))
	wrongSchema := clone()
	wrongSchema.SchemaVersion = 2
	swappedCert := clone()
	swappedCert.Cert = swappedCert.Manifest // wrong payload type for a certificate

	cases := []struct {
		name string
		body any
		want int
	}{
		{"unsigned manifest", unsigned, http.StatusBadRequest},
		{"tampered manifest payload", tamperedManifest, http.StatusBadRequest},
		{"signed by an unpinned root", foreign, http.StatusBadRequest},
		{"revocation list from another root", foreignRev, http.StatusBadRequest},
		{"tampered SHA256SUMS", badSums, http.StatusBadRequest},
		{"missing SHA256SUMS", noSums, http.StatusBadRequest},
		{"manifest without certificate", halfPair, http.StatusBadRequest},
		{"expired revocation list", expired, http.StatusBadRequest},
		{"unknown schema", wrongSchema, http.StatusBadRequest},
		{"certificate slot holds a manifest", swappedCert, http.StatusBadRequest},
		{"not JSON", []byte("not json"), http.StatusBadRequest},
		{"unknown field", []byte(`{"schema_version":1,"revocations":{},"extra":1}`), http.StatusBadRequest},
		{"trailing data", append(ucJSON(t, good), []byte(` {}`)...), http.StatusBadRequest},
		{"oversize", bytes.Repeat([]byte(" "), updateBundleMaxBytes+10), http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := e.imp2(c.body)
			if w.Code != c.want {
				t.Fatalf("status = %d want %d: %s", w.Code, c.want, w.Body.String())
			}
		})
	}
	if v := e.view(); len(v.Releases) != 0 || v.RevocationSequence != 0 {
		t.Fatalf("a refused import changed state: %+v", v)
	}
	acts := e.auditActions()
	if !containsStr(acts, "updates.import:failure") {
		t.Fatalf("refusals are not audited: %v", acts)
	}
	// Control: the unmodified bundle is accepted.
	mustCode(t, e.imp(good), http.StatusCreated)
}

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func ucJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *ucEnv) imp2(body any) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, "/api/updates/import", "admin-secret", body)
}

func TestUpdateCenterFloorRollbackOnlyAndRetag(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	old := e.f.bundle("v0.3.7", "c", rev)
	// below the floor: refused without the exact confirmation
	mustCode(t, e.imp(old), http.StatusConflict)
	mustCode(t, e.imp(old, "X-BAFT-Downgrade-Confirmation", "DOWNGRADE v0.3.7 "+strings.Repeat("d", 40)), http.StatusConflict)
	w := e.imp(old, "X-BAFT-Downgrade-Confirmation", "DOWNGRADE v0.3.7 "+strings.Repeat("c", 40))
	mustCode(t, w, http.StatusCreated)
	if !strings.Contains(w.Body.String(), `"rollback_only":true`) {
		t.Fatalf("body=%s", w.Body.String())
	}
	v := e.view()
	if v.Latest != nil {
		t.Fatalf("a rollback-only release must never be offered as latest: %+v", v.Latest)
	}
	// re-tag: same version, different commit/content
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", rev)), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "e", rev)), http.StatusConflict)
	// the trust-state floor of a configured signed catalog also counts
	if f := e.app.updateFloor(); f != "v0.4.0" {
		t.Fatalf("floor=%s", f)
	}
}

func TestUpdateCenterNoRootPinnedRefuses(t *testing.T) {
	f := newUCFixture(t)
	store, err := OpenStore(filepath.Join(t.TempDir(), "s.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	app.now = func() time.Time { return f.now }
	e := &ucEnv{t: t, f: f, store: store, app: app, h: app.Handler()}
	mustCode(t, e.imp(f.bundle("v0.4.1", "b", f.revocations(1))), http.StatusServiceUnavailable)
	if v := e.view(); v.RootPinned || len(v.Releases) != 0 {
		t.Fatalf("view=%+v", v)
	}
	if err := app.ConfigureUpdateCenter([]byte("short")); err == nil {
		t.Fatal("accepted a malformed root")
	}
}

func TestUpdateCenterAuth(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	e.node("ex-1", "v0.4.0")
	n := 0
	for _, ep := range []struct{ method, path string }{
		{http.MethodGet, "/api/updates"}, {http.MethodPost, "/api/updates/import"},
		{http.MethodPost, "/api/updates/plan"}, {http.MethodPost, "/api/updates/apply"},
	} {
		for _, tok := range []string{"", "wrong", "agent-key-ex-1"} {
			// a distinct client address per attempt keeps the brute-force guard out of the way
			n++
			r := httptest.NewRequest(ep.method, ep.path, strings.NewReader("{}"))
			r.RemoteAddr = "198.51.100." + strconv.Itoa(n) + ":4000"
			if tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s token=%q => %d", ep.method, ep.path, tok, w.Code)
			}
		}
	}
	if len(e.view().Releases) != 0 || len(e.store.st.Jobs) != 0 {
		t.Fatal("an unauthenticated call changed state")
	}
	mustCode(t, e.do(http.MethodPost, "/api/updates", "admin-secret", nil), http.StatusMethodNotAllowed)
	mustCode(t, e.do(http.MethodGet, "/api/updates/import", "admin-secret", nil), http.StatusMethodNotAllowed)
	mustCode(t, e.do(http.MethodGet, "/api/updates/plan", "admin-secret", nil), http.StatusMethodNotAllowed)
	mustCode(t, e.do(http.MethodGet, "/api/updates/apply", "admin-secret", nil), http.StatusMethodNotAllowed)
}

func (e *ucEnv) plan(node, kind, digest string) (*httptest.ResponseRecorder, UpdatePlan) {
	w := e.do(http.MethodPost, "/api/updates/plan", "admin-secret", map[string]string{"node_id": node, "kind": kind, "target_digest": digest})
	var p UpdatePlan
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			e.t.Fatal(err)
		}
	}
	return w, p
}

func TestUpdateCenterPlanDeterministicAndGates(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", rev)), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", rev)), http.StatusCreated)
	e.node("ex-1", "v0.4.0")
	e.node("ex-2", "v0.4.1")
	d041, d040 := e.digestOf("v0.4.1"), e.digestOf("v0.4.0")

	w, p := e.plan("ex-1", "upgrade", d041)
	mustCode(t, w, http.StatusOK)
	_, p2 := e.plan("ex-1", "upgrade", d041)
	if !p.OK || p.Hash == "" || p.Hash != p2.Hash || p.Executor != UpdateExecAgentJob || p.FromVersion != "v0.4.0" || p.ToVersion != "v0.4.1" {
		t.Fatalf("plan=%+v", p)
	}
	if p.Apply == nil || p.Apply.Confirmation != "UPDATE ex-1 v0.4.1" || len(p.Steps) == 0 || len(p.Verify) == 0 || len(p.Rollback) == 0 {
		t.Fatalf("plan lacks steps/rollback/apply: %+v", p)
	}
	if p.computeHash() != p.Hash {
		t.Fatal("hash is not reproducible from the plan body")
	}
	for _, g := range p.Gates {
		if g.Status == "FAIL" {
			t.Fatalf("gate %+v", g)
		}
	}
	// The hash binds the node's installed release and the revocation sequence.
	e.report("ex-1", "v0.4.1", e.f.now.Add(-time.Minute))
	_, p3 := e.plan("ex-1", "upgrade", d041)
	if p3.OK || p3.Hash == p.Hash {
		t.Fatalf("drift not reflected: %+v", p3)
	}
	e.report("ex-1", "v0.4.0", e.f.now.Add(-time.Minute))

	// Already current / downgrade as upgrade / rollback-needs-older.
	if _, pc := e.plan("ex-2", "upgrade", d041); pc.OK {
		t.Fatal("upgrade to the installed release must fail")
	}
	if _, pd := e.plan("ex-2", "upgrade", d040); pd.OK {
		t.Fatal("a downgrade must not be plannable as an upgrade")
	}
	if _, pr := e.plan("ex-1", "rollback", d041); pr.OK {
		t.Fatal("rollback to a newer release must fail")
	}

	// Explicit rollback uses the installer with --allow-downgrade; no apply button.
	_, rb := e.plan("ex-2", "rollback", d040)
	if !rb.OK || rb.Executor != UpdateExecInstaller || rb.Apply != nil || !strings.Contains(rb.InstallerCommand, "--allow-downgrade") ||
		!strings.Contains(rb.InstallerPreview, "--plan") || len(rb.Rollback) == 0 {
		t.Fatalf("rollback plan=%+v", rb)
	}
	warn := false
	for _, g := range rb.Gates {
		warn = warn || (g.Name == "version order" && g.Status == "WARN")
	}
	if !warn {
		t.Fatalf("rollback must carry an explicit downgrade warning: %+v", rb.Gates)
	}

	// Unknown node / release / bad input.
	mustCode(t, mustPlan(e, "nope", "upgrade", d041), http.StatusNotFound)
	mustCode(t, mustPlan(e, "ex-1", "upgrade", strings.Repeat("0", 64)), http.StatusNotFound)
	mustCode(t, mustPlan(e, "ex-1", "sideways", d041), http.StatusBadRequest)
	mustCode(t, mustPlan(e, "ex-1", "upgrade", "xyz"), http.StatusBadRequest)
	mustCode(t, mustPlan(e, "../x", "upgrade", d041), http.StatusBadRequest)
	mustCode(t, e.do(http.MethodPost, "/api/updates/plan", "admin-secret", []byte(`{"node_id":"ex-1","kind":"upgrade","target_digest":"`+d041+`","extra":1}`)), http.StatusBadRequest)
	mustCode(t, e.do(http.MethodPost, "/api/updates/plan", "admin-secret", bytes.Repeat([]byte(" "), updateRequestMaxBytes+5)), http.StatusRequestEntityTooLarge)

	// A revoked target cannot be planned as OK.
	mustCode(t, e.imp(UpdateBundle{SchemaVersion: 1, Revocations: e.f.revocations(2, e.f.relKeyID())}), http.StatusCreated)
	_, pv := e.plan("ex-1", "upgrade", d041)
	if pv.OK || pv.Hash == p.Hash {
		t.Fatalf("revoked release planned OK: %+v", pv)
	}
	acts := e.auditActions()
	if !containsStr(acts, "updates.plan:success") || !containsStr(acts, "updates.import:success") {
		t.Fatalf("audit=%v", acts)
	}
}

func mustPlan(e *ucEnv, node, kind, digest string) *httptest.ResponseRecorder {
	w, _ := e.plan(node, kind, digest)
	return w
}

func TestUpdateCenterApplyIsHashBoundAndUsesExistingDeployJob(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	rev := e.f.revocations(1)
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", rev)), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", rev)), http.StatusCreated)
	e.node("ex-1", "v0.4.0")
	d041, d040 := e.digestOf("v0.4.1"), e.digestOf("v0.4.0")
	_, p := e.plan("ex-1", "upgrade", d041)
	apply := func(hash, conf, kind, digest string) *httptest.ResponseRecorder {
		return e.do(http.MethodPost, "/api/updates/apply", "admin-secret", map[string]string{
			"node_id": "ex-1", "kind": kind, "target_digest": digest, "plan_hash": hash, "confirmation": conf,
		})
	}
	mustCode(t, apply("0000", p.Apply.Confirmation, "upgrade", d041), http.StatusConflict)    // plan hash drift
	mustCode(t, apply(p.Hash, "yes", "upgrade", d041), http.StatusBadRequest)                 // no explicit confirmation
	mustCode(t, apply(p.Hash, p.Apply.Confirmation, "rollback", d040), http.StatusBadRequest) // rollback never runs from BCC
	if len(e.store.st.Jobs) != 0 {
		t.Fatal("a refused apply created a job")
	}
	// The world changes after review: a revocation list arrives. Apply refuses.
	mustCode(t, e.imp(UpdateBundle{SchemaVersion: 1, Revocations: e.f.revocations(2)}), http.StatusCreated)
	mustCode(t, apply(p.Hash, p.Apply.Confirmation, "upgrade", d041), http.StatusConflict)
	_, p = e.plan("ex-1", "upgrade", d041)
	w := apply(p.Hash, p.Apply.Confirmation, "upgrade", d041)
	mustCode(t, w, http.StatusAccepted)
	var jobs []Job
	if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil || len(jobs) != 1 || jobs[0].Type != JobDeployBAFT || jobs[0].Version != "v0.4.1" || jobs[0].NodeID != "ex-1" {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
	// A second apply while the job is in flight is refused.
	mustCode(t, apply(p.Hash, p.Apply.Confirmation, "upgrade", d041), http.StatusConflict)
	// The job a real agent receives is the signed update_baft action.
	key, _, err := LoadOrCreateJobKey(filepath.Join(t.TempDir(), "job.key"))
	if err != nil {
		t.Fatal(err)
	}
	e.app.ConfigureJobSigning(key)
	signed, err := e.app.signAgentJobs(jobs)
	if err != nil || signed[0].Signed == nil {
		t.Fatalf("signed=%v err=%v", signed, err)
	}
	acts := e.auditActions()
	if !containsStr(acts, "deploy.create:success") || !containsStr(acts, "updates.apply:failure") {
		t.Fatalf("audit=%v", acts)
	}
}

func TestUpdateCenterPersistsAcrossRestart(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", e.f.revocations(1))), http.StatusCreated)
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", e.f.revocations(3, "00000000000000000000000000000001"))), http.StatusCreated)
	before := e.view()
	reopened, err := OpenStore(e.path)
	if err != nil {
		t.Fatal(err)
	}
	e2 := &ucEnv{t: t, f: e.f, store: reopened, path: e.path}
	e2.app = e2.newApp(reopened, "v0.4.0")
	e2.h = e2.app.Handler()
	after := e2.view()
	if len(after.Releases) != 2 || after.RevocationSequence != 3 || after.Latest == nil || after.Latest.Version != "v0.4.1" {
		t.Fatalf("lost across restart: %+v", after)
	}
	bj, _ := json.Marshal(before.Releases)
	aj, _ := json.Marshal(after.Releases)
	if !bytes.Equal(bj, aj) {
		t.Fatalf("release records changed across restart:\n%s\n%s", bj, aj)
	}
	// Re-verification on read: a different pinned root marks the records invalid.
	other := newUCFixture(t)
	e3 := &ucEnv{t: t, f: other, store: reopened, path: e.path}
	e3.app = e3.newApp(reopened, "v0.4.0")
	e3.h = e3.app.Handler()
	for _, r := range e3.view().Releases {
		if r.SignatureStatus != "invalid" {
			t.Fatalf("stored release trusted under a different root: %+v", r)
		}
	}
	if e3.view().Latest != nil {
		t.Fatal("invalid releases must not be offered")
	}
}

func TestUpdateCenterBackupRestoreKeepsImportsAndRevocations(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", e.f.revocations(1))), http.StatusCreated)
	backup := filepath.Join(t.TempDir(), "b.baftbak")
	header, err := e.app.BackupToFile(backup, backupTestKey(), e.f.now)
	if err != nil || header.SchemaVersion != 5 {
		t.Fatalf("header=%+v err=%v", header, err)
	}
	// Live state moves on after the backup: a new release and a newer revocation list.
	mustCode(t, e.imp(e.f.bundle("v0.4.1", "b", e.f.revocations(2, "00000000000000000000000000000002"))), http.StatusCreated)
	if err := e.app.RestoreFromFile(backup, backupTestKey()); err != nil {
		t.Fatal(err)
	}
	v := e.view()
	if len(v.Releases) != 2 || v.RevocationSequence != 2 {
		t.Fatalf("restore forgot imports or resurrected an older revocation list: %+v", v)
	}
	// And the other direction: a backup that has the data restores into an empty store.
	backup2 := filepath.Join(t.TempDir(), "b2.baftbak")
	if _, err := e.app.BackupToFile(backup2, backupTestKey(), e.f.now); err != nil {
		t.Fatal(err)
	}
	fresh := newUCEnv(t, "v0.4.0")
	if err := fresh.app.RestoreFromFile(backup2, backupTestKey()); err != nil {
		t.Fatal(err)
	}
	fresh.f = e.f
	fresh.app = fresh.newApp(fresh.store, "v0.4.0")
	fresh.h = fresh.app.Handler()
	if v := fresh.view(); len(v.Releases) != 2 || v.RevocationSequence != 2 || v.Latest == nil || v.Latest.Version != "v0.4.1" {
		t.Fatalf("restore into a fresh BCC lost the Update Center: %+v", v)
	}
	// Preview does not refuse on Update Center state.
	prev, err := e.app.PreviewRestore(backup2, backupTestKey())
	if err == nil && prev.WouldRefuse != "" {
		t.Fatalf("preview refuses: %s", prev.WouldRefuse)
	}
}

func TestUpdateCenterLegacyBackupSchemaCannotCarryUpdateState(t *testing.T) {
	e := newUCEnv(t, "v0.4.0")
	mustCode(t, e.imp(e.f.bundle("v0.4.0", "a", e.f.revocations(1))), http.StatusCreated)
	raw, header, err := e.app.CreateBackupBytes(backupTestKey(), e.f.now)
	if err != nil || header.SchemaVersion != BackupSchemaVersion {
		t.Fatal(err)
	}
	if _, _, err := decodeBackup(raw, backupTestKey()); err != nil {
		t.Fatalf("current backup refused: %v", err)
	}
	// Re-seal the same payload under an old header: an older binary would drop
	// the Update Center silently, so a v4 header must not carry it.
	old := legacyReservationBackupFixture(t, raw, backupTestKey(), 4)
	if _, _, err := decodeBackup(old, backupTestKey()); err == nil || !strings.Contains(err.Error(), "legacy backup schema") {
		t.Fatalf("legacy header accepted Update Center state: %v", err)
	}
}
