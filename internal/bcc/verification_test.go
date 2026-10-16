package bcc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
	"github.com/zarkmakerburg/baft/internal/verification"
)

func dataPendingTunnel(t *testing.T, s *Store, now time.Time) Tunnel {
	t.Helper()
	tn := driveToObserve(t, s, now)
	for _, spec := range []struct{ node, role string }{{tn.IRNode, tunnelnode.RoleIR}, {tn.EXNode, tunnelnode.RoleEX}} {
		jobs := pullAll(t, s, spec.node)
		if len(jobs) != 1 || jobs[0].Type != JobTunnelObserve {
			t.Fatal("missing observation")
		}
		b, _ := json.Marshal(goodObserved(tn, spec.role))
		if err := s.AckJob(spec.node, "tok-"+spec.node, jobs[0].ID, "succeeded", string(b)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AdvanceTunnels(now); err != nil {
			t.Fatal(err)
		}
	}
	tn, _ = s.GetTunnel(tn.ID)
	if tn.Phase != TunnelDataIR {
		t.Fatalf("premature finalization: %s", tn.Phase)
	}
	return tn
}

func ackDeliveryFixture(t *testing.T, s *Store, tn Tunnel, node string, at time.Time, mutate func(*verification.Proof)) {
	t.Helper()
	jobs := pullAll(t, s, node)
	if len(jobs) != 1 {
		t.Fatalf("missing verification job: %+v", jobs)
	}
	p := verificationFixture(tn, node, at, jobs[0].Type == JobTunnelSeal)
	if mutate != nil {
		mutate(&p)
	}
	b, _ := json.Marshal(p)
	if err := s.AckJob(node, "tok-"+node, jobs[0].ID, "succeeded", string(b)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(at); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryCapabilityUnknownStaleOrUnsupportedCreatesNothing(t *testing.T) {
	now := time.Now().UTC()
	for _, kind := range []string{"unknown", "unsupported", "stale", "future"} {
		t.Run(kind, func(t *testing.T) {
			s := tunnelStore(t)
			n := s.st.Nodes["ir-1"]
			switch kind {
			case "unknown":
				n.CapabilitiesAt = time.Time{}
			case "unsupported":
				n.TunnelVerification = 0
			case "stale":
				n.CapabilitiesAt = now.Add(-10 * time.Minute)
			case "future":
				n.CapabilitiesAt = now.Add(time.Minute)
			}
			s.st.Nodes[n.ID] = n
			if _, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}, now); err == nil {
				t.Fatal("unsupported deployment admitted")
			}
			if len(s.st.Tunnels) != 0 || len(s.st.Jobs) != 0 {
				t.Fatal("capability failure created a job or tunnel")
			}
		})
	}
}

func TestDeliveryNoTrafficRetainsBackupsAndUsesBoundedRetries(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := dataPendingTunnel(t, s, now)
	ackDeliveryFixture(t, s, tn, tn.IRNode, now, func(p *verification.Proof) { p.State = verification.NotAssessed; p.Reason = "no data" })
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelDataIR || got.JobID != "" || got.FinalizationBegan {
		t.Fatalf("idle became ready: %+v", got)
	}
	for _, j := range s.st.Jobs {
		if j.Type == JobTunnelFinalize || j.Type == JobTunnelSeal {
			t.Fatal("no traffic caused backup release")
		}
	}
	before := len(s.st.Jobs)
	s.AdvanceTunnels(now.Add(time.Second))
	if len(s.st.Jobs) != before {
		t.Fatal("retry spin was unbounded")
	}
	s.AdvanceTunnels(now.Add(11 * time.Second))
	if len(s.st.Jobs) != before+1 {
		t.Fatal("bounded retry not scheduled")
	}
	s.AdvanceTunnels(now.Add(tunnelStepTimeout))
	got, _ = s.GetTunnel(tn.ID)
	if got.Phase != TunnelRollingBack {
		t.Fatalf("verification deadline extended by retries: %s", got.Phase)
	}
}

func TestDeliveryRejectsOtherInstanceMalformedCountersAndStalePayload(t *testing.T) {
	for _, kind := range []string{"instance", "generation", "artifact", "no-delivery", "stale-payload", "target", "schema"} {
		t.Run(kind, func(t *testing.T) {
			s := tunnelStore(t)
			now := time.Now().UTC()
			tn := dataPendingTunnel(t, s, now)
			ackDeliveryFixture(t, s, tn, tn.IRNode, now, func(p *verification.Proof) {
				switch kind {
				case "instance":
					p.InstanceID = "other"
				case "generation":
					p.Generation++
				case "artifact":
					p.ConfigSHA256 = "wrong"
				case "no-delivery":
					p.Runtime.DeliveredBytes = 0
				case "stale-payload":
					p.DataAgeMS = int64(verification.FreshFor / time.Millisecond)
				case "target":
					p.Runtime.DeliveredBytes = p.Runtime.ReceivedBytes + 1
				case "schema":
					p.Runtime.Version = 0
				}
			})
			got, _ := s.GetTunnel(tn.ID)
			if got.Phase != TunnelRollingBack {
				t.Fatalf("invalid proof accepted: %s", got.Phase)
			}
			for _, j := range s.st.Jobs {
				if j.Type == JobTunnelFinalize {
					t.Fatal("invalid evidence caused finalization")
				}
			}
		})
	}
}

func TestDeliveryBothNodesMustSealBeforeAnyBackupRelease(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := dataPendingTunnel(t, s, now)
	for _, node := range []string{tn.IRNode, tn.EXNode, tn.IRNode} {
		current, _ := s.GetTunnel(tn.ID)
		ackDeliveryFixture(t, s, current, node, now, nil)
	}
	for _, j := range s.st.Jobs {
		if j.Type == JobTunnelFinalize {
			t.Fatal("one-sided sealing released backups")
		}
	}
	current, _ := s.GetTunnel(tn.ID)
	ackDeliveryFixture(t, s, current, tn.EXNode, now, func(p *verification.Proof) { p.State = verification.NotAssessed; p.ReadyToFinalize = false })
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelSealingEX || got.FinalizationBegan {
		t.Fatal("missing EX readiness crossed the release boundary")
	}
}

func TestDeliveryCleanupFailureIsHeldAndCannotClaimActiveOrRollback(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := dataPendingTunnel(t, s, now)
	driveVerificationFixture(t, s, tn.ID, now)
	jobs := pullAll(t, s, tn.IRNode)
	if len(jobs) != 1 || jobs[0].Type != JobTunnelFinalize {
		t.Fatal("missing release")
	}
	s.AckJob(tn.IRNode, "tok-"+tn.IRNode, jobs[0].ID, "failed", "filesystem cleanup failed")
	s.AdvanceTunnels(now)
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelFinalizeIncomplete {
		t.Fatalf("failed cleanup was accepted: %s", got.Phase)
	}
	if _, err := s.CancelTunnel(tn.ID, "cancel", now); err == nil {
		t.Fatal("irreversible cleanup mislabeled rollbackable")
	}
	s.AdvanceTunnels(now.Add(time.Minute))
	got, _ = s.GetTunnel(tn.ID)
	if got.Phase != TunnelFinalizeIncomplete {
		t.Fatal("incomplete cleanup advanced by itself")
	}
	for id, n := range s.st.Nodes {
		n.CapabilitiesAt = now
		s.st.Nodes[id] = n
	}
	if got, err := s.RetryFinalization(tn.ID, now); err != nil || got.Phase != TunnelDataIR {
		t.Fatalf("explicit retry unavailable: %+v %v", got, err)
	}
}

func TestDeliveryDelayedReceiptCannotRefreshOldPayload(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := dataPendingTunnel(t, s, now)
	ackDeliveryFixture(t, s, tn, tn.IRNode, now, func(p *verification.Proof) {
		p.At = now.Add(-verification.FreshFor)
		p.Runtime.GeneratedAt, p.Runtime.LastDeliveredAt = p.At, p.At
	})
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelDataIR || got.JobID != "" || got.FinalizationBegan {
		t.Fatal("old payload became fresh when its acknowledgement arrived")
	}
}

func TestDeliveryMismatchedJobCannotAdvanceVerification(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := dataPendingTunnel(t, s, now)
	j := s.st.Jobs[tn.JobID]
	j.NodeID = tn.EXNode
	s.st.Jobs[j.ID] = j
	if _, err := s.AdvanceTunnels(now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelRollingBack {
		t.Fatal("another node's job advanced verification")
	}
}
