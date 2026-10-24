package bcc

// Release gate operational-ingress-v1 (tests/release/opgate/manifest.v1.json):
// the four BCC-side entries. They use only real code paths (HTTP router,
// persistent store, signing, receiver, observer, a real forwarder); the only
// in-package seam is the observer's loopback allowance for the physical-proof
// test, which production cannot reach.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/publicingress"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

// stageGateIngress records the durable reviewed operation for an ACTIVE
// reviewed tunnel and stages the canonical Apply command plus held dispatch.
func stageGateIngress(t *testing.T, s *Store, tn Tunnel, jobID string) ingressauthority.CommandV1 {
	t.Helper()
	spec, err := PlanPublicIngress(tn.IRNode, tn.ID, tn.ClientPort, tn.RouteListen)
	if err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	now := time.Now().UTC()
	op, err := s.RecordReviewedPublicIngressOperation(spec, tn.PlanHash, proof, now, ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	manifest := publicIngressManifestSpec(spec, tn.InstanceID, proof.Generation)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		t.Fatal(err)
	}
	cmd := ingressauthority.CommandV1{IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: "public_ingress_apply",
		IssuedAt: now.Truncate(time.Second).Add(-time.Second), ExpiresAt: now.Truncate(time.Second).Add(time.Minute),
		Binding: ingressauthority.BindingV1{NodeID: spec.IRNode, OperationID: op.OperationID, LeaseID: op.LeaseID,
			Port: uint16(spec.PublicPort), ConfigPort: uint16(spec.PublicPort), ListenEndpoint: manifest.Config.Listen, LocalTarget: manifest.Config.Target,
			Transport: "dedicated", PlanSHA256: tn.PlanHash, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID, Spec: manifest, Proof: preview}}
	if err := s.StageReviewedIngressCommand(tn.ID, cmd, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StageHeldIngressDispatch(tn.ID, now); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// The reviewed plan carries ONE client port; the same numeric port is the
// reservation's public port, the staged command's config port and listener
// port, and the signed job's public_port/listen_endpoint. A different port or
// plan hash never gets that far.
func TestOperationalIngressV1Gate_WizardReviewedPlanBindsConfigPortToPublicPort(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	const port = 54619
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	req.ClientPort = port
	plan := f.plan(req)
	if plan.Request.ClientPort != port {
		t.Fatalf("plan lost the client port: %+v", plan.Request)
	}
	tn := f.activate(f.create(plan))
	if tn.ClientPort != port || tn.PlanHash != plan.Hash {
		t.Fatalf("active tunnel not bound to the reviewed plan/port: %+v", tn)
	}

	// Desired-only reservation through the reviewed-reservation API.
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"wrong plan hash", map[string]any{"tunnel_id": tn.ID, "plan_hash": strings.Repeat("0", 64), "client_port": port}},
		{"different port", map[string]any{"tunnel_id": tn.ID, "plan_hash": plan.Hash, "client_port": port + 1}},
		{"ssh port", map[string]any{"tunnel_id": tn.ID, "plan_hash": plan.Hash, "client_port": 22}},
		{"unknown tunnel", map[string]any{"tunnel_id": "nope", "plan_hash": plan.Hash, "client_port": port}},
	} {
		f.call(http.MethodPost, "/api/ingress/reviewed-reservations", tunnelHTTPAdmin, tc.body, http.StatusBadRequest, nil)
		if got, _ := f.store.ListPublicIngressReservations(); len(got) != 0 {
			t.Fatalf("%s persisted a reservation: %+v", tc.name, got)
		}
	}
	var reserved struct {
		Reservation       PublicIngressReservation `json:"reservation"`
		DesiredOnly       bool                     `json:"desired_only"`
		OwnershipVerified bool                     `json:"ownership_verified"`
		ReadyToApply      bool                     `json:"ready_to_apply"`
	}
	f.call(http.MethodPost, "/api/ingress/reviewed-reservations", tunnelHTTPAdmin,
		map[string]any{"tunnel_id": tn.ID, "plan_hash": plan.Hash, "client_port": port}, http.StatusOK, &reserved)
	if reserved.Reservation.Spec.PublicPort != port || !reserved.DesiredOnly || reserved.OwnershipVerified || reserved.ReadyToApply {
		t.Fatalf("reservation must be desired-only and bound to the client port: %+v", reserved)
	}

	// Staged command and the signed job derive from that exact reviewed state.
	cmd := stageGateIngress(t, f.store, tn, "gate-wizard-apply-1")
	b := cmd.Binding
	if int(b.ConfigPort) != port || int(b.Port) != port || b.ListenEndpoint != "0.0.0.0:"+strconv.Itoa(port) || b.PlanSHA256 != plan.Hash || b.NodeID != tn.IRNode {
		t.Fatalf("staged binding does not carry the reviewed plan and port: %+v", b)
	}
	// Open the production gate exactly like an up-to-date Agent (authenticated
	// poll declaring the capability), then queue and fetch the signed job.
	markAgentCapable(t, f.store, PublicIngressSpec{IRNode: tn.IRNode}, "tok-ir-1", testIngressCapability)
	if err := f.store.EnqueueOperationalIngress(tn.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobs := f.pull("ir-1") // verifies the BCC signature and params
	if len(jobs) != 1 {
		t.Fatalf("expected one signed job, got %+v", jobs)
	}
	p := jobs[0].Params
	if jobs[0].Type != "public_ingress_apply" || p["public_port"] != strconv.Itoa(port) || p["listen_endpoint"] != "0.0.0.0:"+strconv.Itoa(port) ||
		p["plan_sha256"] != plan.Hash || p["operation_id"] != b.OperationID || p["lease_id"] != b.LeaseID || p["ingress_protocol"] != "1" {
		t.Fatalf("signed job does not carry the reviewed binding: %+v", p)
	}
}

// A plan that fails to build (or a create that does not match a reviewed
// plan) must leave no tunnel, no job, no reservation and no operation behind,
// in memory and after re-opening the persisted store.
func TestOperationalIngressV1Gate_WizardFailedBuildPlanCreatesNoDurableJobs(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	good := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	good.ClientPort = 54619
	reviewed := f.plan(good)
	// f.plan proved plan() is read-only; now every defective request.
	bad := map[string]func(r *TunnelRequest){
		"ssh client port":          func(r *TunnelRequest) { r.ClientPort = 22 },
		"aliases own route":        func(r *TunnelRequest) { r.ClientPort = 1443 },
		"aliases own metrics":      func(r *TunnelRequest) { r.ClientPort = 9301 },
		"out of range":             func(r *TunnelRequest) { r.ClientPort = 70000 },
		"unknown IR":               func(r *TunnelRequest) { r.IRNode = "ir-missing" },
		"unknown EX":               func(r *TunnelRequest) { r.EXNode = "ex-missing" },
		"EX equals IR":             func(r *TunnelRequest) { r.IRNode = r.EXNode },
		"non-loopback route":       func(r *TunnelRequest) { r.RouteListen = "203.0.113.9:1443" },
		"control char in instance": func(r *TunnelRequest) { r.InstanceID = "bad\ninstance" },
		"unsupported engine":       func(r *TunnelRequest) { r.Engine = "does-not-exist" },
	}
	before := f.domainState(f.store)
	for name, mutate := range bad {
		req := good
		mutate(&req)
		for _, path := range []string{"/api/tunnels/plan", "/api/tunnels"} {
			raw, _ := json.Marshal(req)
			if path == "/api/tunnels" {
				req.PlanHash = reviewed.Hash
				raw, _ = json.Marshal(req)
			}
			r, err := http.NewRequest(http.MethodPost, f.url+path, bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+tunnelHTTPAdmin)
			r.Header.Set("Content-Type", "application/json")
			resp, err := f.client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			var p Plan
			_ = json.NewDecoder(resp.Body).Decode(&p)
			resp.Body.Close()
			planned := path == "/api/tunnels/plan" && resp.StatusCode == http.StatusOK && p.OK
			created := path == "/api/tunnels" && resp.StatusCode/100 == 2
			if planned || created {
				t.Fatalf("%s: %s accepted a defective request (status %d, ok=%v)", name, path, resp.StatusCode, p.OK)
			}
		}
	}
	// Stale / foreign plan hashes cannot create either.
	stale := good
	stale.PlanHash = strings.Repeat("0", 64)
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, stale, http.StatusConflict, nil)
	changed := good
	changed.ClientPort = 54620
	changed.PlanHash = reviewed.Hash // plan hash of a different port
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, changed, http.StatusConflict, nil)

	f.unchanged(before)
	if jobs := f.store.ListJobs(); len(jobs) != 0 {
		t.Fatalf("failed builds created durable jobs: %+v", jobs)
	}
	if got, _ := f.store.ListPublicIngressReservations(); len(got) != 0 {
		t.Fatalf("failed builds persisted reservations: %+v", got)
	}
	reopened, err := OpenStore(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.ListTunnels()) != 0 || len(reopened.ListJobs()) != 0 {
		t.Fatal("failed builds are durable")
	}
	if _, ok := reopened.GetPublicIngressOperation("anything"); ok {
		t.Fatal("operation persisted")
	}
	// The good request still works afterwards (nothing was poisoned).
	f.activate(f.create(reviewed))
}

