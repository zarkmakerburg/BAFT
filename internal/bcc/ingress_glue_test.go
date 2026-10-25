package bcc

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// Hand-staging tests opt out of the production glue; glue tests opt in.
func init() { ingressGlueDisabled.Store(true) }

func enableIngressGlue(t *testing.T) {
	t.Helper()
	ingressGlueDisabled.Store(false)
	t.Cleanup(func() { ingressGlueDisabled.Store(true) })
}

type ingressView struct {
	Tunnel
	Ingress *IngressOperationalState `json:"ingress"`
}

func (f *tunnelHTTPFixture) view(id string) ingressView {
	f.t.Helper()
	var v ingressView
	f.call(http.MethodGet, "/api/tunnels?id="+url.QueryEscape(id), tunnelHTTPAdmin, nil, http.StatusOK, &v)
	return v
}

// pullCaps is an authenticated agent poll that declares capabilities.
func (f *tunnelHTTPFixture) pullCaps(node string, caps string) []AgentJob {
	f.t.Helper()
	r, err := http.NewRequest(http.MethodGet, f.url+"/api/agent/jobs?node_id="+url.QueryEscape(node), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer tok-"+node)
	if caps != "" {
		r.Header.Set(agentCapabilitiesHeader, caps)
	}
	resp, err := f.client.Do(r)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.t.Fatalf("poll status %d", resp.StatusCode)
	}
	return nil
}

func glueTunnel(f *tunnelHTTPFixture, port int) Tunnel {
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	req.ClientPort = port
	return f.activate(f.create(f.plan(req)))
}

func applyJobs(s *Store) (n int) {
	for _, j := range s.ListJobs() {
		if j.Type == agentjob.ActionPublicIngressApply {
			n++
		}
	}
	return n
}

// Activation stages the command + held dispatch atomically and creates NO job;
// the gate is closed, so the user sees "held" (not Ready); a later capability
// heartbeat enqueues exactly one signed Apply, idempotently.
func TestGlueActivationStagesHeldThenCapabilityHeartbeatEnqueues(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619)

	op, ok := f.store.GetPublicIngressOperation(tn.ID)
	if !ok || op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != ingressPhaseHeld || op.Dispatch.JobID != op.Command.JobID {
		t.Fatalf("activation did not stage command + held dispatch: %+v", op)
	}
	if int(op.Command.Binding.Port) != 54619 || op.Command.Binding.PlanSHA256 != tn.PlanHash {
		t.Fatalf("staged command not bound to the reviewed plan/port: %+v", op.Command.Binding)
	}
	if applyJobs(f.store) != 0 {
		t.Fatal("staging created a durable apply job while the gate is closed")
	}
	v := f.view(tn.ID)
	if v.Ingress == nil || v.Ingress.State != "held" || v.Ingress.Ready || v.Ingress.Reason == "" {
		t.Fatalf("closed gate must be visibly held and not Ready: %+v", v.Ingress)
	}
	// Staging is durable.
	re, err := OpenStore(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := re.GetPublicIngressOperation(tn.ID); !ok || o.Dispatch == nil || o.Command == nil {
		t.Fatal("staged operation not durable")
	}

	// A poll without the capability keeps it held.
	f.pullCaps("ir-1", "")
	if applyJobs(f.store) != 0 || f.view(tn.ID).Ingress.State != "held" {
		t.Fatal("old agent opened the gate")
	}
	// Capability heartbeat: queued exactly once, idempotent on further polls.
	f.pullCaps("ir-1", testIngressCapability)
	f.pullCaps("ir-1", testIngressCapability)
	if applyJobs(f.store) != 1 {
		t.Fatalf("expected exactly one apply job, got %d", applyJobs(f.store))
	}
	if st := f.view(tn.ID).Ingress; st.State != "queued" || st.Ready {
		t.Fatalf("after heartbeat: %+v", st)
	}
}

// With the gate already open at activation the Apply is queued immediately.
func TestGlueOpenGateAtActivationQueuesApply(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619)
	f.pullCaps("ir-1", testIngressCapability) // first capable heartbeat after activation
	if applyJobs(f.store) != 1 || f.view(tn.ID).Ingress.State != "queued" {
		t.Fatalf("apply not queued: jobs=%d view=%+v", applyJobs(f.store), f.view(tn.ID).Ingress)
	}
}

// Wizard failure (cancelled before activation) leaves no operation and no
// ingress job behind.
func TestGlueFailedWizardCreatesNoIngressOperationOrJob(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	req.ClientPort = 54619
	tn := f.create(f.plan(req))
	f.call(http.MethodPost, "/api/tunnels/cancel", tunnelHTTPAdmin, map[string]string{"id": tn.ID, "reason": "test"}, http.StatusOK, nil)
	if _, ok := f.store.GetPublicIngressOperation(tn.ID); ok || applyJobs(f.store) != 0 {
		t.Fatal("failed wizard left ingress state")
	}
	if got, _ := f.store.ListPublicIngressReservations(); len(got) != 0 {
		t.Fatalf("failed wizard left reservations: %+v", got)
	}
}

