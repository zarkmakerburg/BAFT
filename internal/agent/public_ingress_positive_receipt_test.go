package agent

import (
	"context"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

func TestOperationalIngressVerifiedAckRequiresExactState(t *testing.T) {
	valid := []byte(`{"recorded":true,"verified":true,"ready_to_apply":false,"reconciliation_pending":false}`)
	if err := validateIngressReceiptAck(valid, true); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"recorded":true,"verified":false,"ready_to_apply":false,"reconciliation_pending":true}`,
		`{"recorded":true,"verified":true,"ready_to_apply":true,"reconciliation_pending":false}`,
		`{"recorded":true,"verified":true,"ready_to_apply":false,"reconciliation_pending":true}`,
		`{"recorded":true,"verified":true,"verified":true,"ready_to_apply":false,"reconciliation_pending":false}`,
		`{"recorded":true,"verified":true,"ready_to_apply":false,"reconciliation_pending":false} {}`,
		`{"recorded":true,"verified":true,"ready_to_apply":false,"reconciliation_pending":null}`,
	} {
		if err := validateIngressReceiptAck([]byte(raw), true); err == nil {
			t.Fatalf("ambiguous/incomplete positive ACK accepted: %s", raw)
		}
	}
}

func TestOperationalIngressPositiveSenderRequiresConcreteLiveLease(t *testing.T) {
	command, err := ingressV1Command(validSignedIngressV1(t))
	if err != nil {
		t.Fatal(err)
	}
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding, Phase: "applied", OwnershipVerified: true, RollbackStatus: "not_needed",
		ListenerEvidence: ingressauthority.ListenerEvidenceV1{Endpoint: command.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:123"},
		FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{Family: "inet", Table: "baft", Chain: "ingress", ChainHandle: 1, RuleHandle: 2, OwnerTag: ingressauthority.FirewallOwnerTagV1(command.Binding.Port, command.Binding.Proof)}}}
	if err := receipt.ValidateForCommand(command); err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: Config{NodeID: command.Binding.NodeID, Token: "test", BCCURL: "https://unreachable.invalid"}}
	for _, lease := range []*ingressauthority.Lease{nil, new(ingressauthority.Lease)} {
		if err := a.postVerifiedIngressReceipt(context.Background(), receipt, command, lease); err == nil {
			t.Fatal("synthetic positive receipt bypassed concrete live authority")
		}
	}
}
