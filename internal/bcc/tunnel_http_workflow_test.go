package bcc

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

const tunnelHTTPAdmin = "r1-http-admin-token"

// Exercise the actual HTTP router, authentication, signed job dispatch and
// agent acknowledgments on loopback. Agents return fixtures, not real host
// execution or carrier/continuity evidence.
type tunnelHTTPFixture struct {
	t      *testing.T
	store  *Store
	client *http.Client
	url    string
	pub    ed25519.PublicKey
}

func newTunnelHTTPFixture(t *testing.T) *tunnelHTTPFixture {
	t.Helper()
	s := multiInstanceStore(t)
	app, err := NewServer(s, tunnelHTTPAdmin)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	f := &tunnelHTTPFixture{t: t, store: s, client: client, url: server.URL, pub: pub}
	for _, node := range []string{"ex-1", "ir-1", "ex-2", "ir-2"} {
		if jobs := f.pull(node); len(jobs) != 0 {
			t.Fatalf("unexpected initial jobs for %s: %+v", node, jobs)
		}
	}
	return f
}

func (f *tunnelHTTPFixture) call(method, path, token string, input any, status int, output any) []byte {
	f.t.Helper()
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	r, err := http.NewRequest(method, f.url+path, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := f.client.Do(r)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		f.t.Fatal(err)
	}
	if resp.StatusCode != status {
		f.t.Fatalf("%s %s: status=%d, want %d: %s", method, path, resp.StatusCode, status, data)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			f.t.Fatalf("%s: decode response: %v: %s", path, err, data)
		}
	}
	return data
}

func (f *tunnelHTTPFixture) pull(node string) []AgentJob {
	f.t.Helper()
	var jobs []AgentJob
	f.call(http.MethodGet, "/api/agent/jobs?node_id="+url.QueryEscape(node), "tok-"+node, nil, http.StatusOK, &jobs)
	for _, j := range jobs {
		if j.Signed == nil {
			f.t.Fatalf("job %s is unsigned", j.ID)
		}
		v := agentjob.Verifier{BCCKey: f.pub, NodeID: node}
		verified, err := v.Verify(*j.Signed, time.Now().UTC())
		if err != nil || verified.JobID != j.ID || verified.Action != j.Type || verified.NodeID != node || !maps.Equal(verified.Params, j.Params) {
			f.t.Fatalf("job dispatch identity/signature: %+v, %v", verified, err)
		}
	}
	return jobs
}

func (f *tunnelHTTPFixture) get(id string) Tunnel {
	f.t.Helper()
	var tn Tunnel
	f.call(http.MethodGet, "/api/tunnels?id="+url.QueryEscape(id), tunnelHTTPAdmin, nil, http.StatusOK, &tn)
	return tn
}

func (f *tunnelHTTPFixture) plan(req TunnelRequest) Plan {
	f.t.Helper()
	var p Plan
	before := f.domainState(f.store)
	f.call(http.MethodPost, "/api/tunnels/plan", tunnelHTTPAdmin, req, http.StatusOK, &p)
	if !p.OK || p.Hash == "" {
		f.t.Fatalf("plan is not deployable: %+v", p)
	}
	f.unchanged(before)
	return p
}

func (f *tunnelHTTPFixture) create(p Plan) Tunnel {
	f.t.Helper()
	req := p.Request
	req.PlanHash = p.Hash
	var tn Tunnel
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusAccepted, &tn)
	if tn.PlanHash != p.Hash || tn.Phase != TunnelPreparingEX {
		f.t.Fatalf("create did not bind the reviewed plan: %+v", tn)
	}
	return tn
}

func (f *tunnelHTTPFixture) step(tn Tunnel, node, action, message, output string) {
	f.t.Helper()
	jobs := f.pull(node)
	if len(jobs) != 1 || jobs[0].Type != action || jobs[0].Params["tunnel_id"] != tn.ID || jobs[0].Params["instance_id"] != tn.InstanceID {
		f.t.Fatalf("%s: expected one %s for %s/%s: %+v", node, action, tn.ID, tn.InstanceID, jobs)
	}
	f.call(http.MethodPost, "/api/agent/ack", "tok-"+node, map[string]string{
		"NodeID": node, "JobID": jobs[0].ID, "Status": "succeeded", "Message": message, "Output": output,
	}, http.StatusOK, nil)
}

