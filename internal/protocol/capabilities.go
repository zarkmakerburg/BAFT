package protocol

import (
	"fmt"
	"sort"
	"strings"
)

// Capability negotiation (capability set v1).
//
// HELLO.capabilities and HELLO_ACK.capabilities carry the transport/carrier
// features a node's configuration enables. The session negotiates the
// intersection of the two lists. Nothing is ever downgraded silently: a
// capability that local policy requires and the peer lacks is refused with a
// typed *CapabilityError.
//
// Wire compatibility with v0.3.7. A v0.3.7 node sends and accepts
// `capabilities: []`; its HELLO validation only bounds the list at 16 entries
// and ignores every token, but its HELLO_ACK decoder rejects unknown fields.
// Therefore:
//   - a dialer may advertise tokens to any peer: v0.3.7 listeners ignore them;
//   - a listener adds `capabilities` to HELLO_ACK only when the dialer
//     advertised at least one token, i.e. only to a capability-aware dialer.
//     An empty capability list on the wire is the v0.3.7 baseline and keeps the
//     bytes of both frames identical to v0.3.7.
const (
	// CapNoiseV1: the carrier is authenticated by the pinned-key Noise
	// handshake (config.noise present).
	CapNoiseV1 = "noise.v1"
	// CapCarrierH2 / CapCarrierWS: carrier kinds the node serves (listener) or
	// is configured to dial as primary or fallback (dialer).
	CapCarrierH2 = "carrier.h2"
	CapCarrierWS = "carrier.ws"
	// CapRecordShapeV1: Noise record shaping is enabled. Both peers must enable
	// shaping explicitly; there is no auto-detection.
	CapRecordShapeV1 = "shape.v1"

	// MaxCapabilities is the wire bound shared with v0.3.7 HELLO validation.
	MaxCapabilities = 16
)

// Refusal reasons. They are stable strings used for errors and metric labels.
const (
	// RefusalRequired: a capability named in the local Require set is missing.
	RefusalRequired = "required_missing"
	// RefusalExpected: the peer negotiates capabilities but lacks one that local
	// configuration implies (Noise, shaping, a configured carrier kind).
	RefusalExpected = "expected_missing"
	// RefusalWSNoise: ws would be enabled without Noise.
	RefusalWSNoise = "ws_requires_noise"
	// RefusalUnsolicited: HELLO_ACK carried capabilities nobody asked for.
	RefusalUnsolicited = "unsolicited_capabilities"
	// RefusalInvalid: local offer or peer list is malformed.
	RefusalInvalid = "invalid"
)

// RefusalReasons lists every reason, for bounded metric label sets.
func RefusalReasons() []string {
	return []string{RefusalRequired, RefusalExpected, RefusalWSNoise, RefusalUnsolicited, RefusalInvalid}
}

var knownCapabilities = []string{CapCarrierH2, CapCarrierWS, CapNoiseV1, CapRecordShapeV1} // sorted

// KnownCapabilities returns the sorted capability set this build implements.
func KnownCapabilities() []string { return append([]string(nil), knownCapabilities...) }

// IsKnownCapability reports whether s is a token of this build's set. Only
// exact, canonical spellings match, so case aliases are never known.
func IsKnownCapability(s string) bool {
	i := sort.SearchStrings(knownCapabilities, s)
	return i < len(knownCapabilities) && knownCapabilities[i] == s
}

// CapabilityError is the typed refusal of a negotiation.
type CapabilityError struct {
	Reason  string
	Missing []string
}

func (e *CapabilityError) Error() string {
	if len(e.Missing) == 0 {
		return "capability negotiation refused: " + e.Reason
	}
	return "capability negotiation refused: " + e.Reason + ": " + strings.Join(e.Missing, ",")
}

// CapabilityOffer is one side's negotiation policy.
type CapabilityOffer struct {
	// Advertise is what this node's configuration enables.
	Advertise []string
	// Require must be enabled by the peer, even if the peer sent nothing
	// (operator opt-in: refuses v0.3.7 and other baseline peers).
	Require []string
	// Expect must be enabled by the peer whenever the peer negotiates
	// capabilities at all. A baseline peer is waived: the carrier layer itself
	// enforces Noise, shaping and carrier kind, and refusing every v0.3.7 peer
	// would break rolling upgrades.
	Expect []string
}

