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

func TestDelayedTelemetryRetainsItsEventTimeWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	const token = "delayed-telemetry-test-token"
	if _, err := s.UpsertNode(Node{ID: "n1", Address: "127.0.0.1:34567", Role: "foreign"}, token); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	reportAt := now.Add(-6 * 24 * time.Hour)
	expired := now.Add(-14 * 24 * time.Hour)
	withinEventWindow := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	s.mu.Lock()
	s.st.History["n1"] = []HistoryPoint{
		{Timestamp: expired}, {Timestamp: withinEventWindow}, {Timestamp: recent},
	}
	s.mu.Unlock()
	applyTelemetryDirect(t, s, token, telemetry.Report{
		NodeID: "n1", BootID: "boot", Sequence: 1,
		TimestampUnix: reportAt.Unix(),
	})

	// Inspect the persisted retention window, before the separate read-side
	// current-time filter. Delayed ingestion must keep its preceding history.
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.mu.Lock()
	defer reopened.mu.Unlock()
	got := map[int64]bool{}
	for _, point := range reopened.st.History["n1"] {
		got[point.Timestamp.Unix()] = true
	}
	if got[expired.Unix()] || !got[withinEventWindow.Unix()] || !got[recent.Unix()] || !got[reportAt.Unix()] || len(got) != 3 {
		t.Fatalf("delayed report lost its event-time window or retained expired history: %v", got)
	}
}
