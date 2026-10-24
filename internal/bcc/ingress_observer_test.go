package bcc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testIngressCapability = "operational-ingress-v1"

// markAgentCapable simulates the authenticated poll of an up-to-date agent.
func markAgentCapable(t *testing.T, s *Store, spec PublicIngressSpec, token string, caps ...string) {
	t.Helper()
	if _, err := s.pullJobsReporting(spec.IRNode, token, caps, nil); err != nil {
		t.Fatal(err)
	}
}

func TestProductionGateOpensOnlyForAuthenticatedCapabilityReport(t *testing.T) {
	s, spec, _, token := heldIngress(t) // no test probe installed
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("never-reported node opened gate: %v", err)
	}
	// Old agent: authenticated poll without any capability report.
	markAgentCapable(t, s, spec, token)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("old agent opened gate: %v", err)
	}
	// Unrelated capability only.
	markAgentCapable(t, s, spec, token, "other-v1")
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("unrelated capability opened gate: %v", err)
	}
	// A wrong token can neither authenticate nor report a capability.
	if _, err := s.pullJobsReporting(spec.IRNode, "wrong-token", []string{testIngressCapability}, nil); !errors.Is(err, ErrAgentAuthentication) {
		t.Fatalf("wrong token accepted: %v", err)
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("unauthenticated report opened gate: %v", err)
	}
	if len(s.st.Jobs) != 0 {
		t.Fatal("closed gate queued a job")
	}
	markAgentCapable(t, s, spec, token, testIngressCapability)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatalf("capable fresh agent did not open gate: %v", err)
	}
}

func TestProductionGateClosesOnStaleRevokedOrDowngradedAgent(t *testing.T) {
	s, spec, _, token := heldIngress(t)
	markAgentCapable(t, s, spec, token, testIngressCapability)
	now := time.Now().UTC()
	// Stale heartbeat.
	if err := s.enqueueOperationalIngress(spec.TunnelID, now.Add(ingressCapabilityMaxAge+time.Second)); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("stale heartbeat opened gate: %v", err)
	}
	// Revoked.
	s.mu.Lock()
	n := s.st.Nodes[spec.IRNode]
	n.Revoked = true
	s.st.Nodes[spec.IRNode] = n
	s.mu.Unlock()
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("revoked node opened gate: %v", err)
	}
	s.mu.Lock()
	n.Revoked = false
	s.st.Nodes[spec.IRNode] = n
	s.mu.Unlock()
	// Downgrade: the next poll without the capability clears it.
	markAgentCapable(t, s, spec, token)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("downgraded agent kept gate open: %v", err)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("retire gate: %v", err)
	}
}

func TestCapabilitiesPersistAcrossReopen(t *testing.T) {
	s, spec, _, token := heldIngress(t)
	markAgentCapable(t, s, spec, token, testIngressCapability)
	re, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := re.st.Nodes[spec.IRNode].Capabilities; len(got) != 1 || got[0] != testIngressCapability {
		t.Fatalf("capability not persisted: %v", got)
	}
}