// The Agent -> BCC receipt path: only a real certificate-validated HTTPS
// connection carrying the enrolled node's credential and an exact binding is
// recorded; every transport, credential or content defect leaves state
// byte-identical.
func TestOperationalIngressV1Gate_AuthenticatedHTTPSReceiptFromEnrolledAgent(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	// A second enrolled node whose credential is valid, but not for this IR.
	if _, err := s.UpsertNode(Node{ID: "ir-other", Address: "203.0.113.9:22", Role: "worker"}, "other-agent-token"); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewTLSServer(app.Handler())
	defer secure.Close()
	plain := httptest.NewServer(app.Handler())
	defer plain.Close()
	receipt := appliedReceipt(cmd)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	post := func(client *http.Client, base, cred string, body []byte, hdr map[string]string) (*http.Response, []byte, error) {
		req, err := http.NewRequest(http.MethodPost, base+"/api/ingress/operational-receipt", bytes.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+cred)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		out := new(bytes.Buffer)
		_, _ = out.ReadFrom(http.MaxBytesReader(nil, resp.Body, 1<<16))
		return resp, out.Bytes(), nil
	}
	trusted := secure.Client()
	trusted.Timeout = 5 * time.Second
	untrusted := &http.Client{Timeout: 5 * time.Second}
	snapshot := func() string {
		b, err := s.snapshotState()
		if err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(b)
		return string(out)
	}
	before := snapshot()
	// 1. transport: plaintext (even claiming to be proxied HTTPS) and an
	//    unvalidated certificate chain never reach the recorder.
	if resp, _, err := post(&http.Client{Timeout: 5 * time.Second}, plain.URL, token, raw, map[string]string{"X-Forwarded-Proto": "https"}); err != nil || resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plaintext receipt: %v %v", resp, err)
	}
	if _, _, err := post(untrusted, secure.URL, token, raw, nil); err == nil {
		t.Fatal("receipt accepted over an unvalidated TLS certificate")
	}
	pool := x509.NewCertPool()
	wrongCA := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	if _, _, err := post(wrongCA, secure.URL, token, raw, nil); err == nil {
		t.Fatal("receipt accepted when BCC's certificate is not in the Agent's trust store")
	}
	// 2. identity: wrong, empty and another node's credential.
	for name, cred := range map[string]string{"wrong": "wrong-agent-token", "empty": "", "other node": "other-agent-token", "admin": "admin"} {
		if resp, _, err := post(trusted, secure.URL, cred, raw, nil); err != nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s credential: %v %v", name, resp, err)
		}
	}
	// 3. content: binding drift, unknown / duplicate fields, trailing data,
	//    oversize. Each refused with state unchanged.
	drift := receipt
	drift.Binding.PlanSHA256 = strings.Repeat("e", 64)
	dr, _ := json.Marshal(drift)
	wrongJob := receipt
	wrongJob.JobID = "other-job"
	wj, _ := json.Marshal(wrongJob)
	for name, body := range map[string][]byte{
		"binding drift": dr, "other job": wj,
		"unknown field": bytes.Replace(raw, []byte(`"phase"`), []byte(`"surprise":1,"phase"`), 1),
		"duplicate key": bytes.Replace(raw, []byte(`"phase":"applied"`), []byte(`"phase":"applied","phase":"applied"`), 1),
		"trailing":      append(append([]byte(nil), raw...), []byte(` {}`)...),
		"oversize":      append(append([]byte(nil), raw...), bytes.Repeat([]byte(" "), 20000)...),
	} {
		if resp, _, err := post(trusted, secure.URL, token, body, nil); err != nil || resp.StatusCode < 400 {
			t.Fatalf("%s accepted: %v %v", name, resp, err)
		}
	}
	if snapshot() != before {
		t.Fatal("a refused receipt changed persisted state")
	}
	// 4. the enrolled Agent over verified HTTPS is recorded, idempotently.
	resp, body, err := post(trusted, secure.URL, token, raw, nil)
	if err != nil || resp.StatusCode != http.StatusAccepted || resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 {
		t.Fatalf("enrolled receipt: %v %v %s", resp, err, body)
	}
	var ack struct{ Recorded, Verified, ReadyToApply, ReconciliationPending bool }
	if err := json.Unmarshal(bytes.ReplaceAll(bytes.ReplaceAll(body, []byte("ready_to_apply"), []byte("ReadyToApply")), []byte("reconciliation_pending"), []byte("ReconciliationPending")), &ack); err != nil || !ack.Recorded || ack.Verified || ack.ReadyToApply {
		t.Fatalf("a receipt must never self-verify: %s", body)
	}
	if resp, _, err := post(trusted, secure.URL, token, raw, nil); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("identical replay: %v %v", resp, err)
	}
	conflicting := receipt
	conflicting.ListenerEvidence.SocketIdentity = "socket:99999"
	cr, _ := json.Marshal(conflicting)
	if resp, _, err := post(trusted, secure.URL, token, cr, nil); err != nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting replay accepted: %v %v", resp, err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, ok := reopened.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Receipt == nil || op.Receipt.Binding != cmd.Binding || op.Receipt.JobID != cmd.JobID || op.Receipt.ListenerEvidence != receipt.ListenerEvidence || op.Status != PublicIngressPending {
		t.Fatalf("receipt not durably bound to the stored command: %+v", op.Receipt)
	}
	// Recording the receipt is NOT readiness.
	if r := reopened.EvaluateIngressReadiness(spec.TunnelID, nil, time.Now()); r.Ready || r.Card != nil {
		t.Fatalf("receipt alone produced readiness: %+v", r)
	}
}