func (f *tunnelHTTPFixture) observe(tn Tunnel, node, role string, wrongTarget bool) {
	f.t.Helper()
	o := goodObserved(tn, role)
	o.InstanceID = tn.InstanceID
	gen := tn.ExpectedGen[node].To
	if tn.ExpectedGen[node].Bootstrap {
		gen = 4
	}
	o.Generation, o.PreviousGeneration, o.NodeGeneration, o.MarkerGeneration = gen, gen-1, gen, gen
	if role == tunnelnode.RoleIR {
		o.PeerAddress = net.JoinHostPort(tn.PublicAddress, strconv.Itoa(tn.Port))
	}
	if wrongTarget {
		o.Target = "192.0.2.99:65000"
	}
	b, err := json.Marshal(o)
	if err != nil {
		f.t.Fatal(err)
	}
	f.step(tn, node, JobTunnelObserve, string(b), "")
}

func (f *tunnelHTTPFixture) runToObservation(tn Tunnel) {
	f.t.Helper()
	for _, step := range []struct{ node, action, output string }{
		{tn.EXNode, JobTunnelPrepareEX, testPairCode},
		{tn.IRNode, JobTunnelPrepareIR, testReplyCode},
		{tn.EXNode, JobTunnelCommitEX, ""},
		{tn.IRNode, JobTunnelCommitIR, ""},
		{tn.IRNode, JobTunnelHealth, ""},
		{tn.EXNode, JobTunnelHealth, ""},
	} {
		f.step(tn, step.node, step.action, "ok", step.output)
	}
}

func (f *tunnelHTTPFixture) activate(tn Tunnel) Tunnel {
	f.t.Helper()
	f.runToObservation(tn)
	f.observe(tn, tn.IRNode, tunnelnode.RoleIR, false)
	f.observe(tn, tn.EXNode, tunnelnode.RoleEX, false)
	f.step(tn, tn.IRNode, JobTunnelFinalize, "finalized", "")
	f.step(tn, tn.EXNode, JobTunnelFinalize, "finalized", "")
	got := f.get(tn.ID)
	if got.Phase != TunnelActive {
		f.t.Fatalf("activation: %s %s", got.Phase, got.Error)
	}
	return got
}

// Domain checks intentionally exclude expected audit/security records.
func (f *tunnelHTTPFixture) domainState(s *Store) string {
	f.t.Helper()
	b, err := json.Marshal(struct {
		Tunnels []Tunnel
		Jobs    []Job
	}{s.ListTunnels(), s.ListJobs()})
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *tunnelHTTPFixture) unchanged(before string) {
	f.t.Helper()
	if f.domainState(f.store) != before {
		f.t.Fatal("rejected request or read-only plan changed tunnels/jobs in RAM")
	}
	reopened, err := OpenStore(f.store.path)
	if err != nil {
		f.t.Fatal(err)
	}
	if f.domainState(reopened) != before {
		f.t.Fatal("rejected request or read-only plan changed persisted tunnels/jobs")
	}
}

func (f *tunnelHTTPFixture) siblingUnchanged(before Tunnel) {
	f.t.Helper()
	want, err := json.Marshal(before)
	if err != nil {
		f.t.Fatal(err)
	}
	got, err := json.Marshal(f.get(before.ID))
	if err != nil || !bytes.Equal(got, want) {
		f.t.Fatalf("sibling tunnel changed: %s, want %s: %v", got, want, err)
	}
	for _, j := range f.store.ListJobs() {
		if j.Params["tunnel_id"] == before.ID && (j.Status == "queued" || j.Status == "dispatched") {
			f.t.Fatalf("operation queued work on active sibling: %+v", j)
		}
	}
}

