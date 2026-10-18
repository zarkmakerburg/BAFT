package agent

import (
 "strings"
 "testing"
 "time"
)

func TestValidateComponentReport(t *testing.T) {
 now := time.Now().UTC()
 report, err := BuildComponentReport("node", now, nil)
 if err != nil { t.Fatal(err) }
 if err := ValidateComponentReport(report, now, time.Minute); err != nil { t.Fatal(err) }
 if err := ValidateComponentReport(report, now.Add(2*time.Minute), time.Minute); err == nil { t.Fatal("stale report accepted") }
 if err := ValidateComponentReport(report, now.Add(-time.Second), time.Minute); err == nil { t.Fatal("future report accepted") }
 report.Components[0].State = "observed"
 report.Components[0].SHA256 = strings.Repeat("a", 64)
 if err := ValidateComponentReport(report, now, time.Minute); err != nil { t.Fatal(err) }
 report.Components[0].SHA256 = strings.Repeat("g", 64)
 if err := ValidateComponentReport(report, now, time.Minute); err == nil { t.Fatal("nonhex digest accepted") }
}
