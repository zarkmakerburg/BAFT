package bcc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Reservations are immutable desired intents. There is intentionally no
// DELETE/edit endpoint until generation-aware ownership and recovery exist.
// Operators must retain these intents during restore/migration; this endpoint
// never removes a host listener or establishes that a public port is usable.
func (s *Server) publicIngressReservationsAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		reservations, err := s.store.ListPublicIngressReservations()
		if err != nil {
			http.Error(w, "stored ingress reservations are invalid", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Reservations      []PublicIngressReservation `json:"reservations"`
			DesiredOnly       bool                       `json:"desired_only"`
			OwnershipVerified bool                       `json:"ownership_verified"`
			ReadyToApply      bool                       `json:"ready_to_apply"`
		}{Reservations: reservations, DesiredOnly: true})
	case http.MethodPost:
		s.mutationMu.Lock()
		defer s.mutationMu.Unlock()
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		spec, decodeErr := decodeIngressReservationSpec(dec)
		if decodeErr != nil {
			http.Error(w, "invalid reservation request", http.StatusBadRequest)
			return
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			http.Error(w, "reservation requires one JSON object", http.StatusBadRequest)
			return
		}
		now := s.now().UTC()
		reservation, err := s.store.ReservePublicIngress(spec, now, AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil)})
		if err != nil {
			s.auditFailure(w, r, "public_ingress.reserve", spec.TunnelID, nil, err, http.StatusBadRequest)
			return
		}
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusOK, struct {
			Reservation       PublicIngressReservation `json:"reservation"`
			DesiredOnly       bool                     `json:"desired_only"`
			OwnershipVerified bool                     `json:"ownership_verified"`
			ReadyToApply      bool                     `json:"ready_to_apply"`
		}{Reservation: reservation, DesiredOnly: true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodeIngressReservationSpec(dec *json.Decoder) (PublicIngressSpec, error) {
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return PublicIngressSpec{}, errors.New("expected reservation object")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return PublicIngressSpec{}, err
		}
		key, ok := token.(string)
		if !ok {
			return PublicIngressSpec{}, errors.New("invalid field")
		}
		if _, exists := fields[key]; exists {
			return PublicIngressSpec{}, errors.New("duplicate reservation field")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return PublicIngressSpec{}, err
		}
		fields[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return PublicIngressSpec{}, err
	}
	required := []string{"ir_node", "tunnel_id", "public_port", "local_target"}
	if len(fields) != len(required) {
		return PublicIngressSpec{}, errors.New("reservation fields incomplete or unknown")
	}
	for _, key := range required {
		raw, ok := fields[key]
		if !ok || string(raw) == "null" {
			return PublicIngressSpec{}, errors.New("reservation fields required")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return PublicIngressSpec{}, err
	}
	var spec PublicIngressSpec
	err = json.Unmarshal(raw, &spec)
	return spec, err
}
