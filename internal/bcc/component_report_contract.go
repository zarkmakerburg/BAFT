package bcc

import (
 "errors"
 "time"
)

// ComponentDigestReceipt is a BCC-side contract for future authenticated
// inventory ingestion. An observed hash alone does not establish a version.
type ComponentDigestReceipt struct {
 NodeID string `json:"node_id"`
 ObservedAt time.Time `json:"observed_at"`
 AcceptedAt time.Time `json:"accepted_at"`
 Digests map[string]string `json:"digests"`
}

// CheckComponentReceiptFreshness is a pure validation gate, not an API endpoint.
// Callers must separately authenticate the node, persist replay cursors and
// verify each digest against a signed release manifest.
func CheckComponentReceiptFreshness(observedAt, lastAccepted, now time.Time, maxAge time.Duration) error {
 if observedAt.IsZero() || maxAge <= 0 { return errors.New("invalid report time or max age") }
 age := now.Sub(observedAt)
 if age < 0 || age > maxAge { return errors.New("component report outside freshness window") }
 if !lastAccepted.IsZero() && !observedAt.After(lastAccepted) { return errors.New("duplicate or out-of-order component report") }
 return nil
}
