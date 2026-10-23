package ingressownership

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func executorBinding(t *testing.T) IngressBindingV1 {
	t.Helper()
	spec := ingressmanifest.Spec{
		TunnelID: "contract-owner", InstanceID: "ir-a", Generation: 1,
		Config: ingressmanifest.Config{
			Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443",
			MaxClients: 128, DialTimeoutMS: 3000,
		},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	return IngressBindingV1{
		JobID: "job:001", NodeID: "ir-a", OperationID: "op:001",
		LeaseID: "lease:001", ListenerID: "ir.a:public", FirewallID: "ingress:rule",
		Action: IngressApplyAction, Transport: IngressDedicated,
		PlanSHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Hour), PublicPort: 54619, ConfigPort: 54619,
		Spec: spec, Proof: proof,
	}
}

func TestIngressBindingV1RefusesAliasedOrUnreviewedInputs(t *testing.T) {
	good := executorBinding(t)
	if err := ValidateIngressBindingV1(good); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	for name, change := range map[string]func(*IngressBindingV1){
		"node":                       func(b *IngressBindingV1) { b.NodeID = "" },
		"job-control":                func(b *IngressBindingV1) { b.JobID = "bad\njob" },
		"operation":                  func(b *IngressBindingV1) { b.OperationID = "" },
		"lease":                      func(b *IngressBindingV1) { b.LeaseID = "" },
		"listener-id":                func(b *IngressBindingV1) { b.ListenerID = "../bad" },
		"firewall-id":                func(b *IngressBindingV1) { b.FirewallID = "" },
		"plan-hash":                  func(b *IngressBindingV1) { b.PlanSHA256 = strings.ToUpper(b.PlanSHA256) },
		"plan-hash-missing":          func(b *IngressBindingV1) { b.PlanSHA256 = "" },
		"unknown-action":             func(b *IngressBindingV1) { b.Action = "public_ingress_preflight" },
		"multiplex-unverified":       func(b *IngressBindingV1) { b.Transport = "shared_tls" },
		"zero-port":                  func(b *IngressBindingV1) { b.PublicPort = 0 },
		"ssh-port":                   func(b *IngressBindingV1) { b.PublicPort = 22; b.ConfigPort = 22; b.Spec.Config.Listen = "0.0.0.0:22" },
		"client-config-port-drift":   func(b *IngressBindingV1) { b.ConfigPort = 54620 },
		"public-port-drift":          func(b *IngressBindingV1) { b.PublicPort = 54620; b.ConfigPort = 54620 },
		"noncanonical-listen":        func(b *IngressBindingV1) { b.Spec.Config.Listen = "0.0.0.0:054619" },
		"noncanonical-target":        func(b *IngressBindingV1) { b.Spec.Config.Target = "127.0.0.1:01443" },
		"omitted-client-limit-alias": func(b *IngressBindingV1) { b.Spec.Config.MaxClients = 0 },
		"omitted-dial-timeout-alias": func(b *IngressBindingV1) { b.Spec.Config.DialTimeoutMS = 0 },
		"nonliteral-target":          func(b *IngressBindingV1) { b.Spec.Config.Target = "localhost:1443" },
		"forged-config-digest":       func(b *IngressBindingV1) { b.Proof.ConfigSHA256 = strings.Repeat("b", 64) },
		"forged-manifest-digest":     func(b *IngressBindingV1) { b.Proof.ManifestSHA256 = strings.Repeat("b", 64) },
		"generation-drift":           func(b *IngressBindingV1) { b.Spec.Generation++ },
		"instance-drift":             func(b *IngressBindingV1) { b.Spec.InstanceID = "ir-b" },
		"target-drift":               func(b *IngressBindingV1) { b.Spec.Config.Target = "127.0.0.1:1444" },
	} {
		t.Run(name, func(t *testing.T) {
			b := good
			change(&b)
			if err := ValidateIngressBindingV1(b); !errors.Is(err, ErrInvalid) {
				t.Fatalf("unsafe signed binding accepted: %v", err)
			}
		})
	}
	// No magic port denylist; validation does not claim the port is free.
	b := good
	b.Spec.Config.Listen = "0.0.0.0:2053"
	b.PublicPort, b.ConfigPort = 2053, 2053
	var err error
	b.Proof, err = ingressmanifest.Preview(b.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateIngressBindingV1(b); err != nil {
		t.Fatalf("arbitrary user port incorrectly reserved: %v", err)
	}
}

type contractLeaseTestDouble struct {
	calls int
	err   error
}

func (l *contractLeaseTestDouble) CheckLockedIngress(_ context.Context, _ IngressBindingV1) error {
	l.calls++
	return l.err
}
func TestIngressExecutorMissingLeaseAlwaysFailsClosed(t *testing.T) {
	b := executorBinding(t)
	if err := CheckIngressLeaseV1(context.Background(), nil, b); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil lease accepted: %v", err)
	}
	var typedNil *contractLeaseTestDouble
	if err := CheckIngressLeaseV1(context.Background(), typedNil, b); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("typed-nil authority accepted or panicked: %v", err)
	}
	authority := &contractLeaseTestDouble{}
	if err := CheckIngressLeaseV1(nil, authority, b); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil context accepted: %v", err)
	}
	if authority.calls != 0 {
		t.Fatal("missing context still reached authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckIngressLeaseV1(ctx, authority, b); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context accepted: %v", err)
	}
	if authority.calls != 0 {
		t.Fatal("canceled operation reached authority")
	}
	invalid := b
	invalid.PublicPort++
	if err := CheckIngressLeaseV1(context.Background(), authority, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid manifest reached authority: %v", err)
	}
	if authority.calls != 0 {
		t.Fatal("invalid signed identity reached authority")
	}
	expired := b
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if err := CheckIngressLeaseV1(context.Background(), authority, expired); !errors.Is(err, ErrExecutorAdmissionExpired) {
		t.Fatalf("expired signed action accepted: %v", err)
	}
	if authority.calls != 0 {
		t.Fatal("expired operation reached lease authority")
	}
	authority.err = ErrUnknown
	if err := CheckIngressLeaseV1(context.Background(), authority, b); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown cross-process ownership accepted: %v", err)
	}
	if authority.calls != 1 {
		t.Fatalf("locked lease checked %d times, want 1", authority.calls)
	}
	authority.err = nil
	if err := CheckIngressLeaseV1(context.Background(), authority, b); err != nil {
		t.Fatalf("valid isolated trust-test double rejected: %v", err)
	}
	// A successful check is NOT an Apply, a resource lease implementation,
	// or an authenticated receipt; this test deliberately does not mutate.
	if authority.calls != 2 {
		t.Fatalf("unexpected authority call count: %d", authority.calls)
	}
}
