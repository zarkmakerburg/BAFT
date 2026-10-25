package bcc

import (
	"github.com/zarkmakerburg/baft/internal/release"
	"testing"
	"time"
)

func TestTypedIngressExpiryDoesNotBlockOtherJobsOrReleaseClaim(t *testing.T) {
	s, spec, c, token := stagedIngressBoundary(t)
	op := s.st.PublicIngressOperations[spec.TunnelID]
	op.Command = nil
	s.st.PublicIngressOperations[spec.TunnelID] = op
	now := time.Now().UTC().Truncate(time.Second)
	c.IssuedAt = now.Add(-time.Second)
	c.ExpiresAt = now.Add(2 * time.Second)
	if err := s.StageReviewedIngressCommand(spec.TunnelID, c, now); err != nil {
		t.Fatal(err)
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
	if _, err := s.pullJobs(c.Binding.NodeID, token, func(j []Job) error { _, err := app.signAgentJobs(j); return err }); err != nil {
		t.Fatal(err)
	}
	if err := s.AckJob(c.Binding.NodeID, token, c.JobID, "succeeded", "unproven"); err == nil {
		t.Fatal("generic ACK manufactured operational success without receipt")
	}
	// Exercise the real wall-clock expiry seen by PullJobs, not a forced status.
	time.Sleep(time.Until(c.ExpiresAt.Add(time.Millisecond)))
	others, err := s.CreateDeployJobs([]string{c.Binding.NodeID}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := s.pullJobs(c.Binding.NodeID, token, func(j []Job) error { _, err := app.signAgentJobs(j); return err })
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || delivered[0].ID != others[0].ID {
		t.Fatal("expired ingress poisoned unrelated signed job dispatch")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.Jobs[c.JobID].Status != "failed" || reopened.st.PublicIngressOperations[spec.TunnelID].Status != PublicIngressPending {
		t.Fatal("expiry lost uncertain ingress state")
	}
	if _, ok := reopened.st.PublicIngressReservations[spec.TunnelID]; !ok {
		t.Fatal("job expiry freed durable ingress claim")
	}
}
