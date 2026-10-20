package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestPublicIngressPreflightLostDeliveryCanBeReplaced(t *testing.T) {
	for _, status := range []string{"queued", "dispatched"} {
		t.Run(status, func(t *testing.T) {
			s, req := ingressPreflightStore(t)
			now := time.Now().UTC()
			old, err := s.CreatePublicIngressPreflight(req, now, ingressAudit())
			if err != nil {
				t.Fatal(err)
			}
			// Model a lost job response/ACK, then restart. A fresh pending
			// inspection still excludes another request for this node.
			s.mu.Lock()
			old.Status, old.CreatedAt = status, now.Add(-publicIngressPreflightPendingLifetime)
			s.st.Jobs[old.ID] = old
			err = s.saveLocked()
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			s, err = OpenStore(s.path)
			if err != nil {
				t.Fatal(err)
			}
			before, err := cloneState(s.st)
			if err != nil {
				t.Fatal(err)
			}
			path := s.path
			s.path = t.TempDir() // Saving to a directory must fail atomically.
			if _, err := s.CreatePublicIngressPreflight(req, now, ingressAudit()); err == nil {
				t.Fatal("save failure accepted")
			}
			if !reflect.DeepEqual(s.st, before) {
				t.Fatal("failed replacement changed job/audit state")
			}
			s.path = path
			replacement, err := s.CreatePublicIngressPreflight(req, now, ingressAudit())
			if err != nil {
				t.Fatal(err)
			}
			if replacement.ID == old.ID || !replacement.CreatedAt.Equal(now) {
				t.Fatal("replacement reused stale identity/time")
			}
			if err := s.AckJobOutput(req.IRNode, "tok-src", old.ID, "succeeded", "late ACK", publicIngressReceiptFixture(req)); err == nil {
				t.Fatal("expired job accepted late ACK")
			}
			if _, err := s.CreatePublicIngressPreflight(req, now, ingressAudit()); err == nil {
				t.Fatal("young pending job was replaced")
			}
			s, err = OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			if s.st.Jobs[old.ID].Status != "failed" || s.st.Jobs[replacement.ID].Status != "queued" {
				t.Fatal("replacement not durable")
			}
			found := false
			for _, a := range s.PendingSecurityAuditIntents() {
				if ids, ok := a.Details["expired_preflight_job_ids"].([]any); ok && len(ids) == 1 && ids[0] == old.ID {
					found = true
				}
			}
			if !found {
				t.Fatal("replacement has no durable expiry audit")
			}
		})
	}
}

