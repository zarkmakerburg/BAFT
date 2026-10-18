package agent

import (
 "context"
 "fmt"
 "net/http"
 "path/filepath"
 "time"
)

// SendComponentReport sends local binary observations using the agent's
// enrolled bearer token and configured HTTP transport. A successful response
// means BCC stored observations, not that it verified a release signature.
func (a *Agent) SendComponentReport(ctx context.Context) error {
 if a.cfg.BinDir == "" { return fmt.Errorf("binary directory is not configured") }
 now := time.Now().UTC()
 if a.cfg.Now != nil { now = a.cfg.Now().UTC() }
 paths := map[string]string{
  "transport": filepath.Join(a.cfg.BinDir, "baft"),
  "agent": filepath.Join(a.cfg.BinDir, "baft-agent"),
  "bcc": filepath.Join(a.cfg.BinDir, "baft-bcc"),
 }
 report, err := BuildComponentReport(a.cfg.NodeID, now, paths)
 if err != nil { return err }
 resp, err := a.request(ctx, http.MethodPost, "/api/agent/component-report", report)
 if err != nil { return err }
 defer resp.Body.Close()
 if resp.StatusCode != http.StatusAccepted { return fmt.Errorf("component report: HTTP %d", resp.StatusCode) }
 return nil
}
