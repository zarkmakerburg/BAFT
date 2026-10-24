package bcc

import (
	"encoding/json"
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"io"
	"net/http"
)

// The Agent must POST over a real HTTPS connection to BCC using its enrolled
// credential. Proxy headers are NOT accepted as TLS proof.
func (s *Server) operationalIngressReceiptAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.TLS == nil {
		http.Error(w, "HTTPS required", http.StatusUpgradeRequired)
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	var receipt ingressauthority.ReceiptV1
	if err := dec.Decode(&receipt); err != nil {
		http.Error(w, "invalid receipt", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		http.Error(w, "trailing receipt data", http.StatusBadRequest)
		return
	}
	err := s.store.RecordIngressAgentReceipt(receipt, bearer(r), s.now().UTC())
	s.agentAuthResult(r, err)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, ErrAgentAuthentication) {
			status = http.StatusUnauthorized
		}
		http.Error(w, "ingress receipt refused", status)
		return
	}
	writeJSON(w, http.StatusAccepted, struct {
		Recorded              bool `json:"recorded"`
		Verified              bool `json:"verified"`
		ReadyToApply          bool `json:"ready_to_apply"`
		ReconciliationPending bool `json:"reconciliation_pending"`
	}{Recorded: true, ReconciliationPending: true})
}