func TestParseAgentCapabilitiesIsStrict(t *testing.T) {
	good, err := parseAgentCapabilities([]string{"operational-ingress-v1,other-v2"})
	if err != nil || len(good) != 2 {
		t.Fatalf("good: %v %v", good, err)
	}
	if got, err := parseAgentCapabilities(nil); err != nil || len(got) != 0 {
		t.Fatalf("absent header: %v %v", got, err)
	}
	for _, bad := range [][]string{
		{"a", "b"},                 // repeated header
		{"Operational-Ingress-V1"}, // upper case
		{"a,a"},                    // duplicate
		{"a,,b"},                   // empty token
		{"a b"},                    // space inside token
		{"a, b"},                   // space after comma
		{strings.Repeat("a", 65)},  // too long
		{"a,b,c,d,e,f,g,h,i"},      // too many
		{"a;b"}, {"a\nb"}, {"é"}, {""}, {","},
	} {
		if _, err := parseAgentCapabilities(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestAgentJobsHTTPRecordsCapabilityHeaderAndRejectsMalformed(t *testing.T) {
	s, spec, _, token := heldIngress(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	get := func(hdr ...string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/agent/jobs?node_id="+spec.IRNode, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		for _, h := range hdr {
			req.Header.Add(agentCapabilitiesHeader, h)
		}
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if c := get(testIngressCapability); c != http.StatusOK {
		t.Fatalf("status %d", c)
	}
	if !nodeHasIngressCapability(s, spec.IRNode) {
		t.Fatal("header capability not recorded")
	}
	if c := get("BAD CAP"); c != http.StatusBadRequest {
		t.Fatalf("malformed accepted: %d", c)
	}
	if !nodeHasIngressCapability(s, spec.IRNode) {
		t.Fatal("malformed report must not change recorded state")
	}
	if c := get(); c != http.StatusOK || nodeHasIngressCapability(s, spec.IRNode) {
		t.Fatalf("old agent poll must clear the capability (%d)", c)
	}
}

func nodeHasIngressCapability(s *Store, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.st.Nodes[id].Capabilities {
		if c == testIngressCapability {
			return true
		}
	}
	return false
}

// --- independent observer ---

type probeServer struct {
	ln   net.Listener
	mu   sync.Mutex
	seen []string
}

func startProbeServer(t *testing.T, reply func(got []byte, c net.Conn)) *probeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ps := &probeServer{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 256)
				c.SetReadDeadline(time.Now().Add(2 * time.Second))
				n, _ := c.Read(buf)
				ps.mu.Lock()
				ps.seen = append(ps.seen, string(buf[:n]))
				ps.mu.Unlock()
				reply(buf[:n], c)
			}()
		}
	}()
	return ps
}

func echoReply(got []byte, c net.Conn) { c.Write(got) }

func (ps *probeServer) observer(t *testing.T, wantAddr string, dials *int) *IngressObserver {
	o := NewIngressObserver()
	o.timeout = 400 * time.Millisecond
	o.allowLoopback = true
	o.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dials != nil {
			*dials++
		}
		if network != "tcp" || addr != wantAddr {
			t.Errorf("dialled %s %s, want tcp %s", network, addr, wantAddr)
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", ps.ln.Addr().String())
	}
	return o
}

// appliedIngress returns a store whose operation has a dispatched job and an
// exact applied receipt, with the IR node address set for the probe.
func appliedIngress(t *testing.T) (*Store, PublicIngressSpec, string) {
	t.Helper()
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	r := appliedReceipt(cmd)
	op.Receipt = &r
	s.st.PublicIngressOperations[spec.TunnelID] = op
	n := s.st.Nodes[spec.IRNode]
	n.Address = "127.0.0.1"
	s.st.Nodes[spec.IRNode] = n
	s.mu.Unlock()
	return s, spec, token
}

func TestObserverBuildsIndependentObservationFromRealRoundTrip(t *testing.T) {
	s, spec, _ := appliedIngress(t)
	ps := startProbeServer(t, echoReply)
	now := time.Now().UTC()
	var dials int
	o := ps.observer(t, "127.0.0.1:54619", &dials)
	obs, res, err := o.Observe(context.Background(), s, spec.TunnelID, now)
	if err != nil || obs == nil || !res.Connected || !res.PayloadVerified || dials != 1 {
		t.Fatalf("obs=%v res=%+v err=%v dials=%d", obs, res, err, dials)
	}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); !r.Ready || r.Card == nil {
		t.Fatalf("not ready: %+v", r)
	}
	// A second probe uses a different random nonce.
	if _, _, err := o.Observe(context.Background(), s, spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.seen) != 2 || ps.seen[0] == ps.seen[1] || !strings.HasPrefix(ps.seen[0], ingressProbePrefix) {
		t.Fatalf("payloads: %q", ps.seen)
	}
}

func TestObserverNeverFabricatesFromReceiptWhenProbeFails(t *testing.T) {
	cases := map[string]func(got []byte, c net.Conn){
		"silent":      func(got []byte, c net.Conn) { time.Sleep(900 * time.Millisecond) },
		"wrong echo":  func(got []byte, c net.Conn) { c.Write([]byte(strings.Repeat("x", len(got)))) },
		"short echo":  func(got []byte, c net.Conn) { c.Write(got[:len(got)-1]) },
		"closed":      func(got []byte, c net.Conn) {},
		"http-ish":    func(got []byte, c net.Conn) { io.WriteString(c, "HTTP/1.1 302 Found\r\nLocation: http://x/\r\n\r\n") },
		"replay-prev": func(got []byte, c net.Conn) { c.Write([]byte(ingressProbePrefix + strings.Repeat("0", 32) + "\n")) },
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			s, spec, _ := appliedIngress(t)
			ps := startProbeServer(t, reply)
			o := ps.observer(t, "127.0.0.1:54619", nil)
			now := time.Now().UTC()
			start := time.Now()
			obs, res, err := o.Observe(context.Background(), s, spec.TunnelID, now)
			if obs != nil || err == nil || res.PayloadVerified {
				t.Fatalf("fabricated observation: %v %+v %v", obs, res, err)
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("probe not time bounded")
			}
			if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready {
				t.Fatal("ready without observation")
			}
		})
	}
}

