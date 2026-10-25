package ingressauthority

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// FirewallOwnerTagV1 is inert correlation data, never evidence or authority.
// Callers validate the resource proof before using it.
func FirewallOwnerTagV1(port uint16, owner ingressmanifest.Proof) string {
	raw := owner.TunnelID + "\x00" + owner.InstanceID + "\x00" + strconv.FormatUint(owner.Generation, 10) + "\x00" + owner.ManifestSHA256 + "\x00" + owner.ConfigSHA256 + "\x00" + strconv.Itoa(int(port))
	digest := sha256.Sum256([]byte(raw))
	return "baft-ingress-" + hex.EncodeToString(digest[:16])
}

// ValidateForCommand checks correlation and evidence shape ONLY. It neither
// authenticates an Agent nor proves physical observations or a held Lease.
func (r ReceiptV1) ValidateForCommand(c CommandV1) error {
	if err := c.Validate(c.IssuedAt); err != nil {
		return err
	}
	if r.SchemaVersion != 1 || r.JobID != c.JobID || r.Action != c.Action || r.Binding != c.Binding {
		return errors.New("receipt differs from exact command")
	}
	if r.Phase == "blocked" {
		if r.OwnershipVerified || r.ListenerEvidence != (ListenerEvidenceV1{}) || len(r.FirewallEvidence) != 0 || r.RollbackStatus != "not_attempted" || strings.TrimSpace(r.FailureCause) == "" {
			return errors.New("blocked receipt contains positive or missing failure evidence")
		}
		return nil
	}
	if !r.OwnershipVerified || r.RollbackStatus != "not_needed" || r.FailureCause != "" || len(r.FirewallEvidence) != 1 {
		return errors.New("incomplete operational receipt")
	}
	f := r.FirewallEvidence[0]
	if f.Family != "inet" || f.Table != "baft" || f.Chain != "ingress" || f.ChainHandle == 0 || f.RuleHandle == 0 || f.OwnerTag != FirewallOwnerTagV1(c.Binding.Port, c.Binding.Proof) {
		return errors.New("receipt firewall differs from exact owner")
	}
	switch r.Phase {
	case "applied":
		if c.Action != "public_ingress_apply" {
			return errors.New("applied receipt requires apply command")
		}
		host, _, err := net.SplitHostPort(c.Binding.ListenEndpoint)
		if err != nil {
			return err
		}
		family := "tcp6"
		if net.ParseIP(host).To4() != nil {
			family = "tcp4"
		}
		identity := strings.TrimPrefix(r.ListenerEvidence.SocketIdentity, "inode:")
		inode, err := strconv.ParseUint(identity, 10, 64)
		if r.ListenerEvidence.Endpoint != c.Binding.ListenEndpoint || r.ListenerEvidence.Family != family || err != nil || inode == 0 || r.ListenerEvidence.SocketIdentity != "inode:"+strconv.FormatUint(inode, 10) {
			return errors.New("receipt listener differs from exact endpoint/inode")
		}
	case "retired":
		if c.Action != "public_ingress_retire" || r.ListenerEvidence != (ListenerEvidenceV1{}) {
			return errors.New("retired receipt cannot claim active listener")
		}
	default:
		return errors.New("unknown operational receipt phase")
	}
	return nil
}
