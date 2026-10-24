package bcc

import (
	"reflect"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestIngressIntentRetryCannotMutateStoredCommandOrReceipt(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		name := "legacy-intent"
		if reviewed {
			name = "reviewed-intent"
		}
		t.Run(name, func(t *testing.T) {
			s, spec, cmd, token := stagedIngressBoundary(t)
			receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "unverified", FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{OwnerTag: "original-observation"}}}
			if err := s.RecordIngressAgentReceipt(receipt, token, time.Now()); err != nil {
				t.Fatal(err)
			}
			before, err := s.snapshotState()
			if err != nil {
				t.Fatal(err)
			}
			stored, ok := s.GetPublicIngressOperation(spec.TunnelID)
			if !ok {
				t.Fatal("missing operation")
			}
			var result PublicIngressOperation
			if reviewed {
				result, err = s.RecordReviewedPublicIngressOperation(spec, stored.ReviewedPlanHash, stored.Proof, time.Now(), ingressAudit())
			} else {
				result, err = s.RecordPublicIngressOperationIntent(spec, stored.Proof, time.Now(), ingressAudit())
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Command == nil || result.Receipt == nil {
				t.Fatal("idempotent return lost staged evidence")
			}
			result.Command.JobID = "caller-replaced-signed-job"
			result.Receipt.Binding.PlanSHA256 = "caller-replaced-reviewed-hash"
			result.Receipt.FirewallEvidence[0].OwnerTag = "caller-replaced-host-observation"
			after, err := s.snapshotState()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("idempotent intent returned aliases into persisted command/receipt fences")
			}
			reopened, err := OpenStore(s.Path())
			if err != nil {
				t.Fatal(err)
			}
			disk, ok := reopened.GetPublicIngressOperation(spec.TunnelID)
			live, liveOK := s.GetPublicIngressOperation(spec.TunnelID)
			if !ok || !liveOK || !reflect.DeepEqual(disk, live) {
				t.Fatal("caller mutation diverged live and durable operation identities")
			}
		})
	}
}

func TestStagedIngressCommandJobIDIsUniqueAcrossOperations(t *testing.T) {
	s, spec, cmd, _ := stagedIngressBoundary(t)
	second := spec
	second.TunnelID = "tun-second"
	second.PublicPort++
	second.LocalTarget = "127.0.0.1:1444"
	s.mu.Lock()
	tunnel := s.st.Tunnels[spec.TunnelID]
	tunnel.ID, tunnel.ClientPort, tunnel.RouteListen = second.TunnelID, second.PublicPort, second.LocalTarget
	s.st.Tunnels[tunnel.ID] = tunnel
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, second, 1)
	op, err := s.RecordReviewedPublicIngressOperation(second, tunnel.PlanHash, proof, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	other := cmd
	other.Binding.OperationID, other.Binding.LeaseID = op.OperationID, op.LeaseID
	other.Binding.Port, other.Binding.ConfigPort = uint16(second.PublicPort), uint16(second.PublicPort)
	other.Binding.Spec = publicIngressManifestSpec(second, tunnel.InstanceID, proof.Generation)
	other.Binding.Proof, err = ingressmanifest.Preview(other.Binding.Spec)
	if err != nil {
		t.Fatal(err)
	}
	other.Binding.ListenEndpoint, other.Binding.LocalTarget = other.Binding.Spec.Config.Listen, second.LocalTarget
	other.Binding.ListenerID, other.Binding.FirewallID = proof.ListenerID, proof.FirewallID
	before, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StageReviewedIngressCommand(second.TunnelID, other, time.Now()); err == nil {
		t.Fatal("same signed job ID staged for two independent operations")
	}
	after, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected duplicate changed operation state")
	}
	other.JobID = "agent-job-reviewed-2"
	if err := s.StageReviewedIngressCommand(second.TunnelID, other, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(s.Path()); err != nil {
		t.Fatalf("valid separately identified commands cannot reopen: %v", err)
	}
}
