//go:build unix

package ingressauthority

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// signedEvidence returns sealed verified evidence for a job matching command.
func signedEvidence(t *testing.T, c CommandV1) agentjob.VerifiedJob {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b := c.Binding
	env, err := agentjob.Sign(priv, agentjob.Job{JobID: c.JobID, NodeID: b.NodeID, Action: c.Action,
		IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt, Params: map[string]string{
			"ingress_protocol": ProtocolV1, "tunnel_id": b.Spec.TunnelID, "generation": "1",
			"public_port": "1443", "local_target": b.LocalTarget,
			"manifest_digest": b.Proof.ManifestSHA256, "config_sha256": b.Proof.ConfigSHA256,
			"listen_endpoint": b.ListenEndpoint, "operation_id": b.OperationID, "lease_id": b.LeaseID,
			"plan_sha256": b.PlanSHA256, "transport": b.Transport, "listener_id": b.ListenerID, "firewall_id": b.FirewallID,
		}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := (agentjob.Verifier{BCCKey: pub, NodeID: b.NodeID}).VerifyAdmitted(env, time.Now())
	if err != nil {
		t.Fatalf("fixture job not admitted: %v", err)
	}
	return v
}

func TestMintLeaseRefusesWithoutSealedEvidenceOrHeldPolicy(t *testing.T) {
	b := bindingFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	cmd := CommandV1{IngressProtocol: ProtocolV1, JobID: "apply-mint-1", Action: "public_ingress_apply",
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := g.MintLease(context.Background(), agentjob.VerifiedJob{}, cmd); err == nil || lease != nil {
		t.Fatal("zero VerifiedJob minted a lease")
	}
	ev := signedEvidence(t, cmd)
	if lease, err := g.MintLease(context.Background(), ev, cmd); err == nil || lease != nil {
		t.Fatalf("lease minted without pinned installed policy and recorded intent: %v", err)
	}
	if lease, err := ((*PendingGuard)(nil)).MintLease(context.Background(), ev, cmd); err == nil || lease != nil {
		t.Fatal("nil guard minted")
	}
	other := cmd
	other.JobID = "different"
	if _, err := g.MintLease(context.Background(), ev, other); err == nil {
		t.Fatal("evidence for another job accepted")
	}
	_ = g.Close()
	if lease, err := g.MintLease(context.Background(), ev, cmd); err == nil || lease != nil {
		t.Fatal("closed guard minted")
	}
}