// The configuration card exists only when ALL hold: dispatched signed job,
// exact applied+ownership-verified receipt with complete evidence, AND BCC's
// own fresh nonce round trip through the real forwarder agreeing with the
// receipt. Every earlier stage must show no card.
func TestOperationalIngressV1Gate_VerifiedConfigCardOnlyAfterPhysicalProofAndReceipt(t *testing.T) {
	// The real forwarder fails closed without the kernel's IPv6 TCP table
	// (it must see every family to prove it owns the port).
	if _, err := os.Stat("/proc/net/tcp6"); err != nil {
		if oplab.Gate() {
			t.Fatalf("the LAB needs /proc/net/tcp6: %v", err)
		}
		t.Skipf("no /proc/net/tcp6 (IPv6-less host): %v", err)
	}
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	now := time.Now().UTC()
	const port = 54619
	// A REAL forwarder (the component that answers the BCC nonce probe) in
	// front of a loopback target; the observer is pointed at it.
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fwd, err := publicingress.New(publicingress.Config{Listen: front.Addr().String(), Target: target.Addr().String(), MaxClients: 8, DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- fwd.Serve(ctx, front) }()
	defer func() { cancel(); <-served }()
	var dials int
	observer := func(addr string) *IngressObserver {
		o := NewIngressObserver()
		o.timeout, o.allowLoopback = time.Second, true
		o.dial = func(c context.Context, network, got string) (net.Conn, error) {
			dials++
			if got != "127.0.0.1:"+strconv.Itoa(port) {
				t.Errorf("observer dialed %s, want the node's public ingress port %d", got, port)
			}
			var d net.Dialer
			return d.DialContext(c, network, addr)
		}
		return o
	}
	good := observer(front.Addr().String())
	setAddress := func() {
		s.mu.Lock()
		n := s.st.Nodes[spec.IRNode]
		n.Address = "127.0.0.1"
		s.st.Nodes[spec.IRNode] = n
		s.mu.Unlock()
	}
	setAddress()
	noCard := func(stage string, obs *IndependentIngressObservation) {
		t.Helper()
		if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready || r.Verified || r.Card != nil {
			t.Fatalf("%s: card/readiness without all evidence: %+v", stage, r)
		}
	}
	// 1. held (not queued): nothing, and BCC never dials.
	if obs, _, err := good.Observe(context.Background(), s, spec.TunnelID, now); obs != nil || err == nil || dials != 0 {
		t.Fatalf("observed a held command: %v %v dials=%d", obs, err, dials)
	}
	noCard("held", nil)
	// 2. queued signed job, no Agent receipt: still nothing, still no dial.
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if obs, _, err := good.Observe(context.Background(), s, spec.TunnelID, now); obs != nil || err == nil || dials != 0 {
		t.Fatalf("observed without receipt: %v %v dials=%d", obs, err, dials)
	}
	noCard("queued", nil)
	// 3. failed / blocked receipt: not an applied receipt.
	blocked := appliedReceipt(cmd)
	blocked.Phase, blocked.OwnershipVerified, blocked.FailureCause, blocked.RollbackStatus = "blocked", false, "refused before effects", "not_attempted"
	blocked.ListenerEvidence, blocked.FirewallEvidence = ingressauthority.ListenerEvidenceV1{}, nil
	if err := s.RecordIngressAgentReceipt(blocked, token, now); err != nil {
		t.Fatal(err)
	}
	if obs, _, err := good.Observe(context.Background(), s, spec.TunnelID, now); obs != nil || err == nil || dials != 0 {
		t.Fatalf("observed a blocked receipt: %v %v", obs, err)
	}
	noCard("blocked receipt", nil)

	// Fresh store for the applied-receipt stages.
	s, spec, cmd, token = heldIngress(t)
	setAddress()
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordIngressAgentReceipt(appliedReceipt(cmd), token, now); err != nil {
		t.Fatal(err)
	}
	// 4. receipt only: "awaiting independent verification", no card.
	noCard("receipt only", nil)
	// 5. physical probe against a dead port, a silent socket and a canned
	//    echo never verify.
	dead, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := dead.Addr().String()
	dead.Close()
	if obs, res, err := observer(deadAddr).Observe(context.Background(), s, spec.TunnelID, now); obs != nil || err == nil || res.Connected {
		t.Fatalf("dead port observed: %v %+v", obs, res)
	}
	silent, _ := net.Listen("tcp", "127.0.0.1:0")
	defer silent.Close()
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	if obs, res, err := observer(silent.Addr().String()).Observe(context.Background(), s, spec.TunnelID, now); obs != nil || err == nil || res.PayloadVerified {
		t.Fatalf("silent socket observed: %v %+v", obs, res)
	}
	noCard("failed probes", nil)
	// 6. the real forwarder answers the fresh nonce: physical proof.
	obs, res, err := good.Observe(context.Background(), s, spec.TunnelID, now)
	if err != nil || obs == nil || !res.Connected || !res.PayloadVerified {
		t.Fatalf("real forwarder did not answer the BCC probe: obs=%v res=%+v err=%v", obs, res, err)
	}
	// 7. evidence that disagrees with the receipt is rejected.
	for name, mutate := range map[string]func(o *IndependentIngressObservation){
		"other socket":  func(o *IndependentIngressObservation) { o.SocketIdentity = "socket:1" },
		"other rule":    func(o *IndependentIngressObservation) { o.RuleHandle++ },
		"other chain":   func(o *IndependentIngressObservation) { o.ChainHandle++ },
		"other binding": func(o *IndependentIngressObservation) { o.Binding.PlanSHA256 = strings.Repeat("d", 64) },
		"no payload":    func(o *IndependentIngressObservation) { o.TCPPayloadVerified = false },
		"stale": func(o *IndependentIngressObservation) {
			o.ObservedAt = now.Add(-ingressObservationMaxAge - time.Second)
		},
	} {
		bad := *obs
		mutate(&bad)
		noCard(name, &bad)
	}
	// 8. receipt + matching physical proof => Verified, Ready and the card.
	r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now)
	if !r.Verified || !r.Ready || r.Card == nil || r.Card.TunnelID != spec.TunnelID || r.Card.IRNode != spec.IRNode ||
		int(r.Card.ConfigPort) != spec.PublicPort || int(r.Card.PublicPort) != spec.PublicPort || r.Card.PlanSHA256 != cmd.Binding.PlanSHA256 {
		t.Fatalf("verified card missing/wrong: %+v", r)
	}
	// 9. Readiness is derived, never persisted: a reopened store without a
	//    fresh observation shows no card.
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if r := reopened.EvaluateIngressReadiness(spec.TunnelID, nil, now); r.Ready || r.Card != nil {
		t.Fatalf("readiness persisted across restart: %+v", r)
	}
}
