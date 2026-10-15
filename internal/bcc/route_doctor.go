package bcc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
	"github.com/zarkmakerburg/baft/internal/verification"
)

const (
	DoctorPass        = "PASS"
	DoctorFail        = "FAIL"
	DoctorNotAssessed = "NOT_ASSESSED"
	doctorFreshFor    = 3 * time.Minute
)

type DoctorStageEvidence struct {
	Stage      string    `json:"stage"`
	State      string    `json:"state"`
	Detail     string    `json:"detail,omitempty"`
	Reference  string    `json:"reference,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

type RouteDoctorRun struct {
	SchemaVersion     int                   `json:"schema_version"`
	InstanceID        string                `json:"instance_id,omitempty"`
	Generations       map[string]int        `json:"generations,omitempty"`
	FreshForSeconds   int64                 `json:"fresh_for_seconds"`
	ID                string                `json:"id"`
	TunnelID          string                `json:"tunnel_id"`
	CreatedAt         time.Time             `json:"created_at"`
	Verdict           string                `json:"verdict"`
	FirstFailingStage string                `json:"first_failing_stage,omitempty"`
	SummaryEN         string                `json:"summary_en"`
	SummaryFA         string                `json:"summary_fa"`
	EvidenceDigest    string                `json:"evidence_digest"`
	Stages            []DoctorStageEvidence `json:"stages"`
}

var doctorStages = []string{
	"node_service", "local_listener", "tcp_path", "tls13", "carrier_handshake",
	"route_authorization", "target_reachability", "application_flow", "consistency",
}

func DiagnoseRouteDoctor(tunnelID string, at time.Time, evidence []DoctorStageEvidence) RouteDoctorRun {
	by := map[string]DoctorStageEvidence{}
	for _, e := range evidence {
		switch e.State {
		case DoctorPass, DoctorFail, DoctorNotAssessed:
		default:
			e.State = DoctorNotAssessed
		}
		if old, ok := by[e.Stage]; ok && old != e {
			e = DoctorStageEvidence{Stage: e.Stage, State: DoctorNotAssessed, Detail: "conflicting stage evidence"}
		}
		by[e.Stage] = e
	}
	out := RouteDoctorRun{SchemaVersion: 2, FreshForSeconds: int64(doctorFreshFor / time.Second), TunnelID: tunnelID, CreatedAt: at.UTC(), Verdict: DoctorPass}
	blocked := false
	for _, name := range doctorStages {
		e, ok := by[name]
		if blocked || !ok {
			e = DoctorStageEvidence{Stage: name, State: DoctorNotAssessed}
		}
		if e.State == "" {
			e.State = DoctorNotAssessed
		}
		out.Stages = append(out.Stages, e)
		if !blocked && e.State == DoctorFail {
			blocked = true
			out.Verdict = DoctorFail
			out.FirstFailingStage = name
		}
	}
	if out.Verdict == DoctorPass {
		for _, e := range out.Stages {
			if e.State != DoctorPass {
				out.Verdict = DoctorNotAssessed
				break
			}
		}
	}
	switch out.Verdict {
	case DoctorPass:
		out.SummaryEN, out.SummaryFA = "All assessed Route Doctor stages passed.", "همه مراحل ارزیابی‌شده Route Doctor با موفقیت عبور کردند."
	case DoctorFail:
		out.SummaryEN = "First measured failing stage: " + out.FirstFailingStage + ". Unassessed stages remain unknown."
		out.SummaryFA = "اولین مرحله با شکست مشاهده‌شده: " + out.FirstFailingStage + ". مراحل ارزیابی‌نشده نامشخص می‌مانند."
	default:
		out.SummaryEN, out.SummaryFA = "Diagnosis is incomplete because required evidence is unavailable.", "تشخیص کامل نیست چون شواهد لازم موجود نیست."
	}
	out.updateDigest()
	return out
}

func (r *RouteDoctorRun) updateDigest() {
	canonical, _ := json.Marshal(struct {
		Version     int                   `json:"version"`
		TunnelID    string                `json:"tunnel_id"`
		InstanceID  string                `json:"instance_id"`
		Generations map[string]int        `json:"generations"`
		At          time.Time             `json:"at"`
		FreshFor    int64                 `json:"fresh_for_seconds"`
		Stages      []DoctorStageEvidence `json:"stages"`
	}{r.SchemaVersion, r.TunnelID, r.InstanceID, r.Generations, r.CreatedAt, r.FreshForSeconds, r.Stages})
	sum := sha256.Sum256(canonical)
	r.EvidenceDigest = hex.EncodeToString(sum[:])
}

func newDoctorID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "doctor-" + time.Now().UTC().Format("20060102150405")
	}
	return "doctor-" + hex.EncodeToString(b)
}

// RunRouteDoctor assesses stored evidence under one store lock. It never starts
// a temporary raw-TCP listener on a live BAFT port: those probe handshakes do
// not prove TLS, Noise, route authorization or application traffic.
func (s *Store) RunRouteDoctor(tunnelID string, now time.Time) (RouteDoctorRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.Tunnels[strings.TrimSpace(tunnelID)]
	if !ok {
		return RouteDoctorRun{}, errors.New("tunnel not found")
	}
	if t.Phase != TunnelActive {
		return RouteDoctorRun{}, errors.New("Route Doctor requires an active tunnel")
	}
	service, listener := DoctorPass, DoctorPass
	var observedAt time.Time
	for _, node := range []string{t.IRNode, t.EXNode} {
		o, at := doctorObservation(t, node, now)
		nodeState, listenState := DoctorNotAssessed, DoctorNotAssessed
		if o != nil {
			nodeState = boolDoctor(o.ServiceActive)
			if h := doctorLatestEvidence(t, node, "health"); h != nil && doctorFresh(t, h.At, now) && h.OK && o.ServiceActive {
				listenState = DoctorPass
			}
			if observedAt.IsZero() || at.Before(observedAt) {
				observedAt = at
			}
		}
		service = doctorCombine(service, nodeState)
		listener = doctorCombine(listener, listenState)
	}
	ref := "tunnel:" + t.ID + ":instance:" + t.InstanceID
	evidence := []DoctorStageEvidence{
		{Stage: "node_service", State: service, Reference: ref, ObservedAt: observedAt, Detail: "fresh instance, generation and artifact-scoped service observation"},
		{Stage: "local_listener", State: listener, Reference: ref, ObservedAt: observedAt, Detail: "fresh scoped health check; TCP acceptance only"},
	}
	// A plan hash, node aggregate, or raw BAFTP1 probe is not runtime proof.
	for _, stage := range []string{"tcp_path", "tls13", "carrier_handshake", "route_authorization", "target_reachability", "application_flow"} {
		evidence = append(evidence, DoctorStageEvidence{Stage: stage, State: DoctorNotAssessed, Reference: ref, Detail: "fresh tunnel-scoped runtime proof is unavailable"})
	}
	dataIR, atIR := doctorDeliveryProof(t, t.IRNode, now)
	dataEX, atEX := doctorDeliveryProof(t, t.EXNode, now)
	if dataIR != nil && dataEX != nil {
		at := atIR
		if atEX.Before(at) {
			at = atEX
		}
		for i := range evidence {
			switch evidence[i].Stage {
			case "tcp_path", "tls13", "carrier_handshake", "route_authorization", "target_reachability", "application_flow":
				evidence[i].State, evidence[i].ObservedAt = DoctorPass, at
				evidence[i].Detail = "both exact runtimes delivered fresh authenticated DATA on the only managed route; application semantics are not assessed"
			}
		}
	}
	consistency := doctorConsistency(t, now)
	var driftAt time.Time
	if t.Drift != nil {
		driftAt = t.Drift.CheckedAt
	}
	evidence = append(evidence, DoctorStageEvidence{Stage: "consistency", State: consistency, Reference: ref + ":drift", ObservedAt: driftAt, Detail: "requires a fresh report for both expected nodes"})
	run := DiagnoseRouteDoctor(t.ID, now, evidence)
	run.ID, run.InstanceID = newDoctorID(), t.InstanceID
	run.Generations = map[string]int{}
	for node, generation := range t.ObservedGen {
		run.Generations[node] = generation
	}
	run.updateDigest()
	previous := s.st.RouteDoctorRuns
	next := make(map[string]RouteDoctorRun, len(previous)+1)
	for id, r := range previous {
		next[id] = r
	}
	next[run.ID] = cloneDoctorRun(run)
	s.st.RouteDoctorRuns = next
	if err := s.saveLocked(); err != nil {
		s.st.RouteDoctorRuns = previous
		return RouteDoctorRun{}, err
	}
	return run, nil
}

func doctorFresh(t Tunnel, at, now time.Time) bool {
	return !at.IsZero() && !at.After(now) && now.Sub(at) < doctorFreshFor && !at.Before(t.CreatedAt)
}

func doctorLatestEvidence(t Tunnel, node, step string) *TunnelEvidence {
	var last *TunnelEvidence
	for i := range t.Evidence {
		e := &t.Evidence[i]
		if e.Node == node && e.Step == step && (last == nil || !e.At.Before(last.At)) {
			last = e
		}
	}
	return last
}

func doctorObservation(t Tunnel, node string, now time.Time) (*tunnelnode.Observed, time.Time) {
	e := doctorLatestEvidence(t, node, "observe")
	if e == nil || !doctorFresh(t, e.At, now) {
		return nil, time.Time{}
	}
	var o tunnelnode.Observed
	if json.Unmarshal([]byte(e.Detail), &o) != nil {
		return nil, time.Time{}
	}
	role := tunnelnode.RoleEX
	if node == t.IRNode {
		role = tunnelnode.RoleIR
	}
	want, ok := t.Digests[node]
	if !ok || t.ObservedGen[node] <= 0 || o.Generation != t.ObservedGen[node] ||
		o.ConfigSHA256 != want.Config || o.UnitSHA256 != want.Unit || o.MarkerSHA256 != want.Marker {
		return nil, time.Time{}
	}
	if o.Phase != tunnelnode.PhaseCommitted && o.Phase != tunnelnode.PhaseFinalized {
		return nil, time.Time{}
	}
	checked := o
	checked.Phase, checked.ServiceActive = tunnelnode.PhaseCommitted, true
	if len(verifyObserved(t, node, role, checked)) > 0 {
		return nil, time.Time{}
	}
	return &o, e.At
}

func doctorCombine(a, b string) string {
	if a == DoctorFail || b == DoctorFail {
		return DoctorFail
	}
	if a != DoctorPass || b != DoctorPass {
		return DoctorNotAssessed
	}
	return DoctorPass
}

func doctorConsistency(t Tunnel, now time.Time) string {
	d := t.Drift
	if d == nil || !doctorFresh(t, d.CheckedAt, now) || len(d.Nodes) != 2 {
		return DoctorNotAssessed
	}
	state := DoctorPass
	for _, node := range []string{t.IRNode, t.EXNode} {
		n, ok := d.Nodes[node]
		if !ok {
			return DoctorNotAssessed
		}
		switch n.State {
		case DriftInSync:
		case DriftMissing, DriftDrifted, DriftUnmanaged:
			state = DoctorFail
		default:
			state = doctorCombine(state, DoctorNotAssessed)
		}
	}
	if state == DoctorPass && d.State != DriftInSync {
		return DoctorNotAssessed
	}
	return state
}

func doctorDeliveryProof(t Tunnel, node string, now time.Time) (*verification.Proof, time.Time) {
	e := doctorLatestEvidence(t, node, "data")
	seal := doctorLatestEvidence(t, node, "seal")
	if seal != nil && (e == nil || !seal.At.Before(e.At)) {
		e = seal
	}
	if e == nil || !doctorFresh(t, e.At, now) || !e.OK {
		return nil, time.Time{}
	}
	var p verification.Proof
	if json.Unmarshal([]byte(e.Detail), &p) != nil || p.State != verification.Pass || verifyDeliveryProof(t, node, p) != nil {
		return nil, time.Time{}
	}
	if p.At.After(now) || now.Sub(p.At)+time.Duration(p.DataAgeMS)*time.Millisecond >= verification.FreshFor {
		return nil, time.Time{}
	}
	return &p, p.Runtime.LastDeliveredAt
}

func cloneDoctorRun(r RouteDoctorRun) RouteDoctorRun {
	r.Stages = append([]DoctorStageEvidence(nil), r.Stages...)
	if r.Generations != nil {
		generations := make(map[string]int, len(r.Generations))
		for node, generation := range r.Generations {
			generations[node] = generation
		}
		r.Generations = generations
	}
	return r
}

func boolDoctor(ok bool) string {
	if ok {
		return DoctorPass
	}
	return DoctorFail
}

func (s *Store) ListRouteDoctorRuns(tunnelID string) []RouteDoctorRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []RouteDoctorRun{}
	for _, r := range s.st.RouteDoctorRuns {
		if tunnelID == "" || r.TunnelID == tunnelID {
			out = append(out, cloneDoctorRun(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Server) routeDoctorAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListRouteDoctorRuns(strings.TrimSpace(r.URL.Query().Get("tunnel_id"))))
	case http.MethodPost:
		var in struct {
			TunnelID string `json:"tunnel_id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		run, err := s.store.RunRouteDoctor(in.TunnelID, s.now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = s.audit.Append(AuditEntry{Timestamp: s.now().UTC(), Actor: "admin", RemoteIP: s.clientIP(r), Action: "route.doctor.run", Outcome: run.Verdict, Details: withRequest(r, map[string]any{"run_id": run.ID, "tunnel_id": run.TunnelID, "evidence_digest": run.EvidenceDigest})})
		writeJSON(w, http.StatusCreated, run)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