func ingressPreflightStore(t *testing.T) (*Store, PublicIngressPreflightRequest) {
	t.Helper()
	s := newPathProbeStore(t)
	if _, err := s.PullJobs("src", "tok-src"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	if s.st.Tunnels == nil {
		s.st.Tunnels = map[string]Tunnel{}
	}
	s.st.Tunnels["tun-test"] = Tunnel{ID: "tun-test", IRNode: "src", EXNode: "dst", RouteListen: "127.0.0.1:1443"}
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return s, PublicIngressPreflightRequest{IRNode: "src", TunnelID: "tun-test", PublicPort: 2453, LocalTarget: "127.0.0.1:1443"}
}

func ingressAudit() AuditEntry { return AuditEntry{Timestamp: time.Now().UTC(), Actor: "admin"} }

func TestPublicIngressPreflightStoreSigningAndAudit(t *testing.T) {
	s, req := ingressPreflightStore(t)
	job, err := s.CreatePublicIngressPreflight(req, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.PendingSecurityAuditIntents()) != 1 {
		t.Fatal("job lacks atomic audit intent")
	}
	if _, err := s.CreatePublicIngressPreflight(req, time.Now(), ingressAudit()); err == nil {
		t.Fatal("duplicate pending preflight accepted")
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	jobs, err := s.PullJobs("src", "tok-src")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pull: %+v %v", jobs, err)
	}
	signed, err := app.signAgentJobs(jobs)
	if err != nil || len(signed) != 1 || signed[0].Signed == nil {
		t.Fatalf("sign: %+v %v", signed, err)
	}
	verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: "src"}).Verify(*signed[0].Signed, time.Now())
	if err != nil || verified.Action != agentjob.ActionPublicIngressPreflight || verified.Params["local_target"] != req.LocalTarget {
		t.Fatalf("signed job: %+v %v", verified, err)
	}
	if _, err := (agentjob.Verifier{BCCKey: pub, NodeID: "dst"}).Verify(*signed[0].Signed, time.Now()); err == nil {
		t.Fatal("different node accepted job")
	}
	if err := s.AckJobOutput("dst", "tok-dst", job.ID, "succeeded", "wrong node", `{"ready_to_apply":true}`); err == nil {
		t.Fatal("wrong node ack accepted")
	}
	validReceipt := publicIngressReceiptFixture(req)
	auditCount := len(s.PendingSecurityAuditIntents())
	for _, bad := range []string{
		`{"ready_to_apply":true,"ownership_verified":true}`,
		strings.Replace(validReceipt, `"ready_to_apply":false`, `"ready_to_apply":true`, 1),
		strings.Replace(validReceipt, `"ownership_verified":false`, `"ownership_verified":true`, 1),
		strings.Replace(validReceipt, `"firewall_policy_verified":false`, `"firewall_policy_verified":true`, 1),
		strings.Replace(validReceipt, `"ready_to_apply":false`, `"ready_to_apply":null`, 1),
		strings.Replace(validReceipt, `"public_listener_count":0`, `"public_listener_count":-1`, 1),
		strings.Replace(validReceipt, `"tunnel_id":"tun-test"`, `"tunnel_id":"different"`, 1),
		strings.Replace(validReceipt, `"public_port":2453`, `"public_port":2054`, 1),
		strings.Replace(validReceipt, `"local_target":"127.0.0.1:1443"`, `"local_target":"127.0.0.1:8443"`, 1),
		strings.Replace(validReceipt, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(validReceipt, `"scope":"agent_network_namespace_snapshot"`, `"scope":"host_verified"`, 1),
		validReceipt + ` {}`,
		strings.TrimSuffix(validReceipt, "}") + `,"ready_to_apply":false}`,
	} {
		if err := s.AckJobOutput("src", "tok-src", job.ID, "succeeded", "untrusted inspection", bad); err == nil {
			t.Fatalf("malformed or authorizing receipt accepted: %s", bad)
		}
		if got := s.ListJobs()[0]; got.Status != "dispatched" || got.Output != "" {
			t.Fatalf("rejected receipt changed job: %+v", got)
		}
		if tunnel, _ := s.GetTunnel(req.TunnelID); tunnel.Phase != "" || len(s.PendingSecurityAuditIntents()) != auditCount {
			t.Fatal("rejected receipt changed tunnel or audit state")
		}
	}
	if err := s.AckJobOutput("src", "tok-src", job.ID, "succeeded", "inspection only", validReceipt); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.ListJobs()[0]
	if got.Status != "succeeded" || got.Output != validReceipt {
		t.Fatalf("receipt not durable: %+v", got)
	}
	tunnel, _ := reopened.GetTunnel(req.TunnelID)
	if tunnel.Phase != "" {
		t.Fatalf("preflight receipt changed tunnel: %+v", tunnel)
	}
}

func TestPublicIngressPreflightRejectsUnsafeDispatch(t *testing.T) {
	for _, which := range []string{"revoked", "offline", "unknown node", "wrong role", "wrong tunnel node", "unknown tunnel", "wrong target", "invalid target", "invalid port", "bad audit"} {
		t.Run(which, func(t *testing.T) {
			s, req := ingressPreflightStore(t)
			audit := ingressAudit()
			s.mu.Lock()
			n := s.st.Nodes["src"]
			switch which {
			case "revoked":
				n.Revoked = true
			case "offline":
				n.AgentSeen = time.Now().Add(-6 * time.Minute)
			case "unknown node":
				req.IRNode = "missing"
			case "wrong role":
				n.Role = "foreign"
			case "wrong tunnel node":
				tunnel := s.st.Tunnels["tun-test"]
				tunnel.IRNode = "dst"
				s.st.Tunnels["tun-test"] = tunnel
			case "unknown tunnel":
				req.TunnelID = "missing"
			case "wrong target":
				req.LocalTarget = "127.0.0.1:8888"
			case "invalid target":
				req.LocalTarget = "192.0.2.5:1443"
			case "invalid port":
				req.PublicPort = 65536
			case "bad audit":
				audit = AuditEntry{}
			}
			s.st.Nodes["src"] = n
			s.mu.Unlock()
			if _, err := s.CreatePublicIngressPreflight(req, time.Now(), audit); err == nil {
				t.Fatal("unsafe request queued")
			}
			if len(s.ListJobs()) != 0 || len(s.PendingSecurityAuditIntents()) != 0 {
				t.Fatal("rejected request left job/audit mutation")
			}
		})
	}
}

