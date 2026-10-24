//go:build linux

package operational

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

const (
	pairCode  = "BAFTPAIR1:AAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	replyCode = "BAFTREPLY1:BBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

// bccRig is a real BCC (store + HTTP server over TLS) with job signing.
type bccRig struct {
	t     *testing.T
	store *bcc.Store
	app   *bcc.Server
	srv   *httptest.Server
	pki   *pki
	cl    *http.Client
	pub   ed25519.PublicKey
	key   ed25519.PrivateKey
}

func newBCC(t *testing.T, nodes map[string]string) *bccRig {
	t.Helper()
	store, err := bcc.OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for id, role := range nodes {
		addr := "203.0.113." + strconv.Itoa(10+len(id)) + ":22"
		if id == irNode {
			addr = irAddr + ":22"
		}
		if _, err := store.UpsertNode(bcc.Node{ID: id, Address: addr, Role: role}, nodeToken(id)); err != nil {
			t.Fatal(err)
		}
	}
	app, err := bcc.NewServer(store, adminTk)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	pk := newPKI(t, "bcc-ca")
	srv := pk.server(t, app.Handler())
	b := &bccRig{t: t, store: store, app: app, srv: srv, pki: pk, cl: pk.client(), pub: pub, key: key}
	for id := range nodes {
		if jobs := b.pull(id); len(jobs) != 0 { // first authenticated agent contact
			t.Fatalf("unexpected initial jobs for %s: %+v", id, jobs)
		}
	}
	return b
}

// call performs an authenticated request against the real BCC HTTP API.
func (b *bccRig) call(method, path, token string, in any, want int, out any) []byte {
	b.t.Helper()
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			b.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, b.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		b.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := b.cl.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != want {
		b.t.Fatalf("%s %s: status %d want %d: %s", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			b.t.Fatalf("%s: decode: %v: %s", path, err, data)
		}
	}
	return data
}

func (b *bccRig) pull(node string) []bcc.AgentJob {
	b.t.Helper()
	var jobs []bcc.AgentJob
	b.call(http.MethodGet, "/api/agent/jobs?node_id="+url.QueryEscape(node), nodeToken(node), nil, http.StatusOK, &jobs)
	return jobs
}

func (b *bccRig) step(tn bcc.Tunnel, node, action, message, output string) {
	b.t.Helper()
	jobs := b.pull(node)
	if len(jobs) != 1 || jobs[0].Type != action || jobs[0].Params["tunnel_id"] != tn.ID {
		b.t.Fatalf("%s: expected one %s for %s: %+v", node, action, tn.ID, jobs)
	}
	b.call(http.MethodPost, "/api/agent/ack", nodeToken(node), map[string]string{
		"NodeID": node, "JobID": jobs[0].ID, "Status": "succeeded", "Message": message, "Output": output,
	}, http.StatusOK, nil)
}

func (b *bccRig) tunnel(id string) bcc.Tunnel {
	b.t.Helper()
	var tn bcc.Tunnel
	b.call(http.MethodGet, "/api/tunnels?id="+url.QueryEscape(id), adminTk, nil, http.StatusOK, &tn)
	return tn
}

// wizard drives the reviewed-plan Wizard API: plan (read only), create bound
// to the exact plan hash, then the EX/IR node steps until the tunnel is
// ACTIVE. EX/IR tunnel engines are protocol fixtures here (acknowledgements
// through the real signed-job HTTP API); the ingress half is fully real.
func (b *bccRig) wizard(req bcc.TunnelRequest) (bcc.Tunnel, bcc.Plan) {
	b.t.Helper()
	var plan bcc.Plan
	b.call(http.MethodPost, "/api/tunnels/plan", adminTk, req, http.StatusOK, &plan)
	if !plan.OK || plan.Hash == "" || plan.Request.ClientPort != req.ClientPort {
		b.t.Fatalf("plan not deployable / lost client port: gates=%+v", plan.Gates)
	}
	create := plan.Request
	create.PlanHash = plan.Hash
	var tn bcc.Tunnel
	b.call(http.MethodPost, "/api/tunnels", adminTk, create, http.StatusAccepted, &tn)
	if tn.PlanHash != plan.Hash || tn.ClientPort != req.ClientPort {
		b.t.Fatalf("create did not bind the reviewed plan and client port: %+v", tn)
	}
	for _, s := range []struct{ node, action, out string }{
		{tn.EXNode, bcc.JobTunnelPrepareEX, pairCode},
		{tn.IRNode, bcc.JobTunnelPrepareIR, replyCode},
		{tn.EXNode, bcc.JobTunnelCommitEX, ""},
		{tn.IRNode, bcc.JobTunnelCommitIR, ""},
		{tn.IRNode, bcc.JobTunnelHealth, ""},
		{tn.EXNode, bcc.JobTunnelHealth, ""},
	} {
		b.step(tn, s.node, s.action, "ok", s.out)
	}
	b.observe(tn, tn.IRNode, tunnelnode.RoleIR)
	b.observe(tn, tn.EXNode, tunnelnode.RoleEX)
	b.step(tn, tn.IRNode, bcc.JobTunnelFinalize, "finalized", "")
	b.step(tn, tn.EXNode, bcc.JobTunnelFinalize, "finalized", "")
	got := b.tunnel(tn.ID)
	if got.Phase != bcc.TunnelActive {
		b.t.Fatalf("tunnel not active: %s %s", got.Phase, got.Error)
	}
	return got, plan
}

func (b *bccRig) observe(tn bcc.Tunnel, node, role string) {
	b.t.Helper()
	o := tunnelnode.Observed{
		TunnelID: tn.ID, InstanceID: tn.InstanceID, Role: role, Phase: tunnelnode.PhaseCommitted,
		ServiceActive: true, UnitMatches: true, ConfigSHA256: fmt.Sprintf("%064x", 1), UnitSHA256: fmt.Sprintf("%064x", 2), MarkerSHA256: fmt.Sprintf("%064x", 3), RouteID: tn.RouteID,
		Managed: true, MarkerTunnelID: tn.ID, MarkerConfigMatches: true, MarkerUnitMatches: true,
	}
	gen := tn.ExpectedGen[node].To
	if tn.ExpectedGen[node].Bootstrap {
		gen = 4
	}
	o.Generation, o.PreviousGeneration, o.NodeGeneration, o.MarkerGeneration = gen, gen-1, gen, gen
	if role == tunnelnode.RoleIR {
		o.ConfigRole, o.RouteListen = "dialer", tn.RouteListen
		o.PeerAddress = net.JoinHostPort(tn.PublicAddress, strconv.Itoa(tn.Port))
	} else {
		o.ConfigRole, o.Listen, o.Target = "listener", fmt.Sprintf("0.0.0.0:%d", tn.Port), tn.Target
	}
	raw, err := json.Marshal(o)
	if err != nil {
		b.t.Fatal(err)
	}
	b.step(tn, node, bcc.JobTunnelObserve, string(raw), "")
}

// scoped builds a scoped tunnel request.
func scoped(instance, ex, ir string, port, clientPort int, route, exMetrics, irMetrics string) bcc.TunnelRequest {
	return bcc.TunnelRequest{
		InstanceID: instance, EXNode: ex, IRNode: ir, Port: port, ClientPort: clientPort,
		RouteListen: route, EXMetricsListen: exMetrics, IRMetricsListen: irMetrics,
	}
}

// reserve is the reviewed-reservation Wizard API (desired state only).
func (b *bccRig) reserve(tn bcc.Tunnel, planHash string, port, want int) {
	b.t.Helper()
	b.call(http.MethodPost, "/api/ingress/reviewed-reservations", adminTk,
		map[string]any{"tunnel_id": tn.ID, "plan_hash": planHash, "client_port": port}, want, nil)
}

// stage turns the reviewed reservation into the immutable, held, signed-able
// Apply command: durable operation, canonical binding, staged command, held
// dispatch. Nothing is queued yet (see enqueue).
func (b *bccRig) stageFor(tn bcc.Tunnel, jobID string, validity time.Duration) ingressauthority.CommandV1 {
	b.t.Helper()
	now := time.Now().UTC()
	spec, err := bcc.PlanPublicIngress(tn.IRNode, tn.ID, tn.ClientPort, tn.RouteListen)
	if err != nil {
		b.t.Fatal(err)
	}
	manifest := manifestSpec(spec, tn.InstanceID, 1)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		b.t.Fatal(err)
	}
	proof := bcc.PublicIngressExpectedProof{Generation: 1, ManifestDigest: preview.ManifestSHA256, ListenerID: "listener-1", FirewallID: "firewall-1"}
	op, err := b.store.RecordReviewedPublicIngressOperation(spec, tn.PlanHash, proof, now, bcc.AuditEntry{Timestamp: now, Actor: "admin"})
	if err != nil {
		b.t.Fatalf("record reviewed operation: %v", err)
	}
	cmd := ingressauthority.CommandV1{
		IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: agentjob.ActionPublicIngressApply,
		IssuedAt: now.Truncate(time.Second).Add(-time.Second), ExpiresAt: now.Truncate(time.Second).Add(validity),
		Binding: ingressauthority.BindingV1{
			NodeID: tn.IRNode, OperationID: op.OperationID, LeaseID: op.LeaseID,
			Port: uint16(spec.PublicPort), ConfigPort: uint16(spec.PublicPort),
			ListenEndpoint: manifest.Config.Listen, LocalTarget: manifest.Config.Target, Transport: "dedicated",
			PlanSHA256: tn.PlanHash, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID, Spec: manifest, Proof: preview,
		},
	}
	if err := b.store.StageReviewedIngressCommand(tn.ID, cmd, now); err != nil {
		b.t.Fatalf("stage command: %v", err)
	}
	if _, err := b.store.StageHeldIngressDispatch(tn.ID, now); err != nil {
		b.t.Fatalf("stage held dispatch: %v", err)
	}
	return cmd
}

