package protocol

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/protocol/legacyv037"
)

func TestKnownCapabilitySetIsSortedCanonicalAndCaseExact(t *testing.T) {
	k := KnownCapabilities()
	if len(k) == 0 || len(k) > MaxCapabilities {
		t.Fatalf("bad set size %d", len(k))
	}
	for i, c := range k {
		if !validCapabilityToken(c) || !IsKnownCapability(c) {
			t.Errorf("%q not canonical/known", c)
		}
		if i > 0 && k[i-1] >= c {
			t.Errorf("set not strictly sorted at %q", c)
		}
		if IsKnownCapability(strings.ToUpper(c)) {
			t.Errorf("case alias of %q is known", c)
		}
	}
	// mutating the returned copy must not change the set
	k[0] = "zzz"
	if IsKnownCapability("zzz") {
		t.Fatal("KnownCapabilities leaked internal slice")
	}
	if IsKnownCapability("") || IsKnownCapability("noise") {
		t.Fatal("unknown token reported known")
	}
}

func capErr(t *testing.T, err error) *CapabilityError {
	t.Helper()
	var ce *CapabilityError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CapabilityError, got %v", err)
	}
	return ce
}

func TestNegotiateIntersectionOnlyWhenBothAdvertise(t *testing.T) {
	local := CapabilityOffer{Advertise: []string{CapCarrierH2, CapNoiseV1, CapRecordShapeV1}}
	n, err := NegotiateCapabilities(local, []string{CapCarrierH2, CapNoiseV1})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{CapCarrierH2, CapNoiseV1}; !reflect.DeepEqual(n.Enabled, want) {
		t.Fatalf("enabled %v want %v", n.Enabled, want)
	}
	if n.Has(CapRecordShapeV1) {
		t.Fatal("optional capability enabled although peer did not advertise it")
	}
	if n.Baseline {
		t.Fatal("peer with caps reported baseline")
	}
}

func TestNegotiateBaselinePeerEnablesNothingAndIsWaivedFromExpect(t *testing.T) {
	local := CapabilityOffer{
		Advertise: []string{CapCarrierH2, CapNoiseV1},
		Expect:    []string{CapNoiseV1},
	}
	n, err := NegotiateCapabilities(local, []string{})
	if err != nil {
		t.Fatalf("v0.3.7 baseline peer must be accepted: %v", err)
	}
	if !n.Baseline || len(n.Enabled) != 0 {
		t.Fatalf("baseline outcome wrong: %+v", n)
	}
	// nil (absent HELLO_ACK field) is the same baseline.
	if n, err = NegotiateCapabilities(local, nil); err != nil || !n.Baseline {
		t.Fatalf("nil peer list: %+v %v", n, err)
	}
}

func TestNegotiateRequiredMissingIsExplicitTypedRefusal(t *testing.T) {
	local := CapabilityOffer{
		Advertise: []string{CapCarrierH2, CapNoiseV1},
		Require:   []string{CapNoiseV1},
	}
	for name, peer := range map[string][]string{
		"baseline v0.3.7": {},
		"aware lacking":   {CapCarrierH2},
	} {
		_, err := NegotiateCapabilities(local, peer)
		ce := capErr(t, err)
		if ce.Reason != RefusalRequired || !reflect.DeepEqual(ce.Missing, []string{CapNoiseV1}) {
			t.Errorf("%s: %+v", name, ce)
		}
		if !strings.Contains(err.Error(), CapNoiseV1) {
			t.Errorf("%s: error does not name the capability: %v", name, err)
		}
	}
}

func TestNegotiateExpectedMissingRefusedForAwarePeer(t *testing.T) {
	local := CapabilityOffer{
		Advertise: []string{CapNoiseV1, CapRecordShapeV1},
		Expect:    []string{CapNoiseV1, CapRecordShapeV1},
	}
	_, err := NegotiateCapabilities(local, []string{CapNoiseV1})
	ce := capErr(t, err)
	if ce.Reason != RefusalExpected || !reflect.DeepEqual(ce.Missing, []string{CapRecordShapeV1}) {
		t.Fatalf("%+v", ce)
	}
}

