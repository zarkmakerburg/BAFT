package bcc

import (
	"net/http"
	"strings"
	"testing"
)

const testHostIngressCapability = IngressCapabilityOperationalHostV1

func planGate(p Plan, name string) (PlanGate, bool) {
	for _, g := range p.Gates {
		if g.Name == name {
			return g, true
		}
	}
	return PlanGate{}, false
}

// ADR-003 §4.2(3): on an IR in safe host mode the reviewed plan must carry
// the operator's per-port acknowledgement. Without it the plan FAILs (nothing
// can be created); with it the hash differs and the plan passes, and after
// activation the Apply is queued.
func TestHostModePlanRequiresClientPortAcknowledgement(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	f.pullCaps("ir-1", testHostIngressCapability)
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	req.ClientPort = 54619

	var unacked Plan
	f.call(http.MethodPost, "/api/tunnels/plan", tunnelHTTPAdmin, req, http.StatusOK, &unacked)
	g, ok := planGate(unacked, "public port acknowledgement")
	if unacked.OK || !ok || g.Status != "FAIL" || !strings.Contains(g.Detail, "nginx") || !strings.Contains(g.Detail, "Xray/3X-UI") {
		t.Fatalf("host-mode plan without acknowledgement must FAIL and name checked/unchecked services: ok=%v gate=%+v", unacked.OK, g)
	}
	// The failed plan cannot be deployed.
	bad := unacked.Request
	bad.PlanHash = unacked.Hash
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, bad, http.StatusConflict, nil)

	req.ClientPortAck = true
	acked := f.plan(req)
	if g, ok := planGate(acked, "public port acknowledgement"); !ok || g.Status != "PASS" || !acked.Request.ClientPortAck || acked.Hash == unacked.Hash {
		t.Fatalf("acknowledged plan: gate=%+v request=%+v", g, acked.Request)
	}
	// The acknowledgement is bound to the hash: deploying the acknowledged
	// hash without the acknowledgement is stale.
	strip := acked.Request
	strip.ClientPortAck = false
	strip.PlanHash = acked.Hash
	f.call(http.MethodPost, "/api/tunnels", tunnelHTTPAdmin, strip, http.StatusConflict, nil)

	tn := f.activate(f.create(acked))
	if !tn.ClientPortAck {
		t.Fatalf("tunnel lost the reviewed acknowledgement: %+v", tn)
	}
	f.pullCaps("ir-1", testHostIngressCapability)
	if applyJobs(f.store) != 1 || f.view(tn.ID).Ingress.State != "queued" {
		t.Fatalf("acknowledged host-mode ingress not queued: jobs=%d view=%+v", applyJobs(f.store), f.view(tn.ID).Ingress)
	}
}

// A tunnel reviewed while the IR had no automatic ingress carries no
// acknowledgement. If the IR later switches to safe host mode, BCC must not
// queue that public port: it stays held with a reason that says what to do.
func TestHostModeNeverQueuesUnacknowledgedTunnel(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619)
	if tn.ClientPortAck {
		t.Fatal("acknowledgement invented")
	}
	f.pullCaps("ir-1", testHostIngressCapability)
	f.pullCaps("ir-1", testHostIngressCapability)
	st := f.view(tn.ID).Ingress
	if applyJobs(f.store) != 0 || st == nil || st.State != "held" || !strings.Contains(st.Reason, "not acknowledged") {
		t.Fatalf("unacknowledged host-mode ingress must stay held: jobs=%d state=%+v", applyJobs(f.store), st)
	}
	// The LAB contract is unchanged: same tunnel, LAB capability, queued.
	f.pullCaps("ir-1", testIngressCapability)
	if applyJobs(f.store) != 1 {
		t.Fatalf("LAB capability must still queue: jobs=%d", applyJobs(f.store))
	}
}

// Without automatic ingress the held reason points at both ways forward.
func TestHeldReasonWithoutCapabilityNamesHostModeAndManualPath(t *testing.T) {
	enableIngressGlue(t)
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619)
	f.pullCaps("ir-1", "")
	st := f.view(tn.ID).Ingress
	if st == nil || st.State != "held" || !strings.Contains(st.Reason, "--enable-host-ingress") || !strings.Contains(st.Reason, "route listener") {
		t.Fatalf("held reason: %+v", st)
	}
}

// An acknowledgement without a client port means nothing and must not
// change the reviewed plan.
func TestClientPortAckWithoutClientPortIsIgnored(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	f.pullCaps("ir-1", testHostIngressCapability)
	req := scopedReq("main-de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	plain := f.plan(req)
	req.ClientPortAck = true
	acked := f.plan(req)
	if acked.Hash != plain.Hash || acked.Request.ClientPortAck {
		t.Fatalf("acknowledgement without client port changed the plan: %+v", acked.Request)
	}
	if _, ok := planGate(acked, "public port acknowledgement"); ok {
		t.Fatal("acknowledgement gate without a client port")
	}
}
