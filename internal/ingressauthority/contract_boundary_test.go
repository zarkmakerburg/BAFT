package ingressauthority

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestBoundaryAllowsOwnedPolicyPortsWithoutBlanketPrivilegedBan(t *testing.T) {
	for _, port := range []uint16{443, 2053, 54619} {
		b := bindingFixture(t)
		b.Port = port
		b.ConfigPort = port
		b.ListenEndpoint = fmtEndpoint(port)
		b.Spec.Config.Listen = b.ListenEndpoint
		var err error
		b.Proof, err = ingressmanifest.Preview(b.Spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Validate(); err != nil {
			t.Fatalf("pure shape validation banned port %d: %v", port, err)
		}
	}
}

func fmtEndpoint(port uint16) string {
	return "0.0.0.0:" + strconv.Itoa(int(port))
}

func TestBoundaryRequiresExplicitEffectiveDefaults(t *testing.T) {
	b := bindingFixture(t)
	b.Spec.Config.MaxClients = 0
	if err := b.Validate(); err == nil {
		t.Fatal("implicit signed config default accepted")
	}
}

func TestBoundaryMissingProtocolMustReject(t *testing.T) {
	now := time.Now()
	c := CommandV1{JobID: "legacy-job", Action: "public_ingress_apply", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: bindingFixture(t)}
	if err := c.Validate(now); err == nil {
		t.Fatal("unmarked legacy command entered operational v1")
	}
}

func TestBoundaryLiveLeaseCannotTravelAsJSON(t *testing.T) {
	var lease LiveLease
	if _, err := json.Marshal(&lease); err == nil {
		t.Fatal("opaque live lease serialized")
	}
	if err := json.Unmarshal([]byte(`{"minted":true}`), &lease); err == nil {
		t.Fatal("JSON accepted as a live lease")
	}
}
