//go:build linux

package operational

import (
	"bytes"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
)

// fullPath is the shared positive scenario; later tests build on it.
type fullPath struct {
	h    *host
	b    *bccRig
	a    *agentRig
	tn   bcc.Tunnel
	plan bcc.Plan
	port int
	job  string
	echo *echo
}

func setupFullPath(t *testing.T, clientPort int, route string) *fullPath {
	t.Helper()
	h := newHost(t)
	_, routePort, _ := net.SplitHostPort(route)
	rp, _ := strconv.Atoi(routePort)
	f := &fullPath{h: h, port: clientPort}
	f.echo = startEcho(t, route)
	f.b = newBCC(t, map[string]string{"ex-opv1": "foreign", irNode: "worker"})
	f.tn, f.plan = f.b.wizard(scoped("main-de", "ex-opv1", irNode, 8443, clientPort, route, "127.0.0.1:"+strconv.Itoa(rp+8000), "127.0.0.1:"+strconv.Itoa(rp+8100)))
	f.b.reserve(f.tn, f.plan.Hash, clientPort, 200)
	f.a = startAgent(t, f.b, h, t.TempDir(), nil)
	f.job = "apply-" + strconv.Itoa(clientPort)
	return f
}

// applyAndVerify drives staging, the capability-opened gate, signed enqueue
// and the normal Agent poll.
func (f *fullPath) apply(t *testing.T) {
	t.Helper()
	f.b.stage(f.tn, f.job)
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err == nil {
		t.Fatal("enqueue succeeded before the Agent declared operational-ingress-v1 (gate must be closed)")
	}
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatalf("enqueue with an enrolled, capable, fresh Agent: %v", err)
	}
	res := f.a.runOnce()
	if len(res) != 1 || res[0].Status != "succeeded" || res[0].Action != agentjob.ActionPublicIngressApply {
		t.Fatalf("Agent apply result: %+v", res)
	}
}

// waitReady polls BCC's readiness API (its own probe is cached for 10s).
func (f *fullPath) waitReady(t *testing.T) bcc.IngressReadinessView {
	t.Helper()
	var v bcc.IngressReadinessView
	deadline := time.Now().Add(30 * time.Second)
	for {
		v = f.b.readiness(f.tn.ID)
		if v.Ready || time.Now().After(deadline) {
			return v
		}
		time.Sleep(2 * time.Second)
	}
}

func TestOperationalIngressV1Gate_FullPathWizardToVerifiedCard(t *testing.T) {
	f := setupFullPath(t, 54701, "127.0.0.1:1443")
	if pre := f.b.readiness(f.tn.ID); pre.Ready || pre.Card != nil {
		t.Fatalf("card before any apply: %+v", pre)
	}
	f.apply(t)

	// client TCP bytes through the real listener + nft rule to the tunnel target
	payload := bytes.Repeat([]byte("full-path-"), 512)
	if err := roundTrip(t, "127.0.0.1:"+strconv.Itoa(f.port), payload); err != nil {
		t.Fatalf("client bytes through ingress: %v", err)
	}
	if f.echo.seen() < len(payload) {
		t.Fatalf("tunnel target saw %d bytes, want %d", f.echo.seen(), len(payload))
	}
	if f.h.baftRules() != 1 {
		t.Fatalf("expected exactly one owned nft rule: %s", f.h.chainJSON())
	}
	op := f.b.op(f.tn.ID)
	if op.Receipt == nil || op.Receipt.Phase != "applied" || !op.Receipt.OwnershipVerified || op.Dispatch.Phase != "queued" {
		t.Fatalf("receipt/dispatch: %+v", op)
	}
	v := f.waitReady(t)
	if !v.Ready || !v.Verified || v.Card == nil {
		t.Fatalf("not ready after physical proof: %+v probe=%+v", v, v.Probe)
	}
	c := v.Card
	if c.TunnelID != f.tn.ID || c.IRNode != irNode || int(c.ConfigPort) != f.port || int(c.PublicPort) != f.port || c.PlanSHA256 != f.plan.Hash {
		t.Fatalf("card does not carry the reviewed plan/port: %+v", c)
	}
	if v.Probe == nil || !v.Probe.Connected || !v.Probe.PayloadVerified {
		t.Fatalf("BCC probe: %+v", v.Probe)
	}
}
