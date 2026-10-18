package bcc

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "time"
)

// agentComponentReport records observations only. A digest is never treated as
// an attested release version until a signed manifest is verified separately.
func (s *Server) agentComponentReport(w http.ResponseWriter, r *http.Request) {
 if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
 defer r.Body.Close()
 body, err := io.ReadAll(io.LimitReader(r.Body, 8193))
 if err != nil { http.Error(w, "invalid request body", http.StatusBadRequest); return }
 if len(body) > 8192 { http.Error(w, "request too large", http.StatusRequestEntityTooLarge); return }
 var in ComponentDigestReceipt
 if err := json.Unmarshal(body, &in); err != nil { http.Error(w, "invalid JSON", http.StatusBadRequest); return }
 // AcceptedAt is always server-assigned; the client cannot choose it.
 in.AcceptedAt = time.Time{}
 err = s.store.AcceptComponentDigestReceipt(in, bearer(r), s.now(), 5*time.Minute)
 s.agentAuthResult(r, err)
 if err != nil {
  status := http.StatusBadRequest
  if errors.Is(err, ErrAgentAuthentication) { status = http.StatusUnauthorized }
  http.Error(w, "component report rejected", status)
  return
 }
 writeJSON(w, http.StatusAccepted, map[string]bool{"accepted":true})
}
