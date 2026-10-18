package bcc

import (
 "encoding/json"
 "bytes"
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
 var in struct {
  NodeID string `json:"node_id"`
  ObservedAt time.Time `json:"observed_at"`
  Components []struct {
   Component string `json:"component"`
   SHA256 string `json:"sha256"`
   State string `json:"state"`
  } `json:"components"`
 }
 dec := json.NewDecoder(bytes.NewReader(body))
 dec.DisallowUnknownFields()
 if err := dec.Decode(&in); err != nil { http.Error(w, "invalid JSON", http.StatusBadRequest); return }
 var extra any
 if err := dec.Decode(&extra); err != io.EOF { http.Error(w, "trailing JSON", http.StatusBadRequest); return }
 if len(in.Components) != 3 { http.Error(w, "invalid component inventory", http.StatusBadRequest); return }
 receipt := ComponentDigestReceipt{NodeID:in.NodeID, ObservedAt:in.ObservedAt, Digests:make(map[string]string, 3)}
 for _, c := range in.Components {
  if c.Component != "transport" && c.Component != "agent" && c.Component != "bcc" { http.Error(w, "invalid component", http.StatusBadRequest); return }
  if _, exists := receipt.Digests[c.Component]; exists { http.Error(w, "duplicate component", http.StatusBadRequest); return }
  switch c.State {
  case "observed": if !isHexSHA256(c.SHA256) { http.Error(w, "invalid digest", http.StatusBadRequest); return }; receipt.Digests[c.Component] = c.SHA256
  case "unknown": if c.SHA256 != "" { http.Error(w, "unexpected digest", http.StatusBadRequest); return }; receipt.Digests[c.Component] = "unknown"
  default: http.Error(w, "invalid component state", http.StatusBadRequest); return
  }
 }
 // AcceptedAt is always server-assigned; the client cannot choose it.
 receipt.AcceptedAt = time.Time{}
 err = s.store.AcceptComponentDigestReceipt(receipt, bearer(r), s.now(), 5*time.Minute)
 s.agentAuthResult(r, err)
 if err != nil {
  status := http.StatusBadRequest
  if errors.Is(err, ErrAgentAuthentication) { status = http.StatusUnauthorized }
  http.Error(w, "component report rejected", status)
  return
 }
 writeJSON(w, http.StatusAccepted, map[string]bool{"accepted":true})
}