func (o CapabilityOffer) validate() error {
	check := func(name string, set []string) (map[string]struct{}, error) {
		m := make(map[string]struct{}, len(set))
		for _, c := range set {
			if !validCapabilityToken(c) || !IsKnownCapability(c) {
				return nil, fmt.Errorf("%s: unknown capability %q", name, c)
			}
			if _, dup := m[c]; dup {
				return nil, fmt.Errorf("%s: duplicate capability %q", name, c)
			}
			m[c] = struct{}{}
		}
		return m, nil
	}
	adv, err := check("advertise", o.Advertise)
	if err != nil {
		return err
	}
	if len(o.Advertise) > MaxCapabilities {
		return fmt.Errorf("advertise: more than %d capabilities", MaxCapabilities)
	}
	for _, s := range []struct {
		name string
		set  []string
	}{{"require", o.Require}, {"expect", o.Expect}} {
		if _, err := check(s.name, s.set); err != nil {
			return err
		}
		for _, c := range s.set {
			if _, ok := adv[c]; !ok {
				return fmt.Errorf("%s: %q is not advertised locally", s.name, c)
			}
		}
	}
	if _, ws := adv[CapCarrierWS]; ws {
		if _, n := adv[CapNoiseV1]; !n {
			return fmt.Errorf("advertise: %s requires %s", CapCarrierWS, CapNoiseV1)
		}
	}
	return nil
}

// Validate reports whether the offer is well formed: known canonical tokens,
// no duplicates, Require/Expect advertised locally, ws only with Noise.
func (o CapabilityOffer) Validate() error { return o.validate() }

// Wire returns the canonical (strictly ascending) list for HELLO. It is never
// nil, so an empty offer encodes as the v0.3.7 `[]`.
func (o CapabilityOffer) Wire() []string {
	out := append([]string{}, o.Advertise...)
	sort.Strings(out)
	return out
}

// NegotiatedCapabilities is the outcome of a successful negotiation.
type NegotiatedCapabilities struct {
	// Enabled is the sorted intersection of both sides' known capabilities.
	Enabled []string
	// Ignored lists well-formed peer tokens this build does not know.
	Ignored []string
	// Baseline is true when the peer advertised nothing: it is a v0.3.7 node
	// (or a node with nothing enabled) and the wire is the v0.3.7 baseline.
	Baseline bool
}

// Has reports whether c was negotiated.
func (n NegotiatedCapabilities) Has(c string) bool {
	i := sort.SearchStrings(n.Enabled, c)
	return i < len(n.Enabled) && n.Enabled[i] == c
}

// NegotiateCapabilities computes the intersection of local.Advertise and the
// peer list and enforces Require/Expect. peer must already be canonical.
func NegotiateCapabilities(local CapabilityOffer, peer []string) (NegotiatedCapabilities, error) {
	if err := local.validate(); err != nil {
		return NegotiatedCapabilities{}, &CapabilityError{Reason: RefusalInvalid, Missing: nil}
	}
	if err := validateCapabilityList(peer); err != nil {
		return NegotiatedCapabilities{}, &CapabilityError{Reason: RefusalInvalid}
	}
	peerSet := make(map[string]struct{}, len(peer))
	for _, c := range peer {
		peerSet[c] = struct{}{}
	}
	out := NegotiatedCapabilities{Enabled: []string{}, Baseline: len(peer) == 0}
	for _, c := range local.Advertise {
		if _, ok := peerSet[c]; ok {
			out.Enabled = append(out.Enabled, c)
		}
	}
	sort.Strings(out.Enabled)
	for _, c := range peer {
		if !IsKnownCapability(c) {
			out.Ignored = append(out.Ignored, c)
		}
	}
	missing := func(want []string) []string {
		var m []string
		for _, c := range want {
			if !out.Has(c) {
				m = append(m, c)
			}
		}
		sort.Strings(m)
		return m
	}
	if m := missing(local.Require); len(m) > 0 {
		return NegotiatedCapabilities{}, &CapabilityError{Reason: RefusalRequired, Missing: m}
	}
	if !out.Baseline {
		if m := missing(local.Expect); len(m) > 0 {
			return NegotiatedCapabilities{}, &CapabilityError{Reason: RefusalExpected, Missing: m}
		}
	}
	if out.Has(CapCarrierWS) && !out.Has(CapNoiseV1) {
		return NegotiatedCapabilities{}, &CapabilityError{Reason: RefusalWSNoise, Missing: []string{CapNoiseV1}}
	}
	return out, nil
}
