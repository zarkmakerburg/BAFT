package agent

import (
 "testing"
 "time"
)

func TestAcceptComponentReportTime(t *testing.T) {
 now := time.Now().UTC()
 report, err := BuildComponentReport("node", now, nil)
 if err != nil { t.Fatal(err) }
 if err := AcceptComponentReportTime(report, now.Add(-time.Second), now, time.Minute); err != nil { t.Fatal(err) }
 if err := AcceptComponentReportTime(report, now, now, time.Minute); err == nil { t.Fatal("duplicate accepted") }
 if err := AcceptComponentReportTime(report, now.Add(time.Second), now, time.Minute); err == nil { t.Fatal("out-of-order report accepted") }
}