// Replacing a tunnel with an applied ingress queues a Retire with a NEW JobID;
// a closed gate refuses the replacement before anything durable is created.
func TestGlueReplacementRetiresAppliedIngressFailClosed(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	old := glueTunnel(f, 54619)
	f.pullCaps("ir-1", testIngressCapability)
	op, _ := f.store.GetPublicIngressOperation(old.ID)
	if err := f.store.RecordIngressAgentReceipt(appliedReceipt(*op.Command), "tok-ir-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if st := f.view(old.ID).Ingress; st.State != "applied" || st.Ready {
		t.Fatalf("applied but must not be Ready without independent evidence: %+v", st)
	}

	edit := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	edit.ClientPort = 54620
	plan := f.plan(edit)
	if plan.Replaces != old.ID {
		t.Fatalf("not a replacement: %+v", plan)
	}
	req := plan.Request
	req.PlanHash = plan.Hash

	// Gate closed (stale heartbeat): replacement refused, nothing created.
	f.store.mu.Lock()
	n := f.store.st.Nodes["ir-1"]
	n.AgentSeen = time.Now().UTC().Add(-time.Hour)
	f.store.st.Nodes["ir-1"] = n
	f.store.mu.Unlock()
	tunnelsBefore, jobsBefore := len(f.store.ListTunnels()), len(f.store.ListJobs())
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusConflict, nil)
	if len(f.store.ListTunnels()) != tunnelsBefore || len(f.store.ListJobs()) != jobsBefore {
		t.Fatal("refused replacement left durable state")
	}

	// Gate open: replacement accepted and a Retire with a new JobID is queued.
	f.pullCaps("ir-1", testIngressCapability)
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, req, http.StatusAccepted, nil)
	op, _ = f.store.GetPublicIngressOperation(old.ID)
	if op.RetireCommand == nil || op.RetireCommand.JobID == op.Command.JobID || op.RetireCommand.Action != agentjob.ActionPublicIngressRetire {
		t.Fatalf("retire not queued with a new JobID: %+v", op.RetireCommand)
	}
	if j, ok := getJob(f.store, op.RetireCommand.JobID); !ok || j.Type != agentjob.ActionPublicIngressRetire || j.Status != "queued" {
		t.Fatalf("retire job: %+v", j)
	}
	if st := f.view(old.ID).Ingress; st.State != "retiring" {
		t.Fatalf("state: %+v", st)
	}
}

// Decommission of an applied ingress queues a Retire first; a closed gate
// refuses the decommission; a failed decommission start abandons the Retire.
func TestGlueDecommissionRetiresAppliedIngress(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619)
	f.pullCaps("ir-1", testIngressCapability)
	op, _ := f.store.GetPublicIngressOperation(tn.ID)
	if err := f.store.RecordIngressAgentReceipt(appliedReceipt(*op.Command), "tok-ir-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Stale plan hash: start fails, the queued retire is abandoned (not runnable).
	f.call(http.MethodPost, "/api/tunnels/decommission", tunnelHTTPAdmin, map[string]string{"id": tn.ID, "plan_hash": "0000"}, http.StatusConflict, nil)
	op, _ = f.store.GetPublicIngressOperation(tn.ID)
	if op.RetireCommand == nil {
		t.Fatal("retire was never queued for the applied ingress")
	}
	if j, _ := getJob(f.store, op.RetireCommand.JobID); j.Status != "failed" {
		t.Fatalf("failed decommission left a runnable retire: %+v", j)
	}
	// Closed gate: refused, no new retire job.
	f.store.mu.Lock()
	n := f.store.st.Nodes["ir-1"]
	n.AgentSeen = time.Now().UTC().Add(-time.Hour)
	f.store.st.Nodes["ir-1"] = n
	f.store.mu.Unlock()
	jobs := len(f.store.ListJobs())
	f.call(http.MethodPost, "/api/tunnels/decommission", tunnelHTTPAdmin, map[string]string{"id": tn.ID, "plan_hash": "0000"}, http.StatusConflict, nil)
	if len(f.store.ListJobs()) != jobs {
		t.Fatal("closed gate still created jobs")
	}
	// Open gate + reviewed decommission plan: retire queued with a fresh JobID.
	f.pullCaps("ir-1", testIngressCapability)
	var dp DecommissionPlan
	f.call(http.MethodPost, "/api/tunnels/decommission/plan", tunnelHTTPAdmin, map[string]string{"id": tn.ID}, http.StatusOK, &dp)
	f.call(http.MethodPost, "/api/tunnels/decommission", tunnelHTTPAdmin, map[string]string{"id": tn.ID, "plan_hash": dp.Hash}, http.StatusAccepted, nil)
	prev := op.RetireCommand.JobID
	op, _ = f.store.GetPublicIngressOperation(tn.ID)
	if j, _ := getJob(f.store, op.RetireCommand.JobID); op.RetireCommand.JobID == prev || j.Status != "queued" {
		t.Fatalf("expected a fresh queued retire: %+v %+v", op.RetireCommand, j)
	}
}

func getJob(s *Store, id string) (Job, bool) {
	for _, j := range s.ListJobs() {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}