func TestTunnelHTTPScopedLifecyclePreservesSibling(t *testing.T) {
	for _, tc := range []struct{ name, public, target, edited string }{
		{"IPv4", "203.0.113.8", "192.0.2.40:2443", "192.0.2.40:3443"},
		{"IPv6", "2001:db8::8", "[2001:db8::40]:2443", "[2001:db8::40]:3443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTunnelHTTPFixture(t)
			siblingReq := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
			sibling := f.activate(f.create(f.plan(siblingReq)))
			req := scopedReq("secondary", "ex-2", "ir-1", 9443, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:9302")
			req.Engine, req.PublicAddress, req.Target = "baft", tc.public, tc.target
			original := f.activate(f.create(f.plan(req)))
			f.siblingUnchanged(sibling)
			if original.Target != tc.target || original.PublicAddress != tc.public {
				t.Fatalf("HTTP endpoint changed: %+v", original)
			}

			req.Port, req.Target = 10443, tc.edited
			plan := f.plan(req)
			if plan.Replaces != original.ID {
				t.Fatalf("edit plan replaces %q, want %q", plan.Replaces, original.ID)
			}
			replacement := f.create(plan)
			if got := f.get(original.ID); got.Phase != TunnelActive {
				t.Fatalf("original interrupted before replacement proved active: %s", got.Phase)
			}
			f.siblingUnchanged(sibling)
			replacement = f.activate(replacement)
			if replacement.Target != tc.edited || replacement.RouteListen != original.RouteListen || replacement.InstanceID != original.InstanceID {
				t.Fatalf("edit changed scope or lost target: %+v", replacement)
			}
			if got := f.get(original.ID); got.Phase != TunnelSuperseded {
				t.Fatalf("verified replacement did not supersede its own previous tunnel: %s", got.Phase)
			}
			f.siblingUnchanged(sibling)

			before := f.domainState(f.store)
			var retire DecommissionPlan
			f.call(http.MethodPost, "/api/tunnels/decommission/plan", tunnelHTTPAdmin, map[string]string{"id": replacement.ID}, http.StatusOK, &retire)
			f.unchanged(before)
			if !retire.OK || retire.InstanceID != replacement.InstanceID || len(retire.Nodes) != 2 || retire.Nodes[0].NodeID != replacement.IRNode || retire.Nodes[1].NodeID != replacement.EXNode {
				t.Fatalf("wrong retirement scope/order: %+v", retire)
			}
			f.call(http.MethodPost, "/api/tunnels/decommission", tunnelHTTPAdmin, map[string]string{"id": replacement.ID, "plan_hash": "stale-" + retire.Hash}, http.StatusConflict, nil)
			f.unchanged(before)
			f.call(http.MethodPost, "/api/tunnels/decommission", tunnelHTTPAdmin, map[string]string{"id": replacement.ID, "plan_hash": retire.Hash}, http.StatusAccepted, nil)
			if jobs := f.pull(replacement.EXNode); len(jobs) != 0 {
				t.Fatalf("EX retired before IR acknowledgment: %+v", jobs)
			}
			f.step(replacement, replacement.IRNode, JobTunnelRetire, retireMessage(replacement, replacement.IRNode), "")
			f.step(replacement, replacement.EXNode, JobTunnelRetire, retireMessage(replacement, replacement.EXNode), "")
			if got := f.get(replacement.ID); got.Phase != TunnelDecommissioned {
				t.Fatalf("retirement phase %s: %s", got.Phase, got.Error)
			}
			f.siblingUnchanged(sibling)
			reopened, err := OpenStore(f.store.path)
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := reopened.GetTunnel(replacement.ID); !ok || got.Phase != TunnelDecommissioned {
				t.Fatalf("retirement not durable: %+v", got)
			}
			if got, ok := reopened.GetTunnel(sibling.ID); !ok || got.Phase != TunnelActive {
				t.Fatalf("sibling not active after reopening SQLite: %+v", got)
			}
		})
	}
}

func TestTunnelHTTPRejectsUnauthorizedAndUnavailableEngine(t *testing.T) {
	for _, token := range []string{"", "wrong-admin", "tok-ir-1"} {
		t.Run("unauthorized-"+token, func(t *testing.T) {
			f := newTunnelHTTPFixture(t)
			before := f.domainState(f.store)
			for _, path := range []string{"/api/tunnels/plan", "/api/tunnels", "/api/tunnels/decommission/plan", "/api/tunnels/decommission"} {
				f.call(http.MethodPost, path, token, map[string]string{"id": "unknown", "ex_node": "ex-1", "ir_node": "ir-1"}, http.StatusUnauthorized, nil)
				f.unchanged(before)
			}
		})
	}
	for _, engine := range []string{"frp", "rathole", "backhaul", "wireguard", "ssh-socks", "wss", "quic", "unknown", "BAFT"} {
		t.Run(engine, func(t *testing.T) {
			f := newTunnelHTTPFixture(t)
			before := f.domainState(f.store)
			req := TunnelRequest{Engine: engine, EXNode: "ex-1", IRNode: "ir-1", Target: "192.0.2.40:2443"}
			for _, path := range []string{"/api/tunnels/plan", "/api/tunnels"} {
				if path == "/api/tunnels" {
					req.PlanHash = "invalid-reviewed-hash"
				}
				b := f.call(http.MethodPost, path, tunnelHTTPAdmin, req, http.StatusBadRequest, nil)
				if !strings.Contains(string(b), "engine") {
					t.Fatalf("unexpected rejection: %s", b)
				}
				f.unchanged(before)
			}
		})
	}
}