func TestObserverRefusesConnectionRefusedAndDoesNotRetry(t *testing.T) {
	s, spec, _ := appliedIngress(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	var dials int
	o := NewIngressObserver()
	o.timeout, o.allowLoopback = 400*time.Millisecond, true
	o.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dials++
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	obs, res, err := o.Observe(context.Background(), s, spec.TunnelID, time.Now())
	if obs != nil || err == nil || res.Connected || dials != 1 {
		t.Fatalf("%v %+v %v dials=%d", obs, res, err, dials)
	}
}

func TestObserverRequiresAppliedReceiptAndNeverDialsWithout(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, _, _ := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	var dials int
	o := NewIngressObserver()
	o.allowLoopback = true
	o.dial = func(context.Context, string, string) (net.Conn, error) { dials++; return nil, errors.New("no") }
	if obs, _, err := o.Observe(context.Background(), s, spec.TunnelID, time.Now()); obs != nil || err == nil || dials != 0 {
		t.Fatalf("receipt-less observation: %v %v dials=%d", obs, err, dials)
	}
	if obs, _, err := o.Observe(context.Background(), s, "missing", time.Now()); obs != nil || err == nil || dials != 0 {
		t.Fatalf("unknown tunnel: %v %v", obs, err)
	}
}

func TestObserverRefusesNonPublicTargetsByDefault(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "0.0.0.0", "::1", "", "localhost", "169.254.1.1", "224.0.0.1", "bad host", "10.0.0.1:99999"} {
		s, spec, _ := appliedIngress(t)
		s.mu.Lock()
		n := s.st.Nodes[spec.IRNode]
		n.Address = addr
		s.st.Nodes[spec.IRNode] = n
		s.mu.Unlock()
		var dials int
		o := NewIngressObserver()
		o.dial = func(context.Context, string, string) (net.Conn, error) { dials++; return nil, errors.New("no") }
		if obs, _, err := o.Observe(context.Background(), s, spec.TunnelID, time.Now()); obs != nil || err == nil || dials != 0 {
			t.Fatalf("%q: dialled or observed (%v %v %d)", addr, obs, err, dials)
		}
	}
}

// --- HTTP readiness API ---

func TestIngressReadinessAPIAuthenticatedAndCardOnlyWhenReady(t *testing.T) {
	s, spec, _ := appliedIngress(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	ps := startProbeServer(t, echoReply)
	app.ingressObserver = ps.observer(t, "127.0.0.1:54619", nil)
	call := func(method, auth, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/ingress/readiness"+query, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := call(http.MethodGet, "", "?tunnel_id="+spec.TunnelID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", rec.Code)
	}
	if rec := call(http.MethodPost, "admin", "?tunnel_id="+spec.TunnelID); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", rec.Code)
	}
	if rec := call(http.MethodGet, "admin", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no tunnel: %d", rec.Code)
	}
	rec := call(http.MethodGet, "admin", "?tunnel_id="+spec.TunnelID)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"ready":true`) || !strings.Contains(body, `"card"`) || !strings.Contains(body, `"config_port":54619`) {
		t.Fatalf("ready response: %d %s", rec.Code, body)
	}
	// Probe fails -> not ready, and no card ever leaks.
	app.ingressObserver = startProbeServer(t, func([]byte, net.Conn) {}).observer(t, "127.0.0.1:54619", nil)
	app.resetIngressObservationCache()
	rec = call(http.MethodGet, "admin", "?tunnel_id="+spec.TunnelID)
	body = rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, `"ready":true`) || strings.Contains(body, `"card"`) || !strings.Contains(body, `"ready":false`) {
		t.Fatalf("not-ready response: %d %s", rec.Code, body)
	}
}