func TestPublicIngressPreflightAPIRequiresAdminSigningStrictJSON(t *testing.T) {
	s, req := ingressPreflightStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := release.GenerateKey()
	body, _ := json.Marshal(req)
	for _, tc := range []struct {
		name, token, body string
		sign              bool
		status            int
	}{
		{"agent denied", "tok-src", string(body), true, 401},
		{"unsigned configuration", "admin", string(body), false, 503},
		{"extra field", "admin", strings.TrimSuffix(string(body), "}") + `,"apply":true}`, true, 400},
		{"trailing JSON", "admin", string(body) + ` {}`, true, 400},
		{"accepted", "admin", string(body), true, 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app.ConfigureJobSigning(nil)
			if tc.sign {
				app.ConfigureJobSigning(key)
			}
			rr := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/ingress/preflight", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			app.Handler().ServeHTTP(rr, r)
			if rr.Code != tc.status {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if tc.status == 202 && (!strings.Contains(rr.Body.String(), `"ready_to_apply":false`) || !strings.Contains(rr.Body.String(), `"read_only":true`)) {
				t.Fatal("missing read-only contract")
			}
		})
	}
}

func TestPublicIngressPreflightSessionRequiresCSRF(t *testing.T) {
	r := newAccessRig(t)
	cookie, _ := r.session()
	req := httptest.NewRequest(http.MethodPost, r.base()+"api/ingress/preflight", strings.NewReader(`{}`))
	req.AddCookie(cookie)
	if rr := r.do(req); rr.Code != http.StatusUnauthorized {
		t.Fatalf("write without CSRF status %d", rr.Code)
	}
}

func publicIngressReceiptFixture(req PublicIngressPreflightRequest) string {
	raw, _ := json.Marshal(map[string]any{
		"schema_version": 1, "tunnel_id": req.TunnelID, "public_port": req.PublicPort, "local_target": req.LocalTarget,
		"scope": "agent_network_namespace_snapshot", "socket_inventory_complete": true, "public_port_state": "not_observed", "public_listener_count": 0,
		"target_listener_observed": false, "ownership_verified": false, "firewall_tools": []string{}, "firewall_policy_verified": false, "ready_to_apply": false, "limitations": []string{"observations only"},
	})
	return string(raw)
}

