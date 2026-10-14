package bcc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
	"github.com/zarkmakerburg/baft/internal/verification"
)

func doctorStore(t *testing.T, now time.Time) (*Store, Tunnel) {
	t.Helper()
	s := tunnelStore(t)
	s.st.Tunnels = map[string]Tunnel{}
	s.st.PathProbes = map[string]PathProbe{}
	s.st.Health = map[string]NodeHealthRecord{}
	tn := Tunnel{ID: "doctor-tunnel", InstanceID: "slot-a", EXNode: "ex-1", IRNode: "ir-1", PublicAddress: "203.0.113.7", Port: 8443,
		Target: "127.0.0.1:2443", RouteID: "service-main", RouteListen: "127.0.0.1:1443", PlanHash: strings.Repeat("d", 64), Phase: TunnelActive,
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-10 * time.Second), ObservedGen: map[string]int{"ex-1": 4, "ir-1": 4},
		Digests: map[string]NodeDigests{}, ExpectedGen: map[string]GenExpect{"ex-1": {From: 3, To: 4}, "ir-1": {From: 3, To: 4}}}
	for node, role := range map[string]string{"ex-1": tunnelnode.RoleEX, "ir-1": tunnelnode.RoleIR} {
		o := goodObserved(tn, role)
		o.InstanceID = tn.InstanceID
		b, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		tn.Digests[node] = NodeDigests{Config: o.ConfigSHA256, Unit: o.UnitSHA256, Marker: o.MarkerSHA256}
		tn.Evidence = append(tn.Evidence, TunnelEvidence{Step: "observe", Node: node, At: now.Add(-10 * time.Second), OK: true, Detail: string(b)},
			TunnelEvidence{Step: "health", Node: node, At: now.Add(-10 * time.Second), OK: true, Detail: "active and accepting"})
	}
	// These legacy strings cannot prove target or cryptographic handshakes.
	tn.Evidence = append(tn.Evidence, TunnelEvidence{Step: "listen", Node: tn.IRNode, At: now, OK: true}, TunnelEvidence{Step: "target", Node: tn.EXNode, At: now, OK: true})
	s.st.Tunnels[tn.ID] = tn
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return s, tn
}

func doctorState(r RouteDoctorRun, stage string) string {
	for _, e := range r.Stages {
		if e.Stage == stage {
			return e.State
		}
	}
	return ""
}

func seedDoctorRawProbe(s *Store, tn Tunnel, now time.Time, port int) {
	s.st.PathProbes["raw-probe"] = PathProbe{ID: "raw-probe", SourceNode: tn.IRNode, DestinationNode: tn.EXNode, Family: "4", Phase: PathProbeComplete,
		UpdatedAt: now, Candidates: []PathProbeCandidate{{Port: port, Class: PathFullData}}}
}

