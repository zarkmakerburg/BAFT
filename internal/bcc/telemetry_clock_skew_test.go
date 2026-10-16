package bcc

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/telemetry"
)

func TestFutureTelemetryDoesNotEraseRecentHistory(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	const token = "clock-skew-test-token"
	if _, err := s.UpsertNode(Node{
		ID: "n1", Alias: "N1",
		Address: "127.0.0.1:34567", Role: "foreign",
	}, token); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	first := telemetry.Report{
		NodeID: "n1", BootID: "boot", Sequence: 1,
		IngressBytes:  100,
		TimestampUnix: now.Add(-time.Hour).Unix(),
	}
	applyTelemetryDirect(t, s, token, first)

	future := first
	future.Sequence = 2
	future.IngressBytes = 200
	future.TimestampUnix = now.Add(30 * 24 * time.Hour).Unix()
	applyTelemetryDirect(t, s, token, future)

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for _, point := range s.st.History["n1"] {
		if point.Timestamp.Unix() == first.TimestampUnix {
			found = true
		}
	}
	if !found {
		t.Fatal("future-dated telemetry erased a valid recent history point")
	}
}