func TestPublicIngressPreflightReceiptAPI(t *testing.T) {
	s, req := ingressPreflightStore(t)
	job, err := s.CreatePublicIngressPreflight(req, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	get := func(token, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/ingress/preflight?id="+id, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, r)
		return rr
	}
	if rr := get("tok-src", job.ID); rr.Code != 401 {
		t.Fatalf("agent read receipt status %d", rr.Code)
	}
	if rr := get("admin", "missing"); rr.Code != 404 {
		t.Fatalf("unknown receipt status %d", rr.Code)
	}
	rr := get("admin", job.ID)
	if rr.Code != 200 || strings.Contains(rr.Body.String(), `"evidence"`) || strings.Contains(rr.Body.String(), `"params"`) {
		t.Fatalf("queued receipt: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := s.PullJobs("src", "tok-src"); err != nil {
		t.Fatal(err)
	}
	if err := s.AckJobOutput("src", "tok-src", job.ID, "succeeded", "private message omitted", publicIngressReceiptFixture(req)); err != nil {
		t.Fatal(err)
	}
	rr = get("admin", job.ID)
	var receipt PublicIngressPreflightReceipt
	if err := json.Unmarshal(rr.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if rr.Code != 200 || receipt.Status != "succeeded" || len(receipt.Evidence) == 0 || !receipt.ReadOnly || receipt.ReadyToApply || strings.Contains(rr.Body.String(), "private message") {
		t.Fatalf("succeeded receipt: %d %s", rr.Code, rr.Body.String())
	}
	s.mu.Lock()
	malformed := s.st.Jobs[job.ID]
	malformed.Output = `{"secret":"must-not-leak","ready_to_apply":true}`
	s.st.Jobs[job.ID] = malformed
	s.mu.Unlock()
	rr = get("admin", job.ID)
	if rr.Code != 500 || strings.Contains(rr.Body.String(), "must-not-leak") {
		t.Fatalf("malformed receipt leaked: %d %s", rr.Code, rr.Body.String())
	}
	s.mu.Lock()
	unrelated := s.st.Jobs[job.ID]
	unrelated.Type = JobDeployBAFT
	unrelated.Params = map[string]string{"secret": "must-not-leak"}
	s.st.Jobs[job.ID] = unrelated
	s.mu.Unlock()
	rr = get("admin", job.ID)
	if rr.Code != 404 || strings.Contains(rr.Body.String(), "must-not-leak") {
		t.Fatalf("unrelated job leaked: %d %s", rr.Code, rr.Body.String())
	}
}

func TestPublicIngressPreflightRejectsNullObservationArrays(t *testing.T) {
 req := PublicIngressPreflightRequest{TunnelID: "tun-a", PublicPort: 14443, LocalTarget: "127.0.0.1:1443"}
 params := map[string]string{"tunnel_id": req.TunnelID, "public_port": "14443", "local_target": req.LocalTarget}
 for _, field := range []string{"firewall_tools", "limitations"} {
  t.Run(field, func(t *testing.T) {
   var evidence map[string]any
   if err := json.Unmarshal([]byte(publicIngressReceiptFixture(req)), &evidence); err != nil {t.Fatal(err)}
   evidence[field] = nil
   payload, err := json.Marshal(evidence)
   if err != nil {t.Fatal(err)}
   if err := validatePublicIngressPreflightReceipt(string(payload), params); err == nil {
    t.Fatalf("null %s observation array accepted", field)
   }
  })
 }
 if err := validatePublicIngressPreflightReceipt(publicIngressReceiptFixture(req), params); err != nil {
  t.Fatalf("valid receipt with explicit arrays rejected: %v", err)
 }
}

func TestPublicIngressPreflightRejectsMalformedObservationEntries(t *testing.T) {
 req := PublicIngressPreflightRequest{TunnelID: "tun-a", PublicPort: 14443, LocalTarget: "127.0.0.1:1443"}
 params := map[string]string{"tunnel_id": req.TunnelID, "public_port": "14443", "local_target": req.LocalTarget}
 cases := []struct{ field string; value any }{
  {"firewall_tools", []any{nil}},
  {"firewall_tools", []string{""}},
  {"firewall_tools", []string{"  "}},
  {"firewall_tools", []string{"iptables\nforged"}},
  {"limitations", []any{nil}},
  {"limitations", []string{""}},
  {"limitations", []string{"observations\rforged"}},
 }
 for _, tc := range cases {
  t.Run(tc.field, func(t *testing.T) {
   var evidence map[string]any
   if err := json.Unmarshal([]byte(publicIngressReceiptFixture(req)), &evidence); err != nil {t.Fatal(err)}
   evidence[tc.field] = tc.value
   payload, err := json.Marshal(evidence)
   if err != nil {t.Fatal(err)}
   if err := validatePublicIngressPreflightReceipt(string(payload), params); err == nil {
    t.Fatalf("invalid %s entries accepted: %s", tc.field, payload)
   }
  })
 }
}

func TestPublicIngressPreflightRequiresExplicitLimitations(t *testing.T) {
 req := PublicIngressPreflightRequest{TunnelID: "tun-a", PublicPort: 14443, LocalTarget: "127.0.0.1:1443"}
 params := map[string]string{"tunnel_id": req.TunnelID, "public_port": "14443", "local_target": req.LocalTarget}
 var evidence map[string]any
 if err := json.Unmarshal([]byte(publicIngressReceiptFixture(req)), &evidence); err != nil {t.Fatal(err)}
 evidence["limitations"] = []string{}
 payload, err := json.Marshal(evidence)
 if err != nil {t.Fatal(err)}
 if err := validatePublicIngressPreflightReceipt(string(payload), params); err == nil {
  t.Fatal("preflight receipt without limitations accepted")
 }
 if err := validatePublicIngressPreflightReceipt(publicIngressReceiptFixture(req), params); err != nil {
  t.Fatalf("receipt with explicit limitations rejected: %v", err)
 }
}