func TestDoctorMissingEvidenceIsUnknownAndDoesNotScheduleListeners(t *testing.T) {
	now := time.Now().UTC()
	s, tn := doctorStore(t, now)
	tn.Evidence, tn.Digests, tn.ObservedGen = nil, nil, nil
	s.st.Tunnels[tn.ID] = tn
	before := len(s.st.Jobs)
	r, err := s.RunRouteDoctor(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != DoctorNotAssessed {
		t.Fatalf("absent evidence became a diagnosis: %+v", r)
	}
	for _, e := range r.Stages {
		if e.State != DoctorNotAssessed {
			t.Fatalf("missing %s became %s", e.Stage, e.State)
		}
	}
	if len(s.st.Jobs) != before || len(s.st.PathProbes) != 0 {
		t.Fatal("Doctor scheduled a temporary raw listener on the active tunnel port")
	}
}

func TestDoctorRawTCPDoesNotProveTLSNoiseAuthorizationOrApplication(t *testing.T) {
	now := time.Now().UTC()
	for _, port := range []int{8443, 9443} {
		s, tn := doctorStore(t, now)
		seedDoctorRawProbe(s, tn, now, port)
		r, err := s.RunRouteDoctor(tn.ID, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, stage := range []string{"tcp_path", "tls13", "carrier_handshake", "route_authorization", "target_reachability", "application_flow", "consistency"} {
			if got := doctorState(r, stage); got != DoctorNotAssessed {
				t.Errorf("raw probe on port %d turned %s into %s", port, stage, got)
			}
		}
	}
}

func TestDoctorScopeAndFreshnessProtectInstanceResults(t *testing.T) {
	now := time.Now().UTC()
	for _, kind := range []string{"stale", "future", "other-instance", "other-generation", "other-config", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			s, tn := doctorStore(t, now)
			for i := range tn.Evidence {
				e := &tn.Evidence[i]
				if e.Step != "observe" {
					continue
				}
				switch kind {
				case "stale":
					e.At = now.Add(-10 * time.Minute)
				case "future":
					e.At = now.Add(time.Minute)
				case "unreadable":
					e.Detail = "not JSON"
				default:
					var o tunnelnode.Observed
					if err := json.Unmarshal([]byte(e.Detail), &o); err != nil {
						t.Fatal(err)
					}
					if kind == "other-instance" {
						o.InstanceID = "slot-b"
					} else if kind == "other-generation" {
						o.Generation, o.NodeGeneration = 5, 5
					} else {
						o.ConfigSHA256 = strings.Repeat("e", 64)
					}
					b, _ := json.Marshal(o)
					e.Detail = string(b)
				}
			}
			s.st.Tunnels[tn.ID] = tn
			r, err := s.RunRouteDoctor(tn.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range []string{"node_service", "local_listener"} {
				if got := doctorState(r, stage); got != DoctorNotAssessed {
					t.Errorf("%s evidence made %s=%s", kind, stage, got)
				}
			}
		})
	}
}

func TestDoctorScopedServiceIgnoresSiblingNodeAggregate(t *testing.T) {
	now := time.Now().UTC()
	s, tn := doctorStore(t, now)
	for _, id := range []string{tn.IRNode, tn.EXNode} {
		s.st.Health[id] = NodeHealthRecord{Layers: map[string]*LayerRecord{LayerL0: {State: HealthDown}}}
	}
	r, err := s.RunRouteDoctor(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if doctorState(r, "node_service") != DoctorPass || doctorState(r, "local_listener") != DoctorPass {
		t.Fatalf("aggregate sibling failure replaced fresh scoped evidence: %+v", r)
	}
}

func TestDoctorConsistencyRequiresBothFreshKnownNodes(t *testing.T) {
	now := time.Now().UTC()
	for _, kind := range []string{"missing", "empty", "partial", "unknown", "stale", "future", "in-sync", "drifted"} {
		t.Run(kind, func(t *testing.T) {
			s, tn := doctorStore(t, now)
			seedDoctorRawProbe(s, tn, now, tn.Port)
			tn.Drift = &DriftReport{State: DriftInSync, CheckedAt: now, Nodes: map[string]DriftNode{tn.IRNode: {State: DriftInSync}, tn.EXNode: {State: DriftInSync}}}
			want := DoctorNotAssessed
			switch kind {
			case "missing":
				tn.Drift = nil
			case "empty":
				tn.Drift.Nodes = nil
			case "partial":
				delete(tn.Drift.Nodes, tn.EXNode)
			case "unknown":
				tn.Drift.Nodes[tn.IRNode] = DriftNode{State: DriftUnknown}
			case "stale":
				tn.Drift.CheckedAt = now.Add(-10 * time.Minute)
			case "future":
				tn.Drift.CheckedAt = now.Add(time.Minute)
			case "in-sync":
				want = DoctorPass
			case "drifted":
				want = DoctorFail
				tn.Drift.Nodes[tn.IRNode] = DriftNode{State: DriftDrifted}
			}
			s.st.Tunnels[tn.ID] = tn
			r, err := s.RunRouteDoctor(tn.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := doctorState(r, "consistency"); got != want {
				t.Fatalf("%s consistency=%s want=%s", kind, got, want)
			}
		})
	}
}

func TestDoctorDigestBindsTunnelAndAssessmentTime(t *testing.T) {
	at := time.Unix(200, 0)
	a := DiagnoseRouteDoctor("a", at, doctorFixture(""))
	b := DiagnoseRouteDoctor("b", at, doctorFixture(""))
	c := DiagnoseRouteDoctor("a", at.Add(time.Second), doctorFixture(""))
	if a.EvidenceDigest == b.EvidenceDigest || a.EvidenceDigest == c.EvidenceDigest {
		t.Fatal("digest can be transplanted between tunnel identities or assessment times")
	}
}

func TestDoctorConflictingDuplicateStageIsNotPass(t *testing.T) {
	e := doctorFixture("")
	e = append(e, DoctorStageEvidence{Stage: "node_service", State: DoctorFail}, DoctorStageEvidence{Stage: "node_service", State: DoctorPass})
	r := DiagnoseRouteDoctor("a", time.Unix(200, 0), e)
	if doctorState(r, "node_service") != DoctorNotAssessed || r.Verdict != DoctorNotAssessed {
		t.Fatalf("conflicting evidence silently overwritten: %+v", r)
	}
}

func TestDoctorUsesBothScopedDeliveryProofsAndTheirActualDataAge(t *testing.T) {
	now := time.Now().UTC()
	s, tn := doctorStore(t, now)
	for _, node := range []string{tn.IRNode, tn.EXNode} {
		p := verificationFixture(tn, node, now, false)
		b, _ := json.Marshal(p)
		tn.Evidence = append(tn.Evidence, TunnelEvidence{Step: "data", Node: node, At: now, OK: true, Detail: string(b)})
	}
	s.st.Tunnels[tn.ID] = tn
	r, err := s.RunRouteDoctor(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"tcp_path", "tls13", "carrier_handshake", "route_authorization", "target_reachability", "application_flow"} {
		if doctorState(r, stage) != DoctorPass {
			t.Fatalf("actual delivery did not prove %s", stage)
		}
	}
	// Refreshing an assessment cannot renew the time of the actual DATA event.
	for i := range tn.Evidence {
		if tn.Evidence[i].Step != "data" {
			continue
		}
		var p verification.Proof
		json.Unmarshal([]byte(tn.Evidence[i].Detail), &p)
		p.Runtime.LastDeliveredAt = p.At.Add(-2 * time.Minute)
		p.DataAgeMS = int64(2 * time.Minute / time.Millisecond)
		b, _ := json.Marshal(p)
		tn.Evidence[i].Detail = string(b)
	}
	s.st.Tunnels[tn.ID] = tn
	r, err = s.RunRouteDoctor(tn.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if doctorState(r, "application_flow") != DoctorNotAssessed {
		t.Fatal("old DATA became fresh merely by rerunning Doctor")
	}
}