func manifestSpec(spec bcc.PublicIngressSpec, instance string, generation uint64) ingressmanifest.Spec {
	return ingressmanifest.Spec{TunnelID: spec.TunnelID, InstanceID: instance, Generation: generation, Config: ingressmanifest.Config{
		Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(spec.PublicPort)), Target: spec.LocalTarget, MaxClients: 128, DialTimeoutMS: 3000,
	}}
}

// readiness reads GET /api/ingress/readiness (BCC's own probe + receipt).
func (b *bccRig) readiness(tunnelID string) bcc.IngressReadinessView {
	b.t.Helper()
	var v bcc.IngressReadinessView
	b.call(http.MethodGet, "/api/ingress/readiness?tunnel_id="+url.QueryEscape(tunnelID), adminTk, nil, http.StatusOK, &v)
	return v
}

func (b *bccRig) op(tunnelID string) bcc.PublicIngressOperation {
	op, ok := b.store.GetPublicIngressOperation(tunnelID)
	if !ok {
		b.t.Fatalf("no operation for %s", tunnelID)
	}
	return op
}

// ---------------- Agent side ----------------

// nftFaults wraps the real nft command runner to inject failures at exact
// points of a transaction (all other commands reach the real kernel).
type nftFaults struct {
	ingressownership.NFTCommandRunner
	failAdd      bool // add executed by the kernel, then an error is reported
	failDelete   bool // delete not executed, error reported
	deleteCalled int
}

