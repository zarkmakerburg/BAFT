package agentjob

import (
	"testing"
	"time"
)

func TestPathProbeRunSpeedSecondsIsOptionalAndBounded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	base := func(extra map[string]string) Job {
		p := map[string]string{"probe_id": "pp-1", "family": "4", "target": "203.0.113.5:443", "payload_bytes": "65536", "attempts": "3", "trickle_bytes": "8192", "trickle_ms": "500"}
		for k, v := range extra {
			p[k] = v
		}
		return Job{SchemaVersion: SchemaVersion, JobID: "job-1", NodeID: "ir-1", Action: ActionPathProbeRun, Params: p, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	}
	if err := base(nil).Validate(); err != nil {
		t.Fatalf("legacy run without speed_seconds refused: %v", err)
	}
	for _, ok := range []string{"1", "5", "8"} {
		if err := base(map[string]string{"speed_seconds": ok}).Validate(); err != nil {
			t.Fatalf("speed_seconds=%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"0", "9", "10", "", "5s", "-1"} {
		if err := base(map[string]string{"speed_seconds": bad}).Validate(); err == nil {
			t.Fatalf("speed_seconds=%q accepted", bad)
		}
	}
	listen := Job{SchemaVersion: SchemaVersion, JobID: "job-2", NodeID: "ex-1", Action: ActionPathProbeListen, IssuedAt: now, ExpiresAt: now.Add(time.Hour),
		Params: map[string]string{"probe_id": "pp-1", "family": "4", "port": "443", "ttl_seconds": "45", "speed_seconds": "5"}}
	if err := listen.Validate(); err == nil {
		t.Fatal("speed_seconds must be valid only on path_probe_run")
	}
}
