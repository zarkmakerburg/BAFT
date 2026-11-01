package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

func newPathProbeStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "src", Address: "127.0.0.1:22", PathIPv4: "127.0.0.1", Role: "worker"}, "tok-src"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "dst", Address: "127.0.0.1:22", PathIPv4: "127.0.0.1", PathIPv6: "2001:db8::9", Role: "foreign"}, "tok-dst"); err != nil {
		t.Fatal(err)
	}
	return s
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func startOnePathProbe(t *testing.T, s *Store, port int) PathProbe {
	t.Helper()
	p, err := s.StartPathProbe(PathProbeRequest{
		SourceNode: "src", DestinationNode: "dst", Family: "4", CandidatePorts: []int{port},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPathProbeFullDataBecomesRecommended(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	p := startOnePathProbe(t, s, 22001)

	if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "succeeded", "listener ready", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvancePathProbes(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPathProbe(p.ID)
	run := probeRunEvidence{
		ProbeID: p.ID, Family: "4", Target: "127.0.0.1:22001",
		Bulk: []probeFlowEvidence{
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 7, Connected: true, Handshake: true, ACK: true},
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 5, Connected: true, Handshake: true, ACK: true},
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 6, Connected: true, Handshake: true, ACK: true},
		},
		Trickle: probeFlowEvidence{Requested: 16384, BytesWritten: 16384, ConnectMS: 6, Connected: true, Handshake: true, ACK: true},
	}
	if err := s.AckJobOutput("src", "tok-src", p.JobID, "succeeded", "path probe evidence recorded", mustJSON(t, run)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvancePathProbes(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPathProbe(p.ID)
	listen := probeListenEvidence{
		ProbeID: p.ID, Family: "4", Port: 22001,
		Received: []int64{65536, 65536, 65536, 16384},
		Full:     []bool{true, true, true, true},
	}
	if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "succeeded", "path probe evidence recorded", mustJSON(t, listen)); err != nil {
		t.Fatal(err)
	}
	evs, err := s.AdvancePathProbes(now.Add(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPathProbe(p.ID)
	if p.Phase != PathProbeComplete || p.RecommendedPort != 22001 || p.Candidates[0].Class != PathFullData {
		t.Fatalf("unexpected completed probe: %+v", p)
	}
	if len(evs) != 1 || evs[0].Action != "pathprobe.complete" {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestPathProbeDetectsRepeatedByteCeiling(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	p := startOnePathProbe(t, s, 22002)
	if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "succeeded", "listener ready", ""); err != nil {
		t.Fatal(err)
	}
	_, _ = s.AdvancePathProbes(now.Add(time.Second))
	p, _ = s.GetPathProbe(p.ID)
	run := probeRunEvidence{
		ProbeID: p.ID, Family: "4", Target: "127.0.0.1:22002",
		Bulk: []probeFlowEvidence{
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 4, Connected: true, Handshake: true},
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 4, Connected: true, Handshake: true},
			{Requested: 65536, BytesWritten: 65536, ConnectMS: 5, Connected: true, Handshake: true},
		},
		Trickle: probeFlowEvidence{Requested: 16384, BytesWritten: 16384, ConnectMS: 4, Connected: true, Handshake: true},
	}
	if err := s.AckJobOutput("src", "tok-src", p.JobID, "succeeded", "path probe evidence recorded", mustJSON(t, run)); err != nil {
		t.Fatal(err)
	}
	_, _ = s.AdvancePathProbes(now.Add(2 * time.Second))
	p, _ = s.GetPathProbe(p.ID)
	listen := probeListenEvidence{
		ProbeID: p.ID, Family: "4", Port: 22002,
		Received: []int64{8688, 8688, 8688, 8688},
		Full:     []bool{false, false, false, false},
	}
	if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "succeeded", "path probe evidence recorded", mustJSON(t, listen)); err != nil {
		t.Fatal(err)
	}
	_, _ = s.AdvancePathProbes(now.Add(3 * time.Second))
	p, _ = s.GetPathProbe(p.ID)
	c := p.Candidates[0]
	if c.Class != PathByteCeiling || c.ByteCeiling != 8688 || p.RecommendedPort != 0 {
		t.Fatalf("ceiling not classified: %+v", p)
	}
}

func TestPathProbeOccupiedPortIsSkippedWithoutSourceJob(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	p := startOnePathProbe(t, s, 22)
	if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "failed", "path probe listen: bind: address already in use", ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.AdvancePathProbes(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPathProbe(p.ID)
	if p.Phase != PathProbeComplete || p.Candidates[0].Class != PathLocalConflict {
		t.Fatalf("occupied port not skipped: %+v", p)
	}
	for _, j := range s.ListJobs() {
		if j.NodeID == "src" {
			t.Fatalf("source probe was queued for occupied destination port: %+v", j)
		}
	}
}

func TestPathProbeUsesOnlyEnrolledNodeAddressesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "src", Address: "192.0.2.2:22", PathIPv4: "192.0.2.2", Role: "worker"}, "src-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "dst", Address: "198.51.100.9:22", PathIPv4: "198.51.100.9", PathIPv6: "2001:db8::9", Role: "foreign"}, "dst-token"); err != nil {
		t.Fatal(err)
	}
	p, err := s.StartPathProbe(PathProbeRequest{
		SourceNode: "src", DestinationNode: "dst", Family: "6", CandidatePorts: []int{443},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if p.DestinationIP != "2001:db8::9" {
		t.Fatalf("destination escaped enrolled IPv6 address: %+v", p)
	}
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.GetPathProbe(p.ID)
	if !ok || got.DestinationIP != p.DestinationIP || got.JobID == "" {
		t.Fatalf("path probe did not survive BCC restart: %+v ok=%v", got, ok)
	}
	if _, err := s2.StartPathProbe(PathProbeRequest{
		SourceNode: "src", DestinationNode: "not-enrolled", Family: "4", CandidatePorts: []int{443},
	}, time.Now().UTC()); err == nil {
		t.Fatal("probe to an unenrolled destination was accepted")
	}
}

// A probe started while four probes are already active (for example while
// automatic path discovery runs right after two nodes enroll) is queued with
// no job. It must wait for a slot, not fail as "current probe job is missing"
// (guided-flow e2e run 41257388191).
func TestQueuedPathProbeWaitsForASlotInsteadOfFailing(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	var active []PathProbe
	for i := 0; i < 4; i++ {
		active = append(active, startOnePathProbe(t, s, 22101+i))
	}
	queued := startOnePathProbe(t, s, 22199)
	if queued.Phase != PathProbeQueued || queued.JobID != "" {
		t.Fatalf("fifth probe should wait in the queue: %+v", queued)
	}
	if _, err := s.AdvancePathProbes(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetPathProbe(queued.ID); got.Phase != PathProbeQueued || got.Error != "" {
		t.Fatalf("queued probe was failed instead of waiting: %+v", got)
	}
	// Free one slot: the queued probe must then be scheduled.
	s.mu.Lock()
	done := s.st.PathProbes[active[0].ID]
	done.Phase = PathProbeComplete
	s.st.PathProbes[done.ID] = done
	s.mu.Unlock()
	if _, err := s.AdvancePathProbes(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPathProbe(queued.ID)
	if got.Phase == PathProbeFailed || got.Phase == PathProbeQueued || got.JobID == "" {
		t.Fatalf("queued probe was not scheduled once a slot freed: %+v", got)
	}
}

func TestCancellingAQueuedPathProbeStartsNothing(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		startOnePathProbe(t, s, 22301+i)
	}
	queued := startOnePathProbe(t, s, 22399)
	jobsBefore := len(s.ListJobs())
	got, err := s.CancelPathProbe(queued.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PathProbeCancelled || got.JobID != "" {
		t.Fatalf("cancelled queued probe: %+v", got)
	}
	if n := len(s.ListJobs()); n != jobsBefore {
		t.Fatalf("cancelling a queued probe queued %d agent jobs", n-jobsBefore)
	}
}

func TestQueuedPathProbeGivesUpAfterTheQueueTimeout(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		startOnePathProbe(t, s, 22201+i)
	}
	queued := startOnePathProbe(t, s, 22299)
	s.mu.Lock()
	p := s.st.PathProbes[queued.ID]
	p.CreatedAt = now.Add(-pathProbeQueueTimeout - time.Minute)
	s.st.PathProbes[queued.ID] = p
	s.mu.Unlock()
	if _, err := s.AdvancePathProbes(now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPathProbe(queued.ID)
	if got.Phase != PathProbeFailed || !strings.Contains(got.Error, "waiting behind other path probes") {
		t.Fatalf("stale queued probe must fail with a clear reason: %+v", got)
	}
}

func TestPathProbeCandidateReportsAttemptsAndConnectSpread(t *testing.T) {
	flow := func(ms int64, connected, ack bool) probeFlowEvidence {
		return probeFlowEvidence{Requested: 65536, BytesWritten: 65536, ConnectMS: ms, Connected: connected, Handshake: connected, ACK: ack}
	}
	full := probeFlowEvidence{Requested: 16384, BytesWritten: 16384, Connected: true, Handshake: true, ACK: true}
	var c PathProbeCandidate
	classifyPathProbeCandidate(&c, probeRunEvidence{Bulk: []probeFlowEvidence{flow(7, true, true), flow(5, true, true), flow(6, true, true)}, Trickle: full}, probeListenEvidence{})
	if c.Class != PathFullData || c.Attempts != 3 || c.ACKSuccesses != 3 || c.ConnectRTTMS != 6 || c.ConnectSpreadMS != 1 {
		t.Fatalf("stable full-data candidate: %+v", c)
	}
	c = PathProbeCandidate{}
	classifyPathProbeCandidate(&c, probeRunEvidence{Bulk: []probeFlowEvidence{flow(10, true, true), flow(90, true, false), flow(0, false, false)}, Trickle: full}, probeListenEvidence{})
	if c.Class == PathFullData || c.Attempts != 3 || c.ACKSuccesses != 1 || c.ConnectSpreadMS != 40 {
		t.Fatalf("lossy candidate must not be FULL_DATA and must expose spread over connected flows only: %+v", c)
	}
	c = PathProbeCandidate{}
	classifyPathProbeCandidate(&c, probeRunEvidence{Bulk: []probeFlowEvidence{flow(12, true, true)}}, probeListenEvidence{})
	if c.Attempts != 1 || c.ConnectSpreadMS != 0 {
		t.Fatalf("one sample has no spread: %+v", c)
	}
	if got := connectSpreadMS(nil); got != 0 {
		t.Fatalf("empty spread = %d", got)
	}
}

func TestPathProbeRecommendationIsDeterministic(t *testing.T) {
	cs := []PathProbeCandidate{
		{Port: 8443, Class: PathFullData, ConnectRTTMS: 18},
		{Port: 443, Class: PathFullData, ConnectRTTMS: 18},
		{Port: 22, Class: PathConnectOnly, ConnectRTTMS: 1},
	}
	if got := recommendProbePort(cs); got != 443 {
		t.Fatalf("recommendation=%d want=443", got)
	}
}

func TestPathProbeCancelQueuesDestinationCleanup(t *testing.T) {
	s := newPathProbeStore(t)
	now := time.Now().UTC()
	p := startOnePathProbe(t, s, 22003)
	cancelled, err := s.CancelPathProbe(p.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Phase != PathProbeStopping || !cancelled.CancelRequested || cancelled.JobID == "" {
		t.Fatalf("cancel did not queue cleanup: %+v", cancelled)
	}
	j := s.st.Jobs[cancelled.JobID]
	if j.Type != "path_probe_stop" || j.NodeID != "dst" {
		t.Fatalf("cleanup job is not destination stop: %+v", j)
	}
	if err := s.AckJobOutput("dst", "tok-dst", j.ID, "succeeded", "path probe evidence recorded", mustJSON(t, probeListenEvidence{ProbeID: p.ID})); err != nil {
		t.Fatal(err)
	}
	evs, err := s.AdvancePathProbes(now.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPathProbe(p.ID)
	if got.Phase != PathProbeCancelled || got.JobID != "" {
		t.Fatalf("cancel did not finish cleanly: %+v", got)
	}
	if len(evs) != 1 || evs[0].Action != "pathprobe.cancelled" {
		t.Fatalf("unexpected cancel events: %+v", evs)
	}
}

func TestPathProbeAPIRejectsArbitraryTargetAndDashboardExposesControl(t *testing.T) {
	s := newPathProbeStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	bad := httptest.NewRecorder()
	app.Handler().ServeHTTP(bad, authReq(http.MethodPost, "/api/path-probes", "admin", map[string]any{
		"source_node": "src", "destination_node": "dst", "family": "4",
		"candidate_ports": []int{443}, "target": "203.0.113.250",
	}))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("arbitrary target field accepted: %d %s", bad.Code, bad.Body.String())
	}
	ok := httptest.NewRecorder()
	app.Handler().ServeHTTP(ok, authReq(http.MethodPost, "/api/path-probes", "admin", map[string]any{
		"source_node": "src", "destination_node": "dst", "family": "4",
		"candidate_ports": []int{443, 8443},
	}))
	if ok.Code != http.StatusAccepted {
		t.Fatalf("path probe start=%d %s", ok.Code, ok.Body.String())
	}
	var p PathProbe
	if err := json.Unmarshal(ok.Body.Bytes(), &p); err != nil || p.DestinationIP != "127.0.0.1" {
		t.Fatalf("unexpected API probe: %+v err=%v", p, err)
	}
	for _, want := range []string{"/api/path-probes", "Automatic Path Discovery", "ppRun", "loadPathProbes", "/api/path-graph", "loadPathGraph"} {
		if !strings.Contains(dashboardHTML, want) {
			t.Errorf("dashboard lacks %q", want)
		}
	}
}

func TestNodePathAddressesAreValidatedAndPreserved(t *testing.T) {
	s := newPathProbeStore(t)
	n, ok := s.GetNode("dst")
	if !ok || n.PathIPv6 != "2001:db8::9" {
		t.Fatalf("missing path address: %+v", n)
	}
	if _, err := s.UpsertNode(Node{ID: "dst", Alias: "renamed", Address: n.Address, Role: "foreign"}, ""); err != nil {
		t.Fatal(err)
	}
	n, _ = s.GetNode("dst")
	if n.PathIPv4 != "127.0.0.1" || n.PathIPv6 != "2001:db8::9" {
		t.Fatalf("partial node update erased path addresses: %+v", n)
	}
	if _, err := s.UpsertNode(Node{ID: "bad", Address: "127.0.0.1:22", PathIPv6: "192.0.2.4", Role: "worker"}, "tok"); err == nil {
		t.Fatal("IPv4 accepted as path_ipv6")
	}
}

func TestPathProbeThroughputNeedsAnExactlyAcknowledgedSpeedFlow(t *testing.T) {
	ok := &probeSpeedEvidence{Seconds: 5, BytesWritten: 50_000_000, AckedBytes: 50_000_000, TransferMS: 5000, ACK: true}
	if got, floor := probeThroughputKbps(ok); got != 80_000 || floor {
		t.Fatalf("50 MB in 5 s = %d kbps (floor %v), want 80000 estimate", got, floor)
	}
	// A fast path hits the byte cap long before the time budget: report the
	// measured rate as a floor instead of nothing (two-host LAB evidence).
	capped := &probeSpeedEvidence{Seconds: 5, BytesWritten: agentjob.PathProbeSpeedMaxBytes, AckedBytes: agentjob.PathProbeSpeedMaxBytes, TransferMS: 77, ACK: true}
	if got, floor := probeThroughputKbps(capped); got != int64(agentjob.PathProbeSpeedMaxBytes)*8/77 || !floor {
		t.Fatalf("capped flow: %d kbps floor=%v", got, floor)
	}
	for name, sp := range map[string]*probeSpeedEvidence{
		"missing":            nil,
		"not acked":          {BytesWritten: 50_000_000, AckedBytes: 50_000_000, TransferMS: 5000},
		"short count":        {BytesWritten: 50_000_000, AckedBytes: 49_000_000, TransferMS: 5000, ACK: true},
		"error":              {BytesWritten: 50_000_000, AckedBytes: 50_000_000, TransferMS: 5000, ACK: true, Error: "reset"},
		"too little":         {BytesWritten: 100_000, AckedBytes: 100_000, TransferMS: 5000, ACK: true},
		"too quick":          {BytesWritten: 1_000_000, AckedBytes: 1_000_000, TransferMS: 100, ACK: true},
		"zero time":          {Seconds: 5, BytesWritten: agentjob.PathProbeSpeedMaxBytes, AckedBytes: agentjob.PathProbeSpeedMaxBytes, TransferMS: 0, ACK: true},
		"capped short count": {Seconds: 5, BytesWritten: agentjob.PathProbeSpeedMaxBytes, AckedBytes: agentjob.PathProbeSpeedMaxBytes - 1, TransferMS: 77, ACK: true},
	} {
		if got, _ := probeThroughputKbps(sp); got != 0 {
			t.Fatalf("%s: throughput %d must not be reported", name, got)
		}
	}
	var c PathProbeCandidate
	full := probeFlowEvidence{Requested: 65536, BytesWritten: 65536, ConnectMS: 5, Connected: true, Handshake: true, ACK: true}
	classifyPathProbeCandidate(&c, probeRunEvidence{Bulk: []probeFlowEvidence{full, full, full}, Trickle: full, Speed: ok}, probeListenEvidence{})
	if c.Class != PathFullData || c.ThroughputKbps != 80_000 {
		t.Fatalf("classified candidate: %+v", c)
	}
}

func TestPathProbeRequestsSpeedOnlyWhenBothAgentsAdvertiseIt(t *testing.T) {
	speed := []string{pathProbeSpeedCapability}
	for _, tc := range []struct {
		name     string
		src, dst []string
		want     bool
	}{
		{"both agents", speed, speed, true},
		{"source only", speed, nil, false},
		{"destination only", nil, speed, false},
		{"neither", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newPathProbeStore(t)
			for _, poll := range []struct {
				node, token string
				caps        []string
			}{{"src", "tok-src", tc.src}, {"dst", "tok-dst", tc.dst}} {
				if _, err := s.pullJobsReporting(poll.node, poll.token, poll.caps, func([]Job) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC()
			p := startOnePathProbe(t, s, 22001)
			if err := s.AckJobOutput("dst", "tok-dst", p.JobID, "succeeded", "listener ready", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdvancePathProbes(now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			p, _ = s.GetPathProbe(p.ID)
			var run Job
			for _, j := range s.ListJobs() {
				if j.ID == p.JobID {
					run = j
				}
			}
			if run.Type != "path_probe_run" {
				t.Fatalf("expected the run job, got %+v", run)
			}
			got, has := run.Params["speed_seconds"]
			if has != tc.want || (has && got != "5") {
				t.Fatalf("speed_seconds=%q present=%v, want present=%v", got, has, tc.want)
			}
		})
	}
}
