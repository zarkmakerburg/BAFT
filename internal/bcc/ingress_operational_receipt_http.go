package bcc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// Validate the exact existing shared wire tags before decoding values. The
// standard struct decoder accepts duplicate keys, case aliases and null for
// scalar fields; those must not silently change an enrolled receipt's meaning.
func validateIngressReceiptWireValue(dec *json.Decoder, typ reflect.Type) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	switch typ.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return errors.New("receipt object required")
		}
		fields := make(map[string]reflect.StructField)
		required := make(map[string]bool)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if tag == "" || tag == "-" || !field.IsExported() {
				continue
			}
			fields[tag] = field
			required[tag] = options != "omitempty"
		}
		seen := make(map[string]bool)
		for dec.More() {
			keyToken, err := dec.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok || seen[key] {
				return errors.New("duplicate or invalid receipt field")
			}
			field, known := fields[key]
			if !known {
				return errors.New("unknown receipt field")
			}
			seen[key] = true
			if err := validateIngressReceiptWireValue(dec, field.Type); err != nil {
				return err
			}
		}
		if end, err := dec.Token(); err != nil || end != json.Delim('}') {
			return errors.New("invalid receipt object")
		}
		for key, needed := range required {
			if needed && !seen[key] {
				return errors.New("missing receipt field")
			}
		}
	case reflect.Slice:
		// A nil evidence slice is the normal JSON emitted for a blocked
		// receipt with no observed firewall resources.
		if token == nil {
			return nil
		}
		if token != json.Delim('[') {
			return errors.New("receipt evidence array required")
		}
		for dec.More() {
			if err := validateIngressReceiptWireValue(dec, typ.Elem()); err != nil {
				return err
			}
		}
		if end, err := dec.Token(); err != nil || end != json.Delim(']') {
			return errors.New("invalid receipt evidence array")
		}
	default:
		if _, composite := token.(json.Delim); token == nil || composite {
			return errors.New("non-null receipt scalar required")
		}
	}
	return nil
}

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
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if err != nil {
		http.Error(w, "invalid receipt size", http.StatusBadRequest)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := validateIngressReceiptWireValue(dec, reflect.TypeFor[ingressauthority.ReceiptV1]()); err != nil {
		http.Error(w, "invalid receipt", http.StatusBadRequest)
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		http.Error(w, "trailing receipt data", http.StatusBadRequest)
		return
	}
	var receipt ingressauthority.ReceiptV1
	if err := json.Unmarshal(raw, &receipt); err != nil {
		http.Error(w, "invalid receipt values", http.StatusBadRequest)
		return
	}
	err = s.store.RecordIngressAgentReceipt(receipt, bearer(r), s.now().UTC())
	s.agentAuthResult(r, err)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, ErrAgentAuthentication) {
			status = http.StatusUnauthorized
		}
		http.Error(w, "ingress receipt refused", status)
		return
	}
	op, found := s.store.GetPublicIngressOperation(receipt.Binding.Spec.TunnelID)
	command, dispatch, storedReceipt := ingressQueuedParts(op, receipt.Action == "public_ingress_retire")
	verified := found && dispatch != nil && dispatch.Phase == "queued" && command != nil && storedReceipt != nil && storedReceipt.OwnershipVerified && storedReceipt.ValidateForCommand(*command) == nil && (storedReceipt.Phase == "applied" || storedReceipt.Phase == "retired")
	writeJSON(w, http.StatusAccepted, struct {
		Recorded              bool `json:"recorded"`
		Verified              bool `json:"verified"`
		ReadyToApply          bool `json:"ready_to_apply"`
		ReconciliationPending bool `json:"reconciliation_pending"`
	}{Recorded: true, Verified: verified, ReconciliationPending: !verified})
}