func (r *nftFaults) Run(ctx context.Context, args ...string) error {
	if len(args) > 0 && args[0] == "delete" {
		r.deleteCalled++
		if r.failDelete {
			return fmt.Errorf("LAB injected failure before kernel delete")
		}
	}
	if err := r.NFTCommandRunner.Run(ctx, args...); err != nil {
		return err
	}
	if r.failAdd && len(args) > 0 && args[0] == "add" {
		r.failAdd = false
		return fmt.Errorf("LAB injected response failure after kernel accepted add")
	}
	return nil
}

type journalFaults struct {
	ingressownership.MutationJournal
	failPhase ingressownership.MutationPhase
}

func (j *journalFaults) Advance(r ingressownership.MutationRecord) error {
	if j.failPhase != "" && r.Phase == j.failPhase {
		return fmt.Errorf("LAB injected journal failure at %s", r.Phase)
	}
	return j.MutationJournal.Advance(r)
}

// agentRig is one Agent "process": real Agent + real OperationalExecutor over
// the real reservation store, durable journal, runtime and nft backend.
type agentRig struct {
	t      *testing.T
	b      *bccRig
	h      *host
	dir    string
	Agent  *agent.Agent
	Exec   *ingressownership.OperationalExecutor
	Store  *ingressauthority.ReservationStore
	Runner *nftFaults
	J      *journalFaults
	D      *ingressownership.DurableJournal
	proc   context.Context
	stop   context.CancelFunc
	client *http.Client
	token  string
	node   string
}

