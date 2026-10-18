package agent

import (
 "encoding/json"
 "strings"
 "testing"
 "time"
)

func TestComponentReportJSONContract(t *testing.T) {
 now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
 report, err := BuildComponentReport("node-1", now, nil)
 if err != nil { t.Fatal(err) }
 b, err := json.Marshal(report)
 if err != nil { t.Fatal(err) }
 for _, field := range []string{`"node_id":"node-1"`, `"observed_at":"2026-10-18T12:00:00Z"`, `"component":"transport"`, `"component":"agent"`, `"component":"bcc"`, `"state":"unknown"`} {
  if !strings.Contains(string(b), field) { t.Fatalf("missing %s in %s", field, b) }
 }
 var decoded ComponentReport
 if err := json.Unmarshal(b, &decoded); err != nil { t.Fatal(err) }
 if err := ValidateComponentReport(decoded, now, time.Minute); err != nil { t.Fatal(err) }
}
