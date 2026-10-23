package ingressauthority

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func bindingFixture(t *testing.T) BindingV1 {
	t.Helper()
	spec := ingressmanifest.Spec{TunnelID: "slice-1", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:1443", Target: "127.0.0.1:2443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	return BindingV1{NodeID: "ir-1", OperationID: "op-1", LeaseID: "lease-1", Port: 1443, ListenEndpoint: "0.0.0.0:1443", LocalTarget: "127.0.0.1:2443", Transport: "dedicated", PlanSHA256: strings.Repeat("a", 64), ListenerID: "listener-1", FirewallID: "fw-1", Spec: spec, Proof: proof}
}

func TestBindingV1CanonicalAndIdentity(t *testing.T) {
	b := bindingFixture(t)
	if err := b.Validate(); err != nil {
		t.Fatalf("canonical binding rejected: %v", err)
	}
	variants := map[string]func(*BindingV1){
		"port mismatch":       func(v *BindingV1) { v.Port = 2053 },
		"wrong proof":         func(v *BindingV1) { v.Proof.ConfigSHA256 = strings.Repeat("b", 64) },
		"generation alias":    func(v *BindingV1) { v.Spec.Generation = 2 },
		"target alias":        func(v *BindingV1) { v.LocalTarget = "127.0.0.1:2444" },
		"noncanonical listen": func(v *BindingV1) { v.ListenEndpoint = "0.0.0.0:01443" },
		"non-dedicated":       func(v *BindingV1) { v.Transport = "mux" },
		"wrong plan":          func(v *BindingV1) { v.PlanSHA256 = "abcd" },
		"invalid node":        func(v *BindingV1) { v.NodeID = "../other" },
	}
	for name, change := range variants {
		t.Run(name, func(t *testing.T) {
			v := b
			change(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("invalid signed ingress binding accepted")
			}
		})
	}
	other := b
	other.LeaseID = "lease-2"
	if err := b.MatchReviewed(other); err == nil {
		t.Fatal("changed lease matched reviewed binding")
	}
	if err := b.MatchReviewed(b); err != nil {
		t.Fatal(err)
	}
}

func TestCommandIdentitySeparateFromLease(t *testing.T) {
	b := bindingFixture(t)
	start := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	first := CommandV1{JobID: "signed-apply-1", Action: "public_ingress_apply", IssuedAt: start, ExpiresAt: start.Add(time.Minute), Binding: b}
	retire := first
	retire.JobID = "signed-retire-2"
	retire.Action = "public_ingress_retire"
	if err := first.Validate(start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := retire.Validate(start.Add(time.Second)); err != nil {
		t.Fatalf("retire with new signed job ID must keep same lease: %v", err)
	}
	if retire.Binding != first.Binding {
		t.Fatal("command job ID changed resource owner")
	}
	if err := first.Validate(start.Add(2 * time.Minute)); err == nil {
		t.Fatal("expired signed command admitted")
	}
}

func TestNoForgedLiveLeaseFromZeroValue(t *testing.T) {
	var lease LiveLease
	if err := lease.CheckLockedIngress(context.Background(), bindingFixture(t)); err == nil {
		t.Fatal("unminted lease accepted")
	}
}
