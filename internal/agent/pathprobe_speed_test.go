package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

func startSpeedListener(t *testing.T, a *Agent, id string) string {
	t.Helper()
	port := freeProbePort(t)
	if _, err := a.pathProbeListen(agentjob.Job{Action: agentjob.ActionPathProbeListen, Params: map[string]string{
		"probe_id": id, "family": "4", "port": strconv.Itoa(port), "ttl_seconds": "20",
	}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.pathProbeStop(agentjob.Job{Params: map[string]string{"probe_id": id}}) })
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func TestPathProbeSpeedFlowIsTimeBoundedAndExactlyAcknowledged(t *testing.T) {
	a := &Agent{probeListeners: map[string]*pathProbeListener{}}
	id := "probe-speed-1"
	target := startSpeedListener(t, a, id)
	start := time.Now()
	runJSON, err := a.pathProbeRun(context.Background(), agentjob.Job{Action: agentjob.ActionPathProbeRun, Params: map[string]string{
		"probe_id": id, "family": "4", "target": target,
		"payload_bytes": "4096", "attempts": "1", "trickle_bytes": "2048", "trickle_ms": "10", "speed_seconds": "1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var run pathProbeRunEvidence
	if err := json.Unmarshal([]byte(runJSON), &run); err != nil {
		t.Fatal(err)
	}
	sp := run.Speed
	if sp == nil || !sp.ACK || sp.Error != "" || sp.Seconds != 1 || sp.BytesWritten <= 0 || sp.AckedBytes != sp.BytesWritten {
		t.Fatalf("speed flow not exactly acknowledged: %+v", sp)
	}
	if sp.BytesWritten > maxPathProbeSpeedBytes {
		t.Fatalf("speed flow exceeded the byte cap: %d", sp.BytesWritten)
	}
	if sp.TransferMS <= 0 || time.Since(start) > 15*time.Second {
		t.Fatalf("speed flow not time bounded: %+v after %s", sp, time.Since(start))
	}
	stopJSON := a.pathProbeStop(agentjob.Job{Params: map[string]string{"probe_id": id}})
	var listen pathProbeListenEvidence
	if err := json.Unmarshal([]byte(stopJSON), &listen); err != nil {
		t.Fatal(err)
	}
	if len(listen.Received) != 2 {
		t.Fatalf("speed flow must not be recorded as integrity evidence: %+v", listen.Received)
	}
}

func TestPathProbeWithoutSpeedParamSendsNoSpeedFlow(t *testing.T) {
	a := &Agent{probeListeners: map[string]*pathProbeListener{}}
	id := "probe-speed-2"
	target := startSpeedListener(t, a, id)
	runJSON, err := a.pathProbeRun(context.Background(), agentjob.Job{Action: agentjob.ActionPathProbeRun, Params: map[string]string{
		"probe_id": id, "family": "4", "target": target,
		"payload_bytes": "4096", "attempts": "1", "trickle_bytes": "2048", "trickle_ms": "10",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(runJSON, `"speed"`) {
		t.Fatalf("legacy run produced speed evidence: %s", runJSON)
	}
	for _, bad := range []string{"0", "9", "x"} {
		if _, err := a.pathProbeRun(context.Background(), agentjob.Job{Action: agentjob.ActionPathProbeRun, Params: map[string]string{
			"probe_id": id, "family": "4", "target": target,
			"payload_bytes": "4096", "attempts": "1", "trickle_bytes": "2048", "trickle_ms": "10", "speed_seconds": bad,
		}}); err == nil {
			t.Fatalf("speed_seconds=%q accepted", bad)
		}
	}
}

func TestPathProbeSpeedListenerRefusesOversizedOrForeignFlows(t *testing.T) {
	a := &Agent{probeListeners: map[string]*pathProbeListener{}}
	id := "probe-speed-3"
	target := startSpeedListener(t, a, id)
	for name, header := range map[string]string{
		"over cap":   fmt.Sprintf("BAFTS1 %s %d\n", id, maxPathProbeSpeedBytes+1),
		"zero":       fmt.Sprintf("BAFTS1 %s 0\n", id),
		"other id":   "BAFTS1 someone-else 1024\n",
		"not number": fmt.Sprintf("BAFTS1 %s lots\n", id),
	} {
		t.Run(name, func(t *testing.T) {
			c, err := net.DialTimeout("tcp4", target, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err := c.Write([]byte(header)); err != nil {
				t.Fatal(err)
			}
			if line, err := readProbeLine(c, 32); err == nil && line == "READY" {
				t.Fatalf("listener accepted %q", strings.TrimSpace(header))
			}
		})
	}
	// A sender that writes more than it announced is never acknowledged.
	c, err := net.DialTimeout("tcp4", target, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := fmt.Fprintf(c, "BAFTS1 %s 1024\n", id); err != nil {
		t.Fatal(err)
	}
	if line, err := readProbeLine(c, 32); err != nil || line != "READY" {
		t.Fatalf("ready: %q %v", line, err)
	}
	_, _ = c.Write(make([]byte, 2048))
	_ = c.(*net.TCPConn).CloseWrite()
	if line, err := readProbeLine(c, 32); err == nil && strings.HasPrefix(line, "OK") {
		t.Fatalf("over-sent flow was acknowledged: %q", line)
	}
}
