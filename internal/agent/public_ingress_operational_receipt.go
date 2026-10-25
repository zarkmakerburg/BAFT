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

// Only negative, evidence-free receipts may use this sender. A future live
// outcome needs its own durable authority/readback integration.
func (a *Agent) postBlockedIngressReceipt(ctx context.Context, receipt ingressauthority.ReceiptV1) error {
	if a == nil || ctx == nil || a.cfg.Token == "" || receipt.SchemaVersion != 1 || receipt.Binding.NodeID != a.cfg.NodeID ||
		receipt.Binding.Validate() != nil || receipt.JobID == "" ||
		(receipt.Action != agentjob.ActionPublicIngressApply && receipt.Action != agentjob.ActionPublicIngressRetire) ||
		receipt.Phase != "blocked" || receipt.OwnershipVerified || receipt.FailureCause == "" || receipt.RollbackStatus != "not_attempted" ||
		receipt.ListenerEvidence != (ingressauthority.ListenerEvidenceV1{}) || len(receipt.FirewallEvidence) != 0 {
		return errors.New("invalid blocked operational ingress receipt")
	}
	return a.postIngressReceipt(ctx, receipt, false)
}

// postVerifiedIngressReceipt is private to the signed normal executor path.
// A structurally plausible receipt cannot send positive evidence without the
// original concrete live lease held across transport and acknowledgement.
func (a *Agent) postVerifiedIngressReceipt(ctx context.Context, receipt ingressauthority.ReceiptV1, command ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if a == nil || ctx == nil || a.cfg.Token == "" || receipt.Binding.NodeID != a.cfg.NodeID || receipt.Phase == "blocked" || receipt.ValidateForCommand(command) != nil || lease == nil {
		return ingressauthority.ErrUnverified
	}
	return lease.WithLockedIngress(ctx, command, func() error { return a.postIngressReceipt(ctx, receipt, true) })
}

func (a *Agent) postIngressReceipt(ctx context.Context, receipt ingressauthority.ReceiptV1, verified bool) error {
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
	return validateIngressReceiptAck(body, verified)
}

func validateBlockedIngressReceiptAck(raw []byte) error {
	return validateIngressReceiptAck(raw, false)
}

func validateIngressReceiptAck(raw []byte, verified bool) error {
	expected := map[string]bool{"recorded": true, "verified": verified, "ready_to_apply": false, "reconciliation_pending": !verified}
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
