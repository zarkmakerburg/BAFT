package config

import (
	"fmt"

	"github.com/zarkmakerburg/baft/internal/protocol"
)

// CapabilityOffer derives the HELLO capability policy from the configuration.
//
//   - Advertise: the carrier kinds this node serves (listener) or may dial
//     (dialer: primary and fallback), noise.v1 when Noise is configured, and
//     shape.v1 when Noise record shaping is enabled. Nothing the configuration
//     does not enable is advertised.
//   - Expect: noise.v1 and shape.v1 when enabled locally (both peers must
//     enable them), and, for a dialer, every carrier kind it may dial. Expect
//     binds against capability-aware peers only, so a v0.3.7 peer still connects.
//   - Require: transport.require_capabilities, the operator opt-in that also
//     refuses v0.3.7 / baseline peers.
//
// The result is meaningful only for a config that passed Validate.
func (c Config) CapabilityOffer() protocol.CapabilityOffer {
	var o protocol.CapabilityOffer
	add := func(set *[]string, v string) {
		for _, e := range *set {
			if e == v {
				return
			}
		}
		*set = append(*set, v)
	}
	for _, k := range []string{c.Transport.Primary, c.Transport.Fallback} {
		var tok string
		switch k {
		case "h2":
			tok = protocol.CapCarrierH2
		case "ws":
			tok = protocol.CapCarrierWS
		default:
			continue
		}
		add(&o.Advertise, tok)
		if c.Node.Role == "dialer" {
			add(&o.Expect, tok)
		}
	}
	if c.Noise != nil {
		add(&o.Advertise, protocol.CapNoiseV1)
		add(&o.Expect, protocol.CapNoiseV1)
		if c.Noise.RecordShaping.Enabled {
			add(&o.Advertise, protocol.CapRecordShapeV1)
			add(&o.Expect, protocol.CapRecordShapeV1)
		}
	}
	o.Require = append(o.Require, c.Transport.RequireCapabilities...)
	return o
}

func validateCapabilities(c Config) error {
	for _, tok := range c.Transport.RequireCapabilities {
		if !protocol.IsKnownCapability(tok) {
			return fmt.Errorf("transport.require_capabilities: unknown capability %q", tok)
		}
	}
	if err := c.CapabilityOffer().Validate(); err != nil {
		return fmt.Errorf("transport.require_capabilities: %w", err)
	}
	return nil
}
