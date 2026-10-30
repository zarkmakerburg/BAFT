package config

import "testing"

func validSharedEX() Config {
	c := validIR()
	c.Node = Node{ID: "ex-01", Role: "listener"}
	c.Peer = nil
	c.Server = &Server{Listen: "0.0.0.0:8443", ServerName: "ex.example", AllowedPeerIdentities: []string{"urn:baft:node:ir-a", "urn:baft:node:ir-b"}}
	c.Routes = []Route{
		{ID: "port-443", Direction: "inbound", Target: "127.0.0.1:9001", AllowedPeers: []string{"urn:baft:node:ir-a"}},
		{ID: "port-443", Direction: "inbound", Target: "127.0.0.1:9002", AllowedPeers: []string{"urn:baft:node:ir-b"}},
	}
	return c
}

func TestInboundRouteIDMayRepeatForDisjointTenants(t *testing.T) {
	if err := Validate(validSharedEX()); err != nil {
		t.Fatal(err)
	}
}

func TestInboundRouteIDOverlappingTenantsRejected(t *testing.T) {
	c := validSharedEX()
	c.Routes[1].AllowedPeers = []string{"urn:baft:node:ir-a", "urn:baft:node:ir-b"}
	if err := Validate(c); err == nil {
		t.Fatal("route id repeated for an overlapping tenant must be rejected")
	}
}

func TestOutboundDuplicateRouteStillRejected(t *testing.T) {
	c := validIR()
	c.Routes = append(c.Routes, Route{ID: "service-main", Listen: "127.0.0.1:1444", RemoteRoute: "service-main", Direction: "outbound"})
	if err := Validate(c); err == nil {
		t.Fatal("duplicate outbound route id must still be rejected")
	}
}

func TestMaxFlowsPerPeerBounds(t *testing.T) {
	c := validSharedEX()
	c.Limits.MaxFlowsPerPeer = 8
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	c.Limits.MaxFlowsPerPeer = c.Limits.MaxFlows + 1
	if err := Validate(c); err == nil {
		t.Fatal("per-peer cap above max_flows must be rejected")
	}
	c.Limits.MaxFlowsPerPeer = -1
	if err := Validate(c); err == nil {
		t.Fatal("negative per-peer cap must be rejected")
	}
}
