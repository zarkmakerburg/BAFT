package config

import (
	"strings"
	"testing"
)

// SSH management (port 22) must never become a BAFT transport, route or
// metrics endpoint: a mistaken or hostile config must be refused before any
// socket is opened, not rely on the OS refusing a bind.
func TestValidateRefusesSSHManagementPort(t *testing.T) {
	ir := validIR()
	ir.Routes[0].Listen = "0.0.0.0:22"
	ir.Routes[0].Public = true
	if err := Validate(ir); err == nil || !strings.Contains(err.Error(), "22") {
		t.Fatalf("public outbound route on port 22 accepted: %v", err)
	}
	ir = validIR()
	ir.Routes[0].Listen = "127.0.0.1:22"
	if err := Validate(ir); err == nil {
		t.Fatal("loopback outbound route on port 22 accepted")
	}
	ir = validIR()
	ir.Management.MetricsListen = "127.0.0.1:22"
	if err := Validate(ir); err == nil {
		t.Fatal("metrics listener on port 22 accepted")
	}

	ex := validIR()
	ex.Node.Role = "listener"
	ex.Peer = nil
	ex.Server = &Server{Listen: "0.0.0.0:22", ServerName: "ex.example", AllowedPeerIdentities: []string{"urn:baft:node:ir-01"}}
	ex.Routes = []Route{{ID: "service-main", Direction: "inbound", Target: "127.0.0.1:2443", AllowedPeers: []string{"urn:baft:node:ir-01"}}}
	if err := Validate(ex); err == nil || !strings.Contains(err.Error(), "22") {
		t.Fatalf("EX listener on port 22 accepted: %v", err)
	}
	ex.Server.Listen = "0.0.0.0:8443"
	if err := Validate(ex); err != nil {
		t.Fatalf("a normal EX listener must stay valid: %v", err)
	}
}

// Every resource bound the transport relies on is enforced at config load;
// an out-of-range value is an explicit refusal, never a silent clamp.
func TestValidateResourceLimitBounds(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"max_flows zero", func(c *Config) { c.Limits.MaxFlows = 0 }},
		{"max_flows over 65535", func(c *Config) { c.Limits.MaxFlows = 65536 }},
		{"data memory under 16 MiB", func(c *Config) { c.Limits.DataMemoryMiB = 15 }},
		{"data memory over 4 GiB", func(c *Config) { c.Limits.DataMemoryMiB = 4097 }},
		{"receive initial zero", func(c *Config) { c.Limits.ReceiveInitialKiB = 0 }},
		{"receive max over its pool", func(c *Config) { c.Limits.ReceiveMaxMiB = 129 }},
		{"replay max over its pool", func(c *Config) { c.Limits.ReplayMaxMiB = 129 }},
		{"shards zero", func(c *Config) { c.Transport.Shards = 0 }},
		{"shards over 8", func(c *Config) { c.Transport.Shards = 9 }},
		{"retention over 300s", func(c *Config) { c.Recovery.RetentionSeconds = 301 }},
		{"payload logging", func(c *Config) { c.Logging.Payload = true }},
	}
	for _, tc := range cases {
		c := validIR()
		tc.mutate(&c)
		if err := Validate(c); err == nil {
			t.Errorf("%s: accepted", tc.name)
		}
	}
	if err := Validate(validIR()); err != nil {
		t.Fatalf("baseline must stay valid: %v", err)
	}
}

// A WebSocket carrier is only authenticated by the inner Noise layer, so it
// must be refused when it is only the fallback too, and a fallback can never
// be enabled in a way that skips the recovery authority.
func TestValidateNoInsecureFallbackConfig(t *testing.T) {
	c := validIR()
	c.Recovery.Enabled = true
	c.Recovery.Mode = "same_process"
	c.Transport.Fallback = "ws"
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "ws requires noise") {
		t.Fatalf("ws fallback without noise accepted: %v", err)
	}
	c = validIR()
	c.Transport.Fallback = "h2x"
	if err := Validate(c); err == nil {
		t.Fatal("unknown fallback accepted")
	}
	c = validIR()
	c.Peer.FallbackAddress = "192.0.2.30:443"
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "requires transport.fallback") {
		t.Fatalf("fallback peer path without fallback transport accepted: %v", err)
	}
}
