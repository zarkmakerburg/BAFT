package bcc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func reservationStore(t *testing.T) (*Store, PublicIngressSpec) {
	t.Helper()
	s, req := ingressPreflightStore(t)
	s.mu.Lock()
	tunnel := s.st.Tunnels[req.TunnelID]
	tunnel.Phase = TunnelActive
	s.st.Tunnels[tunnel.ID] = tunnel
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	return s, PublicIngressSpec{IRNode: req.IRNode, TunnelID: req.TunnelID, PublicPort: 54619, LocalTarget: req.LocalTarget}
}

func TestPublicIngressReservationsPersistAndAreIdempotent(t *testing.T) {
	s, spec := reservationStore(t)
	first, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	auditCount := len(s.PendingSecurityAuditIntents())
	second, err := s.ReservePublicIngress(spec, time.Now().Add(time.Hour), ingressAudit())
	if err != nil || first != second || len(s.PendingSecurityAuditIntents()) != auditCount {
		t.Fatalf("idempotence failed: %+v %v", second, err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	reservations, err := reopened.ListPublicIngressReservations()
	if err != nil || len(reservations) != 1 || reservations[0] != first {
		t.Fatalf("restart lost desired reservation: %+v %v", reservations, err)
	}
	cloned, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	if len(cloned.PublicIngressReservations) != 1 {
		t.Fatal("backup clone lost reservations")
	}
}

func TestPublicIngressReservationsRejectOwnedAndProtectedPorts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Store, *PublicIngressSpec)
	}{
		{"2053", func(_ *Store, p *PublicIngressSpec) { p.PublicPort = 2053 }},
		{"ssh", func(_ *Store, p *PublicIngressSpec) { p.PublicPort = 22 }},
		{"route", func(_ *Store, p *PublicIngressSpec) { p.PublicPort = 1443 }},
		{"metrics", func(_ *Store, p *PublicIngressSpec) { p.PublicPort = 9191 }},
		{"node mismatch", func(_ *Store, p *PublicIngressSpec) { p.IRNode = "dst" }},
		{"tunnel missing", func(_ *Store, p *PublicIngressSpec) { p.TunnelID = "missing" }},
		{"target mismatch", func(_ *Store, p *PublicIngressSpec) { p.LocalTarget = "127.0.0.1:1444" }},
		{"revoked", func(s *Store, _ *PublicIngressSpec) { n := s.st.Nodes["src"]; n.Revoked = true; s.st.Nodes["src"] = n }},
		{"transport", func(s *Store, p *PublicIngressSpec) {
			s.st.Tunnels["transport"] = Tunnel{ID: "transport", EXNode: "src", Port: p.PublicPort, Phase: TunnelActive}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, spec := reservationStore(t)
			tc.change(s, &spec)
			if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err == nil {
				t.Fatal("unsafe reservation accepted")
			}
			if len(s.st.PublicIngressReservations) != 0 {
				t.Fatal("rejected allocation persisted")
			}
		})
	}
}

