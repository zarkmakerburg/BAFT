package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

const maxIngressReceiptBytes = 16 << 10

// An authenticated v1 job still has no live authority. Report that real
// refusal to BCC, then fail the normal job ACK regardless of POST outcome.
// This path cannot call the LAB executor or manufacture host evidence.
func (a *Agent) publicIngressOperationalRefusal(ctx context.Context, j agentjob.Job) error {
	if ctx == nil || a == nil || a.cfg.NodeID != j.NodeID {
		return ingressauthority.ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	command, err := ingressV1Command(j)
	if err != nil {
		return err
	}
	now := time.Now()
	if a.cfg.Now != nil {
		now = a.cfg.Now()
	}
	if err := command.Validate(now); err != nil {
		return err
	}
	receipt := ingressauthority.ReceiptV1{
		SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding,
		Phase: "blocked", FailureCause: "durable operational ingress authority is unavailable", RollbackStatus: "not_attempted",
	}
	return errors.Join(ingressauthority.ErrUnverified, a.postBlockedIngressReceipt(ctx, receipt))
}

// postBlockedIngressReceipt sends only negative, evidence-free pre-effect receipts.
func (a *Agent) postBlockedIngressReceipt(ctx context.Context, receipt ingressauthority.ReceiptV1) error {
	if receipt.Phase != "blocked" || receipt.FailureCause == "" || receipt.RollbackStatus != "not_attempted" {
		return errors.New("invalid blocked operational ingress receipt")
	}
	return a.postIngressReceipt(ctx, receipt)
}

// validOutboundIngressReceipt admits exactly four shapes: blocked (no effects
// attempted), applied (owned listener + exact firewall evidence), retired
// (no ownership claim) and failed (effects possible; reconciliation required).
func validOutboundIngressReceipt(r ingressauthority.ReceiptV1) bool {
	if r.SchemaVersion != 1 || r.JobID == "" || r.Binding.Validate() != nil ||
		(r.Action != agentjob.ActionPublicIngressApply && r.Action != agentjob.ActionPublicIngressRetire) {
		return false
	}
	noEvidence := r.ListenerEvidence == (ingressauthority.ListenerEvidenceV1{}) && len(r.FirewallEvidence) == 0
	switch r.Phase {
	case "blocked":
		return !r.OwnershipVerified && r.FailureCause != "" && r.RollbackStatus == "not_attempted" && noEvidence
	case "failed":
		return !r.OwnershipVerified && r.FailureCause != "" && r.RollbackStatus == "reconciliation_required" && noEvidence
	case "applied":
		return r.Action == agentjob.ActionPublicIngressApply && r.OwnershipVerified && r.FailureCause == "" && r.RollbackStatus == "" &&
			r.ListenerEvidence.Endpoint == r.Binding.ListenEndpoint && r.ListenerEvidence.SocketIdentity != "" &&
			len(r.FirewallEvidence) == 1 && r.FirewallEvidence[0].RuleHandle != 0
	case "retired":
		return r.Action == agentjob.ActionPublicIngressRetire && !r.OwnershipVerified && r.FailureCause == "" && r.RollbackStatus == "" && noEvidence
	}
	return false
}

// postIngressReceipt sends one validated receipt to BCC over certificate
// validated HTTPS with the enrolled bearer token. The receipt is inert
// evidence; BCC decides what it means.
func (a *Agent) postIngressReceipt(ctx context.Context, receipt ingressauthority.ReceiptV1) error {
	if a == nil || ctx == nil || a.cfg.Token == "" || receipt.Binding.NodeID != a.cfg.NodeID || !validOutboundIngressReceipt(receipt) {
		return errors.New("invalid operational ingress receipt")
	}
	endpoint, err := url.Parse(a.cfg.BCCURL)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return errors.New("operational ingress receipt requires a direct HTTPS BCC origin")
	}
	var base http.RoundTripper = http.DefaultTransport
	if a.cfg.HTTP != nil && a.cfg.HTTP.Transport != nil {
		base = a.cfg.HTTP.Transport
	}
	transport, ok := base.(*http.Transport)
	if !ok || transport == nil || transport.DialTLS != nil || transport.DialTLSContext != nil ||
		(transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify) {
		return errors.New("operational ingress receipt requires certificate-validated HTTPS transport")
	}
	privateTransport := transport.Clone()
	defer privateTransport.CloseIdleConnections()
	client := &http.Client{Transport: privateTransport, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	raw, err := json.Marshal(receipt)
	if err != nil || len(raw) > maxIngressReceiptBytes {
		return errors.New("operational ingress receipt exceeds bound")
	}
	endpoint.Path = "/api/ingress/operational-receipt"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post blocked ingress receipt: %w", err)
	}
	defer resp.Body.Close()
	if resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 || resp.StatusCode != http.StatusAccepted {
		return errors.New("BCC refused blocked ingress receipt over verified HTTPS")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIngressReceiptBytes+1))
	if err != nil || len(body) > maxIngressReceiptBytes {
		return errors.New("invalid BCC ingress receipt acknowledgement size")
	}
	return validateBlockedIngressReceiptAck(body)
}

func validateBlockedIngressReceiptAck(raw []byte) error {
	expected := map[string]bool{"recorded": true, "verified": false, "ready_to_apply": false, "reconciliation_pending": true}
	seen := make(map[string]bool)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return errors.New("invalid BCC ingress receipt acknowledgement")
	}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return errors.New("ambiguous BCC ingress receipt acknowledgement")
		}
		want, known := expected[key]
		if !known {
			return errors.New("unknown BCC ingress receipt acknowledgement field")
		}
		token, err = decoder.Token()
		value, ok := token.(bool)
		if err != nil || !ok || value != want {
			return errors.New("BCC ingress receipt acknowledgement must remain unverified and pending")
		}
		seen[key] = true
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(expected) {
		return errors.New("incomplete BCC ingress receipt acknowledgement")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing BCC ingress receipt acknowledgement")
	}
	return nil
}
