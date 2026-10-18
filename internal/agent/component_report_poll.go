package agent

import (
 "context"
 "time"
)

const componentReportInterval = 5 * time.Minute

// maybeSendComponentReport bounds disk hashing and network requests to one
// attempt per interval. Failures remain inspectable via ComponentReportStatus;
// they never prevent job polling or imply successful attestation.
func (a *Agent) maybeSendComponentReport(ctx context.Context) {
 now := time.Now().UTC()
 if a.cfg.Now != nil { now = a.cfg.Now().UTC() }
 a.mu.Lock()
 if !a.lastComponentReportAttempt.IsZero() && now.Sub(a.lastComponentReportAttempt) >= 0 && now.Sub(a.lastComponentReportAttempt) < componentReportInterval {
  a.mu.Unlock()
  return
 }
 a.lastComponentReportAttempt = now
 a.mu.Unlock()
 err := a.SendComponentReport(ctx)
 a.mu.Lock()
 if err != nil { a.lastComponentReportError = err.Error() } else { a.lastComponentReportError = "" }
 a.mu.Unlock()
}

// ComponentReportStatus returns the most recent attempt and a sanitized
// transport-level failure message, for local diagnostics (not credentials).
func (a *Agent) ComponentReportStatus() (time.Time, string) {
 a.mu.Lock()
 defer a.mu.Unlock()
 return a.lastComponentReportAttempt, a.lastComponentReportError
}