func TestTunnelHTTPValidatesDestinationAndReviewedHash(t *testing.T) {
	for _, target := range []string{"example.com:443", "2001:db8::40:2443", "192.0.2.40", "192.0.2.40:0", "[2001:db8::40]:65536", "999.1.2.3:443", "[::::]:443"} {
		t.Run(target, func(t *testing.T) {
			f := newTunnelHTTPFixture(t)
			before := f.domainState(f.store)
			req := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Target: target}
			for _, path := range []string{"/api/tunnels/plan", "/api/tunnels"} {
				f.call(http.MethodPost, path, tunnelHTTPAdmin, req, http.StatusBadRequest, nil)
				f.unchanged(before)
			}
		})
	}
	t.Run("changed-target", func(t *testing.T) {
		f := newTunnelHTTPFixture(t)
		req := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Target: "192.0.2.40:2443"}
		plan := f.plan(req)
		changed := plan.Request
		changed.Target, changed.PlanHash = "[2001:db8::40]:3443", plan.Hash
		before := f.domainState(f.store)
		var stale struct{ Plan Plan }
		f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, changed, http.StatusConflict, &stale)
		f.unchanged(before)
		if stale.Plan.Hash == "" || stale.Plan.Hash == plan.Hash || stale.Plan.Request.Target != changed.Target {
			t.Fatalf("stale refusal must return the new target's fresh plan: %+v", stale.Plan)
		}
		if got := f.create(plan); got.Target != req.Target {
			t.Fatalf("failed changed-target attempt consumed/corrupted original plan: %+v", got)
		}
	})
	t.Run("generation-advanced", func(t *testing.T) {
		f := newTunnelHTTPFixture(t)
		plan := f.plan(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Target: "192.0.2.40:2443"})
		f.store.mu.Lock()
		n := f.store.st.Nodes["ex-1"]
		n.AppliedGeneration = 7
		f.store.st.Nodes[n.ID] = n
		err := f.store.saveLocked()
		f.store.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		req := plan.Request
		req.PlanHash = plan.Hash
		before := f.domainState(f.store)
		var stale struct{ Plan Plan }
		f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusConflict, &stale)
		f.unchanged(before)
		if stale.Plan.Hash == "" || stale.Plan.Hash == plan.Hash || stale.Plan.Nodes[0].GenerationFrom != 7 {
			t.Fatalf("generation change did not invalidate reviewed hash: %+v", stale.Plan)
		}
	})
}

func TestTunnelHTTPPlanDoesNotReserveAListener(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	a := scopedReq("first", "ex-1", "ir-1", 9443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	b := scopedReq("second", "ex-1", "ir-2", 9443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9301")
	first, second := f.plan(a), f.plan(b)
	created := f.create(first)
	req := second.Request
	req.PlanHash = second.Hash
	before := f.domainState(f.store)
	data := f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusBadRequest, nil)
	if !strings.Contains(string(data), "EX listener port") {
		t.Fatalf("unexpected conflict: %s", data)
	}
	f.unchanged(before)
	if got := f.get(created.ID); got.PlanHash != first.Hash || got.Phase != TunnelPreparingEX {
		t.Fatalf("losing deploy corrupted first reviewed deployment: %+v", got)
	}
}

func TestTunnelHTTPWrongObservedTargetRollsBackReplacement(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	req.Target = "192.0.2.40:2443"
	original := f.activate(f.create(f.plan(req)))
	siblingReq := scopedReq("secondary", "ex-2", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:9302")
	sibling := f.activate(f.create(f.plan(siblingReq)))
	req.Port, req.Target = 9443, "[2001:db8::40]:3443"
	replacement := f.create(f.plan(req))
	f.runToObservation(replacement)
	f.observe(replacement, replacement.IRNode, tunnelnode.RoleIR, false)
	f.observe(replacement, replacement.EXNode, tunnelnode.RoleEX, true)
	got := f.get(replacement.ID)
	if got.Phase != TunnelRollingBack || !strings.Contains(got.Error, "target") {
		t.Fatalf("wrong target accepted or failed without rollback: %+v", got)
	}
	f.siblingUnchanged(original)
	f.siblingUnchanged(sibling)
	f.step(replacement, replacement.IRNode, JobTunnelRollback, "restored", "")
	f.step(replacement, replacement.EXNode, JobTunnelRollback, "restored", "")
	if got := f.get(replacement.ID); got.Phase != TunnelRolledBack {
		t.Fatalf("rollback did not complete: %+v", got)
	}
	f.siblingUnchanged(original)
	f.siblingUnchanged(sibling)
}

func TestTunnelHTTPCreateRequiresReviewedPlanHash(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	req := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 8443, Target: "127.0.0.1:443"}
	before := f.domainState(f.store)
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusBadRequest, nil)
	f.unchanged(before)
}
