package bcc

import (
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
	"testing"
	"time"
)

// Candidates below test BCC identity/admission only, not physical Apply/Retire.
func typedApplyReceiptCandidate(c ingressauthority.CommandV1) ingressauthority.ReceiptV1 {
	return ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: c.JobID, Action: c.Action, Binding: c.Binding, Phase: "applied", OwnershipVerified: true, RollbackStatus: "not_needed", ListenerEvidence: ingressauthority.ListenerEvidenceV1{Endpoint: c.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "inode:456"}, FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{Family: "inet", Table: "baft", Chain: "ingress", ChainHandle: 4, RuleHandle: 5, OwnerTag: ingressauthority.FirewallOwnerTagV1(c.Binding.Port, c.Binding.Proof)}}}
}

func TestTypedIngressRetireKeepsSeparateImmutableHistory(t *testing.T) {
	s, spec, apply, token := stagedIngressBoundary(t)
	now := time.Now().UTC()
	retire := apply
	retire.Action = "public_ingress_retire"
	retire.JobID = "retire-new-job"
	if err := s.StageReviewedIngressRetireCommand(spec.TunnelID, retire, now); err == nil {
		t.Fatal("retirement before applied evidence accepted")
	}
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.enqueueReviewedIngress(spec.TunnelID, now, ingressAudit()); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	dispatch := func() {
		t.Helper()
		if _, err := s.pullJobs(apply.Binding.NodeID, token, func(j []Job) error { _, err := app.signAgentJobs(j); return err }); err != nil {
			t.Fatal(err)
		}
	}
	dispatch()
	applied := typedApplyReceiptCandidate(apply)
	if err := s.RecordIngressAgentReceipt(applied, token, now); err != nil {
		t.Fatal(err)
	}
	if err := s.StageReviewedIngressRetireCommand(spec.TunnelID, retire, now); err != nil {
		t.Fatal(err)
	}
	staged := stateJSONForTest(t, s)
	if err := s.StageReviewedIngressRetireCommand(spec.TunnelID, retire, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if string(staged) != string(stateJSONForTest(t, s)) {
		t.Fatal("idempotent retirement restaged")
	}
	other := retire
	other.JobID = "different-retire"
	if err := s.StageReviewedIngressRetireCommand(spec.TunnelID, other, now); err == nil {
		t.Fatal("retirement identity replaced")
	}
	if _, err := s.enqueueReviewedIngressRetire(spec.TunnelID, now, ingressAudit()); err != nil {
		t.Fatal(err)
	}
	dispatch()
	retired := applied
	retired.JobID = retire.JobID
	retired.Action = retire.Action
	retired.Phase = "retired"
	retired.ListenerEvidence = ingressauthority.ListenerEvidenceV1{}
	wrong := retired
	wrong.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), retired.FirewallEvidence...)
	wrong.FirewallEvidence[0].RuleHandle++
	before := stateJSONForTest(t, s)
	if err := s.RecordIngressAgentReceipt(wrong, token, now); err == nil {
		t.Fatal("retirement accepted replacement handle")
	}
	if string(before) != string(stateJSONForTest(t, s)) {
		t.Fatal("refused retirement receipt changed durable state")
	}
	if err := s.RecordIngressAgentReceipt(retired, token, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordIngressAgentReceipt(retired, token, retire.ExpiresAt.Add(time.Second)); err != nil {
		t.Fatal("recorded exact retire ACK cannot retry after expiry")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, _ := reopened.GetPublicIngressOperation(spec.TunnelID)
	if op.Command == nil || *op.Command != apply || op.Receipt == nil || op.Receipt.JobID != apply.JobID || op.Retirement == nil || op.Retirement.Command != retire || op.Retirement.Receipt == nil || op.Retirement.Receipt.Phase != "retired" || op.Status != PublicIngressPending {
		t.Fatal("retirement overwrote Apply evidence or marked Wizard Ready")
	}
	op.Retirement.Command.JobID = "modified"
	op.Retirement.Receipt.FirewallEvidence[0].RuleHandle++
	original, _ := reopened.GetPublicIngressOperation(spec.TunnelID)
	if original.Retirement.Command != retire || original.Retirement.Receipt.FirewallEvidence[0].RuleHandle != applied.FirewallEvidence[0].RuleHandle {
		t.Fatal("retirement getter leaked mutable state")
	}
	for _, alter := range []func(*PublicIngressOperation){func(op *PublicIngressOperation) { op.Retirement.Command.JobID = apply.JobID }, func(op *PublicIngressOperation) { op.Retirement.Command.Binding.LeaseID = "different" }, func(op *PublicIngressOperation) { op.Retirement.Receipt.FirewallEvidence[0].RuleHandle++ }} {
		snapshot, err := cloneState(reopened.st)
		if err != nil {
			t.Fatal(err)
		}
		op := snapshot.PublicIngressOperations[spec.TunnelID]
		alter(&op)
		snapshot.PublicIngressOperations[spec.TunnelID] = op
		if err := validatePersistedIngressOperations(snapshot); err == nil {
			t.Fatal("tampered retirement history restored")
		}
	}
}
