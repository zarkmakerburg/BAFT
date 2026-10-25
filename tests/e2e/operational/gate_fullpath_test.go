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
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// site is one reviewed tunnel (one public ingress) on the IR host.
type site struct {
	f      *fullPath
	tn     bcc.Tunnel
	plan   bcc.Plan
	port   int
	route  string
	job    string
	echo   *echo
	cmd    ingressauthority.CommandV1
	window time.Duration
}

// fullPath is the shared positive scenario: a private-netns IR host, a real
// BCC, one real Agent process and one or more sites.
type fullPath struct {
	*site
	h   *host
	b   *bccRig
	a   *agentRig
	tap *tap
}

func setupFullPath(t *testing.T, clientPort int, route string) *fullPath {
	t.Helper()
	return setupFullPathWindow(t, clientPort, route, 0)
}

// bccHeldCommandWindow mirrors the production held-command window BCC signs at
// activation (internal/bcc ingress_glue.go). Only the expiry gate shortens it,
// by activating the tunnel on a clock moved back by (bccHeldCommandWindow -
// window); stageFor verifies the resulting window instead of trusting it.
const bccHeldCommandWindow = 24 * time.Hour

func setupFullPathWindow(t *testing.T, clientPort int, route string, window time.Duration) *fullPath {
	t.Helper()
	h := newHost(t)
	f := &fullPath{h: h}
	f.b = newBCC(t, map[string]string{"ex-opv1": "foreign", "ex-opv2": "foreign", "ex-opv3": "foreign", irNode: "worker"})
	if window > 0 {
		f.b.warp = bccHeldCommandWindow - window
	}
	f.tap = newTap(t, f.b)
	f.a = startAgent(t, f.b, h, t.TempDir(), &agentOpt{url: f.tap.srv.URL, client: f.b.cl})
	f.site = f.addSite(t, "ex-opv1", "main-de", 8443, clientPort, route)
	return f
}

// addSite creates a tunnel through the reviewed Wizard API plus its reviewed
// desired-state reservation, with a loopback echo as the IR tunnel entry.
func (f *fullPath) addSite(t *testing.T, ex, instance string, exPort, clientPort int, route string) *site {
	t.Helper()
	_, p, err := net.SplitHostPort(route)
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := strconv.Atoi(p)
	s := &site{f: f, port: clientPort, route: route, job: "apply-" + strconv.Itoa(clientPort), window: 2 * time.Minute}
	s.echo = startEcho(t, route)
	s.tn, s.plan = f.b.wizard(scoped(instance, ex, irNode, exPort, clientPort, route,
		"127.0.0.1:"+strconv.Itoa(rp+8000), "127.0.0.1:"+strconv.Itoa(rp+8100)))
	f.b.reserve(s.tn, s.plan.Hash, clientPort, 200)
	return s
}

func (s *site) addr() string { return "127.0.0.1:" + strconv.Itoa(s.port) }

// stage stages the held Apply command for this site.
func (s *site) stage(t *testing.T) {
	t.Helper()
	s.cmd = s.f.b.stageFor(s.tn, s.job, s.window)
}

// apply drives staging, the capability-opened gate, signed enqueue and the
// normal Agent poll; it requires success.
func (s *site) apply(t *testing.T) {
	t.Helper()
	s.stage(t)
	f := s.f
	if !f.a.gate {
		if err := f.b.store.EnqueueOperationalIngress(s.tn.ID); err == nil {
			t.Fatal("enqueue succeeded before the Agent declared operational-ingress-v1 (gate must be closed)")
		}
	}
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngress(s.tn.ID); err != nil {
		t.Fatalf("enqueue with an enrolled, capable, fresh Agent: %v", err)
	}
	res := f.a.runOnce()
	if len(res) != 1 || res[0].Status != "succeeded" || res[0].Action != agentjob.ActionPublicIngressApply {
		t.Fatalf("Agent apply result: %+v", res)
	}
}

// waitReady polls BCC's readiness API (its own probe is cached for 10s).
func (s *site) waitReady(t *testing.T) bcc.IngressReadinessView {
	t.Helper()
	var v bcc.IngressReadinessView
	deadline := time.Now().Add(30 * time.Second)
	for {
		v = s.f.b.readiness(s.tn.ID)
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
	if err := roundTrip(t, f.addr(), payload); err != nil {
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
