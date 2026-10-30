package bcc

import (
	"testing"
	"time"
)

// Ported from hq's TestTypedIngressExpiryDoesNotBlockOtherJobsOrReleaseClaim:
// an operational ingress job that expires after dispatch must not poison the
// delivery of unrelated jobs and must not free the durable ingress claim.
func TestOperationalIngressExpiryDoesNotBlockOtherJobsOrReleaseClaim(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, c, token := stagedIngressBoundary(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	cmd := *op.Command
	cmd.IssuedAt, cmd.ExpiresAt = now.Add(-time.Second), now.Add(2*time.Second)
	op.Command = &cmd
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if jobs, err := s.PullJobs(c.Binding.NodeID, token); err != nil || len(jobs) != 1 || jobs[0].ID != cmd.JobID {
		t.Fatalf("operational job not delivered once: %v %+v", err, jobs)
	}
	// A generic acknowledgement never makes the operation anything but PENDING.
	if err := s.AckJob(c.Binding.NodeID, token, cmd.JobID, "succeeded", "unproven"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetPublicIngressOperation(spec.TunnelID); got.Status != PublicIngressPending || got.Receipt != nil {
		t.Fatalf("generic ACK manufactured operational success: %+v", got)
	}

	// Real wall-clock expiry, not a forced status.
	time.Sleep(time.Until(cmd.ExpiresAt.Add(time.Millisecond)))
	others, err := s.CreateDeployJobs([]string{c.Binding.NodeID}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := s.PullJobs(c.Binding.NodeID, token)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || delivered[0].ID != others[0].ID {
		t.Fatalf("expired ingress poisoned unrelated job dispatch: %+v", delivered)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.PublicIngressOperations[spec.TunnelID].Status != PublicIngressPending {
		t.Fatal("expiry lost uncertain ingress state")
	}
	if _, ok := reopened.st.PublicIngressReservations[spec.TunnelID]; !ok {
		t.Fatal("job expiry freed the durable ingress claim")
	}
}
