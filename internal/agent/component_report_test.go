package agent

import (
 "testing"
 "time"
)

func TestBuildComponentReport(t *testing.T) {
 now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*3600))
 report, err := BuildComponentReport("node-1", now, nil)
 if err != nil { t.Fatal(err) }
 if report.NodeID != "node-1" || !report.ObservedAt.Equal(now) || report.ObservedAt.Location() != time.UTC { t.Fatalf("invalid report: %+v", report) }
 if len(report.Components) != 3 { t.Fatalf("expected 3 component observations, got %d", len(report.Components)) }
 for _, item := range report.Components { if item.State != "unknown" { t.Fatalf("missing binary claimed valid: %+v", item) } }
}

func TestBuildComponentReportRejectsInvalidIdentity(t *testing.T) {
 if _, err := BuildComponentReport(" ", time.Now(), nil); err == nil { t.Fatal("empty node ID accepted") }
 if _, err := BuildComponentReport("node-1", time.Time{}, nil); err == nil { t.Fatal("zero observation time accepted") }
}
