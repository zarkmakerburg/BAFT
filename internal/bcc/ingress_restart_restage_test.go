package bcc

import (
	"reflect"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// appliedQueuedIngress: a held Apply that was queued and then reported
// applied and ownership-verified by the enrolled Agent.
func appliedQueuedIngress(t *testing.T) (*Store, PublicIngressSpec, ingressauthority.CommandV1, string) {
	t.Helper()
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordIngressAgentReceipt(appliedReceipt(cmd), token, time.Now()); err != nil {
		t.Fatal(err)
	}
	return s, spec, cmd, token
}

func restartLossReceipt(cmd ingressauthority.CommandV1) ingressauthority.ReceiptV1 {
	return ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "failed",
		FailureCause:   "operational ingress interrupted by an agent restart; the owned rule was removed after kernel evidence and the reservation was released",
		RollbackStatus: ingressauthority.RollbackReleasedAfterRestart}
}

// A restart (or reboot) that loses an applied ingress must not leave the
// public port closed for good: BCC answers the exact, authenticated loss
// receipt with a fresh HELD Apply for the same reviewed plan at the next
// generation, and the production gate then queues it like the first one.
func TestRestartLossReceiptRestagesFreshApplyAtNextGeneration(t *testing.T) {
	s, spec, cmd, token := appliedQueuedIngress(t)
	before, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if err := s.RecordIngressAgentReceipt(restartLossReceipt(cmd), token, time.Now()); err != nil {
		t.Fatalf("restart-loss receipt refused: %v", err)
	}
	got, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if got.Receipt != nil || got.Dispatch == nil || got.Dispatch.Phase != ingressPhaseHeld || got.RestartRestages != 1 {
		t.Fatalf("not restaged as a held Apply: %+v", got)
	}
	if got.OperationID == before.OperationID || got.LeaseID == before.LeaseID || got.Command.JobID == cmd.JobID {
		t.Fatal("restage reused an identity of the lost operation")
	}
	if got.Proof.Generation != before.Proof.Generation+1 || got.Command.Binding.Spec.Generation != got.Proof.Generation ||
		got.Proof.ManifestDigest == before.Proof.ManifestDigest || got.Command.Binding.Proof.ManifestSHA256 != got.Proof.ManifestDigest {
		t.Fatalf("restage is not at the next generation: before=%+v after=%+v", before.Proof, got.Proof)
	}
	if got.ReviewedPlanHash != before.ReviewedPlanHash || got.Command.Binding.PlanSHA256 != cmd.Binding.PlanSHA256 ||
		got.Command.Binding.Port != cmd.Binding.Port || got.Spec != before.Spec {
		t.Fatal("restage changed the reviewed plan, port or spec")
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatalf("restaged operation fails durable validation: %v", err)
	}
	if st := s.IngressOperationalState(spec.TunnelID, nil, time.Now()); st == nil || st.State != "held" || st.Ready {
		t.Fatalf("state after restage: %+v", st)
	}
	// The ordinary poll-time retry queues it through the production gate.
	jobsBefore := len(s.ListJobs())
	s.RetryHeldOperationalIngress(spec.IRNode)
	queued, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if queued.Dispatch.Phase != ingressPhaseQueued || len(s.ListJobs()) != jobsBefore+1 {
		t.Fatalf("restaged Apply was not queued: %+v jobs=%d->%d", queued.Dispatch, jobsBefore, len(s.ListJobs()))
	}
	// The old loss receipt cannot be replayed into a second restage.
	if err := s.RecordIngressAgentReceipt(restartLossReceipt(cmd), token, time.Now()); err == nil {
		t.Fatal("replayed restart-loss receipt accepted")
	}
	again, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if again.OperationID != queued.OperationID || again.RestartRestages != 1 {
		t.Fatal("replay changed the operation")
	}
	// The new Apply is applied again; a second restart restages once more.
	next := *queued.Command
	if err := s.RecordIngressAgentReceipt(appliedReceipt(next), token, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordIngressAgentReceipt(restartLossReceipt(next), token, time.Now()); err != nil {
		t.Fatal(err)
	}
	second, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if second.RestartRestages != 2 || second.Proof.Generation != queued.Proof.Generation+1 {
		t.Fatalf("second restart: %+v", second)
	}
}

// Only the exact shape is accepted; every other receipt after an applied one
// stays a conflicting replay and changes nothing.
func TestRestartLossReceiptRefusedUnlessExact(t *testing.T) {
	cases := map[string]func(s *Store, spec PublicIngressSpec, r *ingressauthority.ReceiptV1){
		"still fenced": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) {
			r.RollbackStatus = "reconciliation_required"
		},
		"no cause": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) { r.FailureCause = "" },
		"claims ownership": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) {
			r.OwnershipVerified = true
		},
		"carries evidence": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) {
			r.FirewallEvidence = []ingressauthority.FirewallEvidenceV1{{Family: "ip", Table: "baft", Chain: "ingress", ChainHandle: 3, RuleHandle: 9}}
		},
		"blocked phase": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) { r.Phase = "blocked" },
		"other binding": func(_ *Store, _ PublicIngressSpec, r *ingressauthority.ReceiptV1) { r.Binding.LeaseID = "lease-other" },
		"retire in progress": func(s *Store, spec PublicIngressSpec, _ *ingressauthority.ReceiptV1) {
			if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err != nil {
				panic(err)
			}
		},
		"tunnel not active": func(s *Store, spec PublicIngressSpec, _ *ingressauthority.ReceiptV1) {
			s.mu.Lock()
			tn := s.st.Tunnels[spec.TunnelID]
			tn.Phase = TunnelDecommissioningIR
			s.st.Tunnels[spec.TunnelID] = tn
			s.mu.Unlock()
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, spec, cmd, token := appliedQueuedIngress(t)
			r := restartLossReceipt(cmd)
			mutate(s, spec, &r)
			before, _ := s.GetPublicIngressOperation(spec.TunnelID)
			if err := s.RecordIngressAgentReceipt(r, token, time.Now()); err == nil {
				t.Fatal("accepted")
			}
			after, _ := s.GetPublicIngressOperation(spec.TunnelID)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("refused receipt changed the operation:\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
}

// The loss receipt must come from the enrolled IR Agent credential.
func TestRestartLossReceiptRequiresAgentCredential(t *testing.T) {
	s, spec, cmd, _ := appliedQueuedIngress(t)
	before, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if err := s.RecordIngressAgentReceipt(restartLossReceipt(cmd), "not-the-agent-token", time.Now()); err == nil {
		t.Fatal("unauthenticated restart-loss receipt accepted")
	}
	after, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unauthenticated receipt changed the operation")
	}
}