type agentOpt struct {
	node   string
	token  string
	client *http.Client
	key    ed25519.PublicKey
	url    string
}

// startAgent builds a fresh Agent "process" over the durable state in dir.
func startAgent(t *testing.T, b *bccRig, h *host, dir string, o *agentOpt) *agentRig {
	t.Helper()
	r := &agentRig{t: t, b: b, h: h, dir: dir, node: irNode, token: nodeToken(irNode), client: b.cl}
	pub, bccURL := b.pub, b.srv.URL
	if o != nil {
		if o.node != "" {
			r.node = o.node
		}
		if o.token != "" {
			r.token = o.token
		}
		if o.client != nil {
			r.client = o.client
		}
		if o.key != nil {
			pub = o.key
		}
		if o.url != "" {
			bccURL = o.url
		}
	}
	r.proc, r.stop = context.WithCancel(context.Background())
	mk := func(name string) string {
		p := filepath.Join(dir, name)
		if err := mkdirPrivate(p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	store, err := ingressauthority.OpenReservationStore(mk("authority"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := ingressownership.OpenDurableJournal(mk("journal"))
	if err != nil {
		t.Fatal(err)
	}
	r.Store = store
	r.D = journal
	r.J = &journalFaults{MutationJournal: journal}
	r.Runner = &nftFaults{NFTCommandRunner: ingressownership.NFTCommandRunner{Binary: "/usr/sbin/nft"}}
	backend := &ingressownership.NFTLabBackend{Runner: r.Runner, ExpectedChainHandle: h.handle}
	r.Exec, err = ingressownership.NewOperationalExecutor(ingressownership.NewRuntime(), backend, r.J)
	if err != nil {
		t.Fatal(err)
	}
	r.Agent, err = agent.New(agent.Config{
		BCCURL: bccURL, NodeID: r.node, Token: r.token, BCCJobKey: pub, StateDir: mk("state"), HTTP: r.client,
		IngressOperational: &agent.IngressOperational{Store: store, Executor: r.Exec},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.kill)
	return r
}

// kill simulates the Agent process dying: every socket it owned closes, while
// the nft rules, reservation store, journal and seen-job ledger stay on disk.
func (r *agentRig) kill() { r.stop() }

// runOnce polls once (RunOnce) under the process context.
func (r *agentRig) runOnce() []agent.Result {
	r.t.Helper()
	res, err := r.Agent.RunOnce(r.proc)
	if err != nil {
		r.t.Fatalf("RunOnce: %v", err)
	}
	return res
}

// openGate has the Agent perform one authenticated poll so BCC records its
// operational-ingress-v1 capability (the production enqueue gate).
func (r *agentRig) openGate() {
	r.t.Helper()
	if jobs := r.runOnce(); len(jobs) != 0 {
		r.t.Fatalf("unexpected jobs while opening the gate: %+v", jobs)
	}
}

// release/signing helper for forged jobs.
func (b *bccRig) sign(j agentjob.Job) release.Envelope {
	b.t.Helper()
	// release.Sign directly: agentjob.Sign would refuse (Validate) the very
	// malformed jobs a compromised signer or key thief could still emit.
	j.SchemaVersion = agentjob.SchemaVersion
	j.IssuedAt, j.ExpiresAt = j.IssuedAt.UTC().Truncate(time.Second), j.ExpiresAt.UTC().Truncate(time.Second)
	payload, err := json.Marshal(j)
	if err != nil {
		b.t.Fatal(err)
	}
	return release.Sign(agentjob.PayloadType, payload, b.key)
}
