package agentjob

import (
	"testing"
	"time"
)

// The trickle BCC sends must be able to finish inside the agents' probe
// connection deadlines. 16 KiB at 1000 ms (sent from 2026-10-07 until v0.5.0)
// needed ~15 s against a 10 s deadline, so FULL_DATA was impossible.
func TestPathProbeTrickleMustFitTheConnectionDeadline(t *testing.T) {
	cases := []struct {
		bytes, ms int
		fits      bool
	}{
		{PathProbeTrickleBytes, PathProbeTrickleMS, true},
		{8192, 500, true},
		{16384, 200, true},   // 15 pauses x 200 ms = 3 s
		{65536, 10, true},    // 63 x 10 ms
		{1024, 1000, true},   // one chunk, no pause
		{7168, 1000, true},   // 6 pauses x 1 s = 6 s, the bound
		{8192, 1000, false},  // 7 s
		{16384, 1000, false}, // the old production value
		{65536, 1000, false},
		{0, 10, false},
		{1024, -1, false},
	}
	for _, c := range cases {
		if got := PathProbeTrickleFits(c.bytes, c.ms); got != c.fits {
			t.Errorf("PathProbeTrickleFits(%d, %d) = %v, want %v", c.bytes, c.ms, got, c.fits)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	job := func(bytes, ms string) Job {
		return Job{SchemaVersion: SchemaVersion, JobID: "job-1", NodeID: "ir-1", Action: ActionPathProbeRun, IssuedAt: now, ExpiresAt: now.Add(time.Hour),
			Params: map[string]string{"probe_id": "pp-1", "family": "4", "target": "203.0.113.5:443", "payload_bytes": "65536", "attempts": "3", "trickle_bytes": bytes, "trickle_ms": ms}}
	}
	if err := job("8192", "500").Validate(); err != nil {
		t.Fatalf("production trickle refused: %v", err)
	}
	if err := job("16384", "1000").Validate(); err == nil {
		t.Fatal("a trickle that cannot finish before the deadline was accepted (and could be signed)")
	}
}
