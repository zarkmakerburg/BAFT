package bcc

import (
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
	"testing"
	"time"
)

func TestTypedIngressQueuePreservesSignedCommandAcrossRestart(t *testing.T) {
	s, spec, c, _ := stagedIngressBoundary(t)
	now := time.Now().UTC()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	j, err := s.enqueueReviewedIngress(spec.TunnelID, now, ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if j.ID != c.JobID || j.IngressCommand == nil || *j.IngressCommand != c {
		t.Fatal("queue lost immutable command")
	}
	j.Params["lease_id"] = "changed"
	j.IngressCommand.Binding.LeaseID = "changed"
	again, err := s.enqueueReviewedIngress(spec.TunnelID, now.Add(time.Second), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if again.Params["lease_id"] != c.Binding.LeaseID || *again.IngressCommand != c || len(s.st.Jobs) != 1 {
		t.Fatal("queue getter mutation or retry duplicated command")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(reopened, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	app.now = func() time.Time { return now.Add(2 * time.Second) }
	signed, err := app.signAgentJobs([]Job{reopened.st.Jobs[c.JobID]})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (agentjob.Verifier{BCCKey: pub, NodeID: c.Binding.NodeID}).Verify(*signed[0].Signed, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ingressauthority.CommandFromJob(got)
	if err != nil || decoded != c {
		t.Fatalf("signing changed durable review identity: %+v %v", decoded, err)
	}
	if reopened.st.PublicIngressOperations[spec.TunnelID].Status != PublicIngressPending {
		t.Fatal("queue or signature falsely verified ingress")
	}
	beforeRetry := stateJSONForTest(t, reopened)
	if err := reopened.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatalf("public enqueue retry was not idempotent: %v", err)
	}
	if afterRetry := stateJSONForTest(t, reopened); string(afterRetry) != string(beforeRetry) {
		t.Fatal("idempotent public enqueue changed durable state")
	}
}

func TestTypedIngressQueueSaveFailureStaleNodeAndTamperedRestore(t *testing.T) {
	for _, failure := range []string{"save", "stale-node", "wrong-plan"} {
		t.Run(failure, func(t *testing.T) {
			s, spec, _, _ := stagedIngressBoundary(t)
			now := time.Now().UTC()
			if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
				t.Fatal(err)
			}
			originalPath := s.path
			switch failure {
			case "save":
				s.path = t.TempDir()
			case "stale-node":
				n := s.st.Nodes[spec.IRNode]
				n.AgentSeen = now.Add(-6 * time.Minute)
				s.st.Nodes[n.ID] = n
			case "wrong-plan":
				tun := s.st.Tunnels[spec.TunnelID]
				tun.PlanHash = "changed"
				s.st.Tunnels[tun.ID] = tun
			}
			before := stateJSONForTest(t, s)
			if _, err := s.enqueueReviewedIngress(spec.TunnelID, now, ingressAudit()); err == nil {
				t.Fatal("invalid enqueue accepted")
			}
			if string(before) != string(stateJSONForTest(t, s)) {
				t.Fatal("failed enqueue modified state")
			}
			s.path = originalPath
		})
	}
	s, spec, c, _ := stagedIngressBoundary(t)
	now := time.Now().UTC()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.enqueueReviewedIngress(spec.TunnelID, now, ingressAudit()); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*Job){func(j *Job) { j.IngressCommand.ExpiresAt = j.IngressCommand.ExpiresAt.Add(time.Second) }, func(j *Job) { j.Params["public_port"] = "54618" }, func(j *Job) { j.IngressCommand = nil }, func(j *Job) { j.ID = "other" }} {
		st, err := cloneState(s.st)
		if err != nil {
			t.Fatal(err)
		}
		j := st.Jobs[c.JobID]
		alter(&j)
		st.Jobs[c.JobID] = j
		if err := validatePersistedIngressOperations(st); err == nil {
			t.Fatal("tampered queued command restored")
		}
	}
}
