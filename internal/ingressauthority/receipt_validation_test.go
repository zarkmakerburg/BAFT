package ingressauthority

import (
	"testing"
	"time"
)

func TestReceiptShapeNeverConfusesBlockedAppliedAndRetired(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := CommandV1{IngressProtocol: ProtocolV1, JobID: "job-1", Action: "public_ingress_apply", IssuedAt: now, ExpiresAt: now.Add(time.Minute), Binding: bindingFixture(t)}
	applied := ReceiptV1{SchemaVersion: 1, JobID: c.JobID, Action: c.Action, Binding: c.Binding, Phase: "applied", OwnershipVerified: true, RollbackStatus: "not_needed", ListenerEvidence: ListenerEvidenceV1{Endpoint: c.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:123"}, FirewallEvidence: []FirewallEvidenceV1{{Family: "inet", Table: "baft", Chain: "ingress", ChainHandle: 1, RuleHandle: 2, OwnerTag: FirewallOwnerTagV1(c.Binding.Port, c.Binding.Proof)}}}
	if err := applied.ValidateForCommand(c); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ReceiptV1){
		"positive-blocked":  func(r *ReceiptV1) { r.Phase = "blocked" },
		"wrong-action":      func(r *ReceiptV1) { r.Phase = "retired" },
		"unverified":        func(r *ReceiptV1) { r.OwnershipVerified = false },
		"aliased-inode":     func(r *ReceiptV1) { r.ListenerEvidence.SocketIdentity = "inode:0123" },
		"foreign-family":    func(r *ReceiptV1) { r.ListenerEvidence.Family = "tcp6" },
		"wrong-config-port": func(r *ReceiptV1) { r.Binding.ConfigPort++ },
		"wrong-job":         func(r *ReceiptV1) { r.JobID = "other" },
		"foreign-owner":     func(r *ReceiptV1) { r.FirewallEvidence[0].OwnerTag = "foreign" },
		"missing-handle":    func(r *ReceiptV1) { r.FirewallEvidence[0].RuleHandle = 0 },
		"duplicate-rule":    func(r *ReceiptV1) { r.FirewallEvidence = append(r.FirewallEvidence, r.FirewallEvidence[0]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := applied
			r.FirewallEvidence = append([]FirewallEvidenceV1(nil), applied.FirewallEvidence...)
			change(&r)
			if err := r.ValidateForCommand(c); err == nil {
				t.Fatal("mixed or foreign evidence accepted")
			}
		})
	}
	blocked := ReceiptV1{SchemaVersion: 1, JobID: c.JobID, Action: c.Action, Binding: c.Binding, Phase: "blocked", FailureCause: "authority unavailable", RollbackStatus: "not_attempted"}
	if err := blocked.ValidateForCommand(c); err != nil {
		t.Fatal(err)
	}
	c.Action = "public_ingress_retire"
	c.JobID = "retire-2"
	retired := applied
	retired.Action = c.Action
	retired.JobID = c.JobID
	retired.Phase = "retired"
	retired.ListenerEvidence = ListenerEvidenceV1{}
	if err := retired.ValidateForCommand(c); err != nil {
		t.Fatal(err)
	}
}
