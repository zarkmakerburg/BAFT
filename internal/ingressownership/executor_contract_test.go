package ingressownership

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func executorCommandV1(t *testing.T) ingressauthority.CommandV1 {
	t.Helper()
	spec := ingressmanifest.Spec{
		TunnelID: "executor-contract", InstanceID: "ir-a", Generation: 1,
		Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	return ingressauthority.CommandV1{
		IngressProtocol: ingressauthority.ProtocolV1, JobID: "job-a",
		Action: "public_ingress_apply", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
		Binding: ingressauthority.BindingV1{
			NodeID: "ir-a", OperationID: "op-a", LeaseID: "lease-a",
			Port: 54619, ConfigPort: 54619, ListenEndpoint: spec.Config.Listen,
			LocalTarget: spec.Config.Target, Transport: "dedicated",
			PlanSHA256: strings.Repeat("a", 64), ListenerID: "listener-a",
			FirewallID: "firewall-a", Spec: spec, Proof: proof,
		},
	}
}

func TestExecutorConsumesOnlySharedOpaqueLeaseAndNeverMintsIt(t *testing.T) {
	cmd := executorCommandV1(t)
	if err := cmd.Validate(time.Now()); err != nil {
		t.Fatalf("shared contract invalid: %v", err)
	}
	if err := CheckIngressExecutorAuthority(context.Background(), nil, cmd); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil lease accepted: %v", err)
	}
	var zero ingressauthority.Lease
	if err := CheckIngressExecutorAuthority(context.Background(), &zero, cmd); !errors.Is(err, ingressauthority.ErrUnverified) {
		t.Fatalf("zero-value lease accepted: %v", err)
	}
	if err := CheckIngressExecutorAuthority(nil, &zero, cmd); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil context accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckIngressExecutorAuthority(ctx, &zero, cmd); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission accepted: %v", err)
	}
	expired := cmd
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if err := CheckIngressExecutorAuthority(context.Background(), &zero, expired); err == nil {
		t.Fatal("expired signed command accepted")
	}
	changed := cmd
	changed.Binding.Port++
	if err := CheckIngressExecutorAuthority(context.Background(), &zero, changed); !errors.Is(err, ingressauthority.ErrBinding) {
		t.Fatalf("changed port accepted: %v", err)
	}
	retire := cmd
	retire.Action = "public_ingress_retire"
	retire.JobID = "job-b"
	if err := retire.Validate(time.Now()); err != nil {
		t.Fatalf("same-owner new-job retire rejected: %v", err)
	}
	if err := CheckIngressExecutorAuthority(context.Background(), &zero, retire); !errors.Is(err, ingressauthority.ErrUnverified) {
		t.Fatalf("retire command with zero lease accepted: %v", err)
	}
	// Explicitly: no "positive fake permit" case. This package cannot mint
	// real authority, and none of these checks invokes runtime or nft.
}

func TestExecutorObservedEvidenceIsStrictButNotAuthority(t *testing.T) {
	cmd := executorCommandV1(t)
	plan, err := PlanIngressRule(cmd.Binding.Port, cmd.Binding.Proof)
	if err != nil {
		t.Fatal(err)
	}
	observed := ObservedIngressV1{
		Listener: ingressauthority.ListenerEvidenceV1{
			Endpoint: cmd.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:123456",
		},
		Firewall: ingressauthority.FirewallEvidenceV1{
			Family: "inet", Table: "baft", Chain: "ingress",
			ChainHandle: 44, RuleHandle: 55, OwnerTag: plan.Tag,
		},
		Chain: 44,
	}
	if err := ValidateObservedIngressV1(cmd.Binding, observed); err != nil {
		t.Fatalf("coherent synthetic evidence rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ObservedIngressV1){
		"endpoint":       func(e *ObservedIngressV1) { e.Listener.Endpoint = "127.0.0.1:54619" },
		"family":         func(e *ObservedIngressV1) { e.Listener.Family = "tcp6" },
		"unknown-socket": func(e *ObservedIngressV1) { e.Listener.SocketIdentity = "" },
		"foreign-owner":  func(e *ObservedIngressV1) { e.Firewall.OwnerTag = "other" },
		"wrong-handle":   func(e *ObservedIngressV1) { e.Firewall.RuleHandle = 0 },
		"wrong-chain":    func(e *ObservedIngressV1) { e.Firewall.ChainHandle++ },
		"unknown-chain":  func(e *ObservedIngressV1) { e.Chain = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			change := observed
			mutate(&change)
			if err := ValidateObservedIngressV1(cmd.Binding, change); err == nil {
				t.Fatal("unverified host observation accepted")
			}
		})
	}
}