func TestNegotiateWSAlwaysRequiresNoise(t *testing.T) {
	// A local offer that advertises ws without Noise is invalid.
	_, err := NegotiateCapabilities(CapabilityOffer{Advertise: []string{CapCarrierWS}}, []string{CapCarrierWS})
	if capErr(t, err).Reason != RefusalInvalid {
		t.Fatal("ws without noise accepted locally")
	}
	// A peer that offers ws but not Noise is refused, not downgraded.
	local := CapabilityOffer{Advertise: []string{CapCarrierWS, CapNoiseV1}}
	_, err = NegotiateCapabilities(local, []string{CapCarrierWS})
	if ce := capErr(t, err); ce.Reason != RefusalWSNoise {
		t.Fatalf("%+v", ce)
	}
	n, err := NegotiateCapabilities(local, []string{CapCarrierWS, CapNoiseV1})
	if err != nil || !n.Has(CapCarrierWS) || !n.Has(CapNoiseV1) {
		t.Fatalf("%+v %v", n, err)
	}
}

func TestNegotiateIgnoresUnknownWellFormedPeerTokens(t *testing.T) {
	local := CapabilityOffer{Advertise: []string{CapNoiseV1}}
	n, err := NegotiateCapabilities(local, []string{CapNoiseV1, "x-future.v9"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n.Enabled, []string{CapNoiseV1}) || !reflect.DeepEqual(n.Ignored, []string{"x-future.v9"}) {
		t.Fatalf("%+v", n)
	}
}

func TestNegotiateRefusesMalformedPeerListAndInvalidOffers(t *testing.T) {
	local := CapabilityOffer{Advertise: []string{CapNoiseV1}}
	for name, peer := range map[string][]string{
		"unsorted":  {CapNoiseV1, CapCarrierH2},
		"duplicate": {CapNoiseV1, CapNoiseV1},
		"uppercase": {"NOISE.V1"},
		"empty":     {""},
		"oversize":  strings.Split(strings.Repeat("a,", 17)+"a", ","),
	} {
		_, err := NegotiateCapabilities(local, peer)
		if capErr(t, err).Reason != RefusalInvalid {
			t.Errorf("%s accepted", name)
		}
	}
	bad := map[string]CapabilityOffer{
		"unknown advertise":      {Advertise: []string{"nope"}},
		"case alias":             {Advertise: []string{"Noise.V1"}},
		"dup advertise":          {Advertise: []string{CapNoiseV1, CapNoiseV1}},
		"require not advertised": {Advertise: []string{CapNoiseV1}, Require: []string{CapRecordShapeV1}},
		"expect not advertised":  {Advertise: []string{CapNoiseV1}, Expect: []string{CapCarrierH2}},
		"unknown require":        {Advertise: []string{CapNoiseV1}, Require: []string{"nope"}},
	}
	for name, o := range bad {
		if o.Validate() == nil {
			t.Errorf("%s: offer accepted", name)
		}
		if _, err := NegotiateCapabilities(o, nil); capErr(t, err).Reason != RefusalInvalid {
			t.Errorf("%s: negotiate did not refuse", name)
		}
	}
}

func TestOfferWireIsSortedNonNilAndMatchesV037EmptyForm(t *testing.T) {
	w := CapabilityOffer{Advertise: []string{CapRecordShapeV1, CapCarrierH2, CapNoiseV1}}.Wire()
	if !reflect.DeepEqual(w, []string{CapCarrierH2, CapNoiseV1, CapRecordShapeV1}) {
		t.Fatalf("%v", w)
	}
	if err := validateCapabilities(w); err != nil {
		t.Fatalf("own wire form refused: %v", err)
	}
	e := CapabilityOffer{}.Wire()
	if e == nil || len(e) != 0 {
		t.Fatalf("empty offer must encode as [] not null: %#v", e)
	}
}

// ---- v0.3.7 wire interoperability (frozen v0.3.7 codec in legacyv037) -------

const sess = "ffeeddccbbaa99887766554433221100"
const boot = "00112233445566778899aabbccddeeff"

func newHello(caps []string) Hello {
	return Hello{ProtocolMin: 1, ProtocolMax: 1, NodeID: "ir-01", BootID: boot, SessionID: sess, Epoch: "1", Mode: "new",
		ProfileID: "secure-fast", ProfileVersion: 1, ConfigRevision: "config-v1", Capabilities: caps}
}

func ackPayload(caps []string) HelloAck {
	return HelloAck{SelectedProtocol: 1, SessionID: sess, Epoch: "1", PeerBootID: boot,
		AcceptedProfile:  AcceptedProfile{ID: "secure-fast", Version: 1},
		NegotiatedLimits: NegotiatedLimits{MaxFramePayloadBytes: MaxPayloadSize, MaxFlowsPerShard: 64, ReceiveInitialBytes: 1, ReceiveMaxBytes: 2, RetentionMS: 3},
		Capabilities:     caps}
}

// A v0.3.7 listener must accept every HELLO a new dialer can emit.
func TestNewHelloIsAcceptedByV037Decoder(t *testing.T) {
	for _, caps := range [][]string{{}, {CapNoiseV1}, KnownCapabilities()} {
		b, err := EncodeControl(newHello(caps))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacyv037.DecodeHello(b); err != nil {
			t.Errorf("v0.3.7 refused new HELLO %v: %v", caps, err)
		}
		if _, err := DecodeHello(b); err != nil {
			t.Errorf("new refused its own HELLO %v: %v", caps, err)
		}
	}
}

// A new node must accept every HELLO a v0.3.7 dialer emits, including a nil
// slice encoded the v0.3.7 way ([]).
func TestV037HelloIsAcceptedByNewDecoder(t *testing.T) {
	b, err := legacyv037.EncodeControl(legacyv037.Hello{ProtocolMin: 1, ProtocolMax: 1, NodeID: "ir-01", BootID: boot, SessionID: sess,
		Epoch: "1", Mode: "new", ProfileID: "secure-fast", ProfileVersion: 1, ConfigRevision: "config-v1", Capabilities: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := DecodeHello(b)
	if err != nil {
		t.Fatalf("new refused v0.3.7 HELLO: %v", err)
	}
	if n, err := NegotiateCapabilities(CapabilityOffer{Advertise: []string{CapNoiseV1}}, h.Capabilities); err != nil || !n.Baseline {
		t.Fatalf("v0.3.7 HELLO did not negotiate as baseline: %+v %v", n, err)
	}
}

// HELLO_ACK without capabilities must be byte-for-byte what v0.3.7 emits and
// accepted by the v0.3.7 decoder; WITH capabilities the v0.3.7 decoder refuses
// it (unknown field), which is exactly why a listener only adds them for a
// capability-aware dialer.
func TestHelloAckBaselineBytesMatchV037AndCapsAreNeverSentUnsolicited(t *testing.T) {
	newB, err := EncodeControl(ackPayload(nil))
	if err != nil {
		t.Fatal(err)
	}
	oldB, err := legacyv037.EncodeControl(legacyv037.HelloAck{SelectedProtocol: 1, SessionID: sess, Epoch: "1", PeerBootID: boot,
		AcceptedProfile:  legacyv037.AcceptedProfile{ID: "secure-fast", Version: 1},
		NegotiatedLimits: legacyv037.NegotiatedLimits{MaxFramePayloadBytes: MaxPayloadSize, MaxFlowsPerShard: 64, ReceiveInitialBytes: 1, ReceiveMaxBytes: 2, RetentionMS: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(newB, oldB) {
		t.Fatalf("baseline HELLO_ACK differs from v0.3.7:\nnew %s\nold %s", newB, oldB)
	}
	if _, err := legacyv037.DecodeHelloAck(newB); err != nil {
		t.Fatalf("v0.3.7 refused baseline ack: %v", err)
	}
	got, err := DecodeHelloAck(oldB)
	if err != nil || got.Capabilities != nil {
		t.Fatalf("new refused v0.3.7 ack: %v %+v", err, got)
	}
	withCaps, _ := EncodeControl(ackPayload([]string{CapNoiseV1}))
	if _, err := legacyv037.DecodeHelloAck(withCaps); err == nil {
		t.Fatal("test premise broken: v0.3.7 must reject an ack carrying capabilities")
	}
	if _, err := DecodeHelloAck(withCaps); err != nil {
		t.Fatalf("new refused ack with capabilities: %v", err)
	}
}

func TestHelloAckCapabilitiesCanonicalOrRefused(t *testing.T) {
	base := string(mustEncode(t, ackPayload(nil)))
	with := func(v string) []byte { return []byte(strings.TrimSuffix(base, "}") + `,"capabilities":` + v + "}") }
	for _, ok := range []string{`[]`, `["noise.v1"]`, `["carrier.h2","noise.v1"]`, `["x-unknown.v9"]`} {
		if _, err := DecodeHelloAck(with(ok)); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for name, v := range map[string]string{
		"null":      `null`,
		"duplicate": `["noise.v1","noise.v1"]`,
		"unsorted":  `["noise.v1","carrier.h2"]`,
		"uppercase": `["Noise.V1"]`,
		"empty":     `[""]`,
		"string":    `"noise.v1"`,
		"object":    `{}`,
		"oversize":  `[` + manyCaps(17) + `]`,
		"too long":  `["` + strings.Repeat("a", 33) + `"]`,
	} {
		if _, err := DecodeHelloAck(with(v)); err == nil {
			t.Errorf("%s: %s accepted", name, v)
		}
	}
	dup := []byte(strings.TrimSuffix(base, "}") + `,"capabilities":[],"capabilities":[]}`)
	if _, err := DecodeHelloAck(dup); err == nil {
		t.Error("duplicate capabilities key accepted")
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := EncodeControl(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ---- fuzzing -----------------------------------------------------------------

func FuzzHelloCapabilities(f *testing.F) {
	for _, s := range []string{
		`[]`, `null`, `["noise.v1"]`, `["carrier.h2","noise.v1"]`, `["noise.v1","noise.v1"]`,
		`["noise.v1","carrier.h2"]`, `["NOISE.V1"]`, `["Noise.v1"]`, `[""]`, `["x-future.v9"]`,
		`["` + strings.Repeat("a", 33) + `"]`, `[` + manyCaps(17) + `]`, `[` + manyCaps(16) + `]`,
		`["a\u0000b"]`, `"noise.v1"`, `{}`, `[null]`, `[1]`, `["-x"]`, `["carrier.ws"]`,
	} {
		f.Add([]byte(s))
	}
	local := CapabilityOffer{Advertise: []string{CapCarrierH2, CapNoiseV1}, Expect: []string{CapNoiseV1}}
	f.Fuzz(func(t *testing.T, caps []byte) {
		for _, payload := range [][]byte{helloWithCaps(string(caps)), append(append([]byte(strings.TrimSuffix(string(mustEncodeF(ackPayload(nil))), "}")), []byte(`,"capabilities":`)...), append(caps, '}')...)} {
			var list []string
			if h, err := DecodeHello(payload); err == nil {
				list = h.Capabilities
			} else if a, err := DecodeHelloAck(payload); err == nil {
				list = a.Capabilities
			} else {
				continue
			}
			if len(list) > MaxCapabilities {
				t.Fatalf("accepted oversize list %q", list)
			}
			for i, c := range list {
				if !validCapabilityToken(c) || (i > 0 && list[i-1] >= c) {
					t.Fatalf("accepted non-canonical list %q", list)
				}
			}
			n, err := NegotiateCapabilities(local, list)
			if err != nil {
				var ce *CapabilityError
				if !errors.As(err, &ce) {
					t.Fatalf("untyped refusal: %v", err)
				}
				continue
			}
			for _, c := range n.Enabled {
				if !IsKnownCapability(c) {
					t.Fatalf("enabled unknown capability %q", c)
				}
			}
			if n.Has(CapCarrierWS) && !n.Has(CapNoiseV1) {
				t.Fatal("ws enabled without noise")
			}
		}
	})
}

func mustEncodeF(v any) []byte {
	b, err := EncodeControl(v)
	if err != nil {
		panic(err)
	}
	return b
}