func TestPublicIngressReservationConcurrencyAndImmutableOwner(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	s.st.Tunnels["tun-other"] = Tunnel{ID: "tun-other", IRNode: "src", EXNode: "dst", RouteListen: "127.0.0.1:1444", Phase: TunnelActive}
	s.mu.Unlock()
	other := spec
	other.TunnelID = "tun-other"
	other.LocalTarget = "127.0.0.1:1444"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, candidate := range []PublicIngressSpec{spec, other} {
		wg.Go(func() { _, err := s.ReservePublicIngress(candidate, time.Now(), ingressAudit()); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("same port got %d owners", success)
	}
	reservations, err := s.ListPublicIngressReservations()
	if err != nil || len(reservations) != 1 {
		t.Fatalf("conflicting reservations: %+v %v", reservations, err)
	}
	changed := reservations[0].Spec
	changed.PublicPort++
	if _, err := s.ReservePublicIngress(changed, time.Now(), ingressAudit()); err == nil {
		t.Fatal("owner silently changed tuple")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.ListPublicIngressReservations(); err != nil || len(got) != 1 || got[0] != reservations[0] {
		t.Fatalf("race result not durable: %+v %v", got, err)
	}
}

func TestPublicIngressReservationFailedSaveRollsBack(t *testing.T) {
	s, spec := reservationStore(t)
	before := stateJSONForTest(t, s)
	diskBefore := diskStateJSONForTest(t, s)
	path := s.path
	s.path = t.TempDir()
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err == nil {
		t.Fatal("save to directory succeeded")
	}
	s.path = path
	if !bytes.Equal(before, stateJSONForTest(t, s)) || !bytes.Equal(diskBefore, diskStateJSONForTest(t, s)) {
		t.Fatal("failed save changed reservation/audit state")
	}
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
}

func TestPublicIngressReservationBlocksLaterTunnelCreation(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	before := stateJSONForTest(t, s)
	_, err := s.CreateTunnel(TunnelRequest{IRNode: "src", EXNode: "dst", RouteListen: "127.0.0.1:54619"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "reserved for public ingress") {
		t.Fatalf("new tunnel stole desired public port: %v", err)
	}
	if !bytes.Equal(before, stateJSONForTest(t, s)) {
		t.Fatal("blocked tunnel creation changed state")
	}
}

func TestPublicIngressReservationEncryptedBackupAndOlderRestore(t *testing.T) {
	s, spec := reservationStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(t.TempDir(), "before.baftbak")
	if _, err := app.BackupToFile(older, backupTestKey(), time.Now()); err != nil {
		t.Fatal(err)
	}
	reservation, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.FlushSecurityAuditIntents(); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(t.TempDir(), "after.baftbak")
	if _, err := app.BackupToFile(newer, backupTestKey(), time.Now()); err != nil {
		t.Fatal(err)
	}
	freshStore, err := OpenStore(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := NewServer(freshStore, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.RestoreFromFile(newer, backupTestKey()); err != nil {
		t.Fatal(err)
	}
	if got, _ := freshStore.ListPublicIngressReservations(); len(got) != 1 || got[0] != reservation {
		t.Fatalf("encrypted restore lost reservation: %+v", got)
	}
	preview, err := app.PreviewRestore(older, backupTestKey())
	if err != nil || preview.WouldRefuse != "" {
		t.Fatalf("old backup preview: %+v %v", preview, err)
	}
	if err := app.RestoreFromFile(older, backupTestKey()); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.ListPublicIngressReservations(); len(got) != 1 || got[0] != reservation {
		t.Fatalf("old backup freed current port: %+v", got)
	}
	conflict := reservation
	conflict.Spec.PublicPort++
	restored := state{PublicIngressReservations: map[string]PublicIngressReservation{spec.TunnelID: conflict}}
	if err := mergePublicIngressReservations(&restored, s.st); err == nil {
		t.Fatal("conflicting backup tuple accepted")
	}
}

func TestPublicIngressReservationsAPIAuthAndDesiredOnly(t *testing.T) {
	s, spec := reservationStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "tok-src"} {
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/api/ingress/reservations", token, spec))
		if rr.Code != 401 {
			t.Fatalf("unauthorized reserve: %d", rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/api/ingress/reservations", "admin", spec))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"desired_only":true`) || !strings.Contains(rr.Body.String(), `"ownership_verified":false`) || !strings.Contains(rr.Body.String(), `"ready_to_apply":false`) {
		t.Fatalf("reservation API: %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, authReq(http.MethodGet, "/api/ingress/reservations", "admin", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), spec.TunnelID) {
		t.Fatalf("reservation list: %d %s", rr.Code, rr.Body.String())
	}
	access := newAccessRig(t)
	cookie, _ := access.session()
	req := httptest.NewRequest(http.MethodPost, access.base()+"api/ingress/reservations", strings.NewReader(`{}`))
	req.AddCookie(cookie)
	if got := access.do(req); got.Code != 401 {
		t.Fatalf("missing CSRF accepted: %d", got.Code)
	}
}

func TestPublicIngressReservationOldStateDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(path, []byte(`{"nodes":{},"jobs":{},"next_job":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if reservations, err := s.ListPublicIngressReservations(); err != nil || len(reservations) != 0 {
		t.Fatalf("old state migration: %+v %v", reservations, err)
	}
	var payload state
	if err := json.Unmarshal([]byte(`{"nodes":{}}`), &payload); err != nil {
		t.Fatal(err)
	}
	normalizeState(&payload)
	if payload.PublicIngressReservations == nil {
		t.Fatal("old backup reservation defaults missing")
	}
}

func TestPublicIngressReservationsGuardSSHNodeAndProbes(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	before := stateJSONForTest(t, s)
	ssh := SSHMigrationRequest{NodeID: "src", OldPort: 22, NewPort: spec.PublicPort, User: "root", Fingerprint: "SHA256:pinned"}
	if _, err := s.PlanSSHMigration(ssh, time.Now()); err == nil {
		t.Fatal("SSH plan took ingress port")
	}
	if _, err := s.StartSSHMigration(ssh, sshPlanHash(ssh), time.Now()); err == nil {
		t.Fatal("SSH start took ingress port")
	}
	if _, err := s.UpsertNode(Node{ID: "src", Role: "worker", Address: "127.0.0.1:54619"}, ""); err == nil {
		t.Fatal("node management address took reserved port")
	}
	if _, err := s.StartPathProbe(PathProbeRequest{SourceNode: "dst", DestinationNode: "src", Family: "4", CandidatePorts: []int{spec.PublicPort}}, time.Now()); err == nil {
		t.Fatal("path probe took reserved port")
	}
	s.mu.Lock()
	_, err := s.createPathProbeLocked("dst", "src", "4", "127.0.0.1", []int{spec.PublicPort}, time.Now())
	s.mu.Unlock()
	if err == nil {
		t.Fatal("automatic path probe took reserved port")
	}
	if !bytes.Equal(before, stateJSONForTest(t, s)) {
		t.Fatal("rejected allocator changed state")
	}
	for _, phase := range []string{SSHMigStaging, SSHMigComplete, SSHMigRolledBack} {
		other, p := reservationStore(t)
		other.st.SSHMigrations = map[string]SSHMigration{"ssh": {ID: "ssh", NodeID: p.IRNode, OldPort: 22, NewPort: p.PublicPort, Phase: phase}}
		if _, err := other.ReservePublicIngress(p, time.Now(), ingressAudit()); err == nil {
			t.Fatalf("ignored %s SSH port", phase)
		}
	}
	other, p := reservationStore(t)
	other.st.PathProbes = map[string]PathProbe{"probe": {ID: "probe", DestinationNode: p.IRNode, Phase: PathProbeQueued, Candidates: []PathProbeCandidate{{Port: p.PublicPort}}}}
	if _, err := other.ReservePublicIngress(p, time.Now(), ingressAudit()); err == nil {
		t.Fatal("reservation ignored pending path probe")
	}
}

func TestPublicIngressReservationsRestoreChecksMergedResources(t *testing.T) {
	s, spec := reservationStore(t)
	reservation, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	for _, which := range []string{"transport", "management", "SSH", "role", "owner", "target"} {
		t.Run(which, func(t *testing.T) {
			restored := state{Nodes: map[string]Node{}, Tunnels: map[string]Tunnel{}, SSHMigrations: map[string]SSHMigration{}}
			switch which {
			case "transport":
				restored.Tunnels["other"] = Tunnel{ID: "other", EXNode: spec.IRNode, Port: spec.PublicPort, Phase: TunnelActive}
			case "management":
				restored.Nodes[spec.IRNode] = Node{ID: spec.IRNode, Role: "worker", Address: "127.0.0.1:54619"}
			case "SSH":
				restored.SSHMigrations["ssh"] = SSHMigration{NodeID: spec.IRNode, OldPort: 22, NewPort: spec.PublicPort}
			case "role":
				restored.Nodes[spec.IRNode] = Node{ID: spec.IRNode, Role: "foreign", Address: "127.0.0.1:22"}
			case "owner":
				restored.Tunnels[spec.TunnelID] = Tunnel{ID: spec.TunnelID, IRNode: "other", RouteListen: spec.LocalTarget, Phase: TunnelActive}
			case "target":
				restored.Tunnels[spec.TunnelID] = Tunnel{ID: spec.TunnelID, IRNode: spec.IRNode, RouteListen: "127.0.0.1:5555", Phase: TunnelActive}
			}
			if err := mergePublicIngressReservations(&restored, s.st); err == nil {
				t.Fatal("restore introduced conflicting live resource")
			}
		})
	}
	protected := reservation
	protected.Spec.PublicPort = 2053
	isolated := state{PublicIngressReservations: map[string]PublicIngressReservation{spec.TunnelID: protected}}
	if err := mergePublicIngressReservations(&isolated, state{}); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("2053 reservation not specifically refused: %v", err)
	}
	orphan := state{}
	if err := mergePublicIngressReservations(&orphan, s.st); err != nil {
		t.Fatal(err)
	}
	if orphan.PublicIngressReservations[spec.TunnelID] != reservation {
		t.Fatal("orphan reservation was released")
	}
}

func TestPublicIngressReservationsAPIRejectsDuplicateFields(t *testing.T) {
	s, _ := reservationStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"ir_node":"src","tunnel_id":"tun-test","public_port":54619,"public_port":54620,"local_target":"127.0.0.1:1443"}`,
		`{"ir_node":"src","tunnel_id":"tun-test","public_port":54619,"local_target":"127.0.0.1:1443","ready":true}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/ingress/reservations", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != 400 {
			t.Fatalf("ambiguous reservation accepted: %d %s", rr.Code, rr.Body.String())
		}
	}
}

func TestPublicIngressReservationStoredSetValidationAcrossNodes(t *testing.T) {
 s,_:=reservationStore(t)
 a:=PublicIngressSpec{IRNode:"src",TunnelID:"tun-a",PublicPort:54619,LocalTarget:"127.0.0.1:1443"}
 b:=PublicIngressSpec{IRNode:"src",TunnelID:"tun-b",PublicPort:54619,LocalTarget:"127.0.0.1:1444"}
 s.mu.Lock()
 s.st.PublicIngressReservations=map[string]PublicIngressReservation{
  "tun-a":{Spec:a,CreatedAt:time.Now()},
  "tun-b":{Spec:b,CreatedAt:time.Now()},
 }
 _,err:=s.publicIngressReservationSpecsLocked()
 s.mu.Unlock()
 if err==nil {t.Fatal("conflicting persisted reservation set was accepted")}
 s.mu.Lock()
 b.IRNode="other-node"
 s.st.PublicIngressReservations["tun-b"]=PublicIngressReservation{Spec:b,CreatedAt:time.Now()}
 // Missing historical owners are allowed, but a second node can use the same public port.
 _,err=s.publicIngressReservationSpecsLocked()
 s.mu.Unlock()
 if err!=nil {t.Fatalf("cross-node reuse should not conflict: %v",err)}
}
