package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/protocol"
)

func noiseIR() Config {
	c := validIR()
	c.Noise = &Noise{KeyFile: "/tmp/key", PeerPublicKey: strings.Repeat("A", 43)}
	return c
}

func TestCapabilityOfferDefaultAdvertisesOnlyWhatIsEnabled(t *testing.T) {
	o := validIR().CapabilityOffer()
	if !reflect.DeepEqual(o.Advertise, []string{protocol.CapCarrierH2}) || len(o.Require) != 0 {
		t.Fatalf("%+v", o)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityOfferFromNoiseWSFallbackAndShaping(t *testing.T) {
	c := noiseIR()
	c.Recovery = Recovery{Enabled: true, RetentionSeconds: 30, Mode: "same_process"}
	c.Transport.Fallback = "ws"
	c.Noise.RecordShaping.Enabled = true
	o := c.CapabilityOffer()
	want := []string{protocol.CapCarrierH2, protocol.CapCarrierWS, protocol.CapNoiseV1, protocol.CapRecordShapeV1}
	got := o.Wire()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wire %v want %v", got, want)
	}
	if !reflect.DeepEqual(append([]string(nil), o.Expect...), []string{protocol.CapCarrierH2, protocol.CapCarrierWS, protocol.CapNoiseV1, protocol.CapRecordShapeV1}) {
		t.Fatalf("dialer expect %v", o.Expect)
	}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	// A listener expects Noise/shaping from the peer but not the dialer's carrier choice.
	c.Node.Role = "listener"
	if l := c.CapabilityOffer(); !reflect.DeepEqual(l.Expect, []string{protocol.CapNoiseV1, protocol.CapRecordShapeV1}) {
		t.Fatalf("listener expect %v", l.Expect)
	}
}

func TestWSOfferAlwaysImpliesNoise(t *testing.T) {
	c := noiseIR()
	c.Transport.Primary = "ws"
	o := c.CapabilityOffer()
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Noise = nil
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "ws requires noise") {
		t.Fatalf("ws without noise accepted: %v", err)
	}
	if err := c.CapabilityOffer().Validate(); err == nil {
		t.Fatal("capability offer allowed ws without noise")
	}
}

func TestRequireCapabilitiesValidation(t *testing.T) {
	c := noiseIR()
	c.Transport.RequireCapabilities = []string{protocol.CapNoiseV1}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	if got := c.CapabilityOffer().Require; !reflect.DeepEqual(got, []string{protocol.CapNoiseV1}) {
		t.Fatalf("%v", got)
	}
	for name, mut := range map[string]func(*Config){
		"unknown":               func(c *Config) { c.Transport.RequireCapabilities = []string{"nope"} },
		"case alias":            func(c *Config) { c.Transport.RequireCapabilities = []string{"Noise.V1"} },
		"duplicate":             func(c *Config) { c.Transport.RequireCapabilities = []string{protocol.CapNoiseV1, protocol.CapNoiseV1} },
		"not enabled locally":   func(c *Config) { c.Transport.RequireCapabilities = []string{protocol.CapRecordShapeV1} },
		"noise required no cfg": func(c *Config) { c.Noise = nil; c.Transport.RequireCapabilities = []string{protocol.CapNoiseV1} },
		"carrier not served":    func(c *Config) { c.Transport.RequireCapabilities = []string{protocol.CapCarrierWS} },
		"empty token":           func(c *Config) { c.Transport.RequireCapabilities = []string{""} },
	} {
		c := noiseIR()
		mut(&c)
		if err := Validate(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRequireCapabilitiesDecodesAndRejectsUnknown(t *testing.T) {
	good := `{"schema_version":1,"node":{"id":"ir-01","role":"dialer"},"peer":{"address":"192.0.2.20:443","server_name":"ex.example","allowed_identity":"urn:baft:node:ex-01"},"tls":{"min_version":"1.3","ca_file":"/c","cert_file":"/p","key_file":"/k"},"transport":{"primary":"h2","h3_enabled":false,"shards":1,"profile":"secure-fast","require_capabilities":["carrier.h2"]},"limits":{"max_flows":256,"data_memory_mib":256,"receive_initial_kib":64,"receive_max_mib":16,"replay_max_mib":16},"recovery":{"enabled":false,"retention_seconds":30},"routes":[{"id":"service-main","listen":"127.0.0.1:1443","remote_route":"service-main","direction":"outbound","traffic_class":"interactive"}],"management":{"unix_socket":"/run/baft/admin.sock","metrics_listen":"127.0.0.1:9191"},"logging":{"level":"info","payload":false}}`
	cfg, err := DecodeJSON(strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Transport.RequireCapabilities, []string{"carrier.h2"}) {
		t.Fatalf("%v", cfg.Transport.RequireCapabilities)
	}
	bad := strings.Replace(good, `["carrier.h2"]`, `["h2"]`, 1)
	if _, err := DecodeJSON(strings.NewReader(bad)); err == nil {
		t.Fatal("unknown capability accepted")
	}
}
