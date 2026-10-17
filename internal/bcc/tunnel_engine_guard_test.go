package bcc

import (
	"strings"
	"testing"
	"time"
)

// Regression: the API must not silently treat a future/unknown engine as BAFT.
func TestTunnelEngineRequestFailsClosedOnPlanAndDeploy(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	for _, engine := range []string{"frp", "rathole", "wss", "quic", "unsupported", "BAFT"} {
		req := TunnelRequest{Engine: engine, EXNode: "ex-1", IRNode: "ir-1"}
		if _, err := s.BuildPlan(req, now); err == nil || !strings.Contains(err.Error(), "engine") {
			t.Fatalf("%q plan did not reject unavailable engine: %v", engine, err)
		}
		if _, err := s.CreateTunnel(req, now); err == nil || !strings.Contains(err.Error(), "engine") {
			t.Fatalf("%q create did not reject unavailable engine: %v", engine, err)
		}
		if got := len(s.ListTunnels()); got != 0 {
			t.Fatalf("%q left %d tunnels after rejection", engine, got)
		}
	}
	req := TunnelRequest{Engine: "baft", EXNode: "ex-1", IRNode: "ir-1"}
	plan, err := s.BuildPlan(req, now)
	if err != nil || plan.Request.Engine != "baft" {
		t.Fatalf("native BAFT explicit engine not preserved in plan: %+v %v", plan.Request, err)
	}
	if _, err := s.CreateTunnel(req, now); err != nil {
		t.Fatalf("native BAFT explicit engine rejected: %v", err)
	}
}
