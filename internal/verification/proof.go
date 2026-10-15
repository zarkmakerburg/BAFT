// Package verification defines bounded, payload-free proof of delivery through
// one managed BAFT route. Counters alone never identify an arbitrary instance.
package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

const (
	Version     = 1
	Capability  = "tunnel-verify-v1"
	Pass        = "PASS"
	Fail        = "FAIL"
	NotAssessed = "NOT_ASSESSED"
	FreshFor    = 3 * time.Minute
)

type RuntimeReport struct {
	Version            int       `json:"version"`
	ConfigDigest       string    `json:"config_digest"`
	BootID             string    `json:"boot_id"`
	Supported          bool      `json:"supported"`
	GeneratedAt        time.Time `json:"generated_at"`
	AuthenticatedPeers int       `json:"authenticated_peers"`
	ReceivedBytes      uint64    `json:"received_bytes"`
	DeliveredBytes     uint64    `json:"delivered_bytes"`
	LastDeliveredAt    time.Time `json:"last_delivered_at"`
}

type Proof struct {
	Version         int           `json:"version"`
	TunnelID        string        `json:"tunnel_id"`
	InstanceID      string        `json:"instance_id,omitempty"`
	Role            string        `json:"role"`
	Generation      int           `json:"generation"`
	ConfigSHA256    string        `json:"config_sha256"`
	UnitSHA256      string        `json:"unit_sha256"`
	MarkerSHA256    string        `json:"marker_sha256"`
	State           string        `json:"state"`
	Reason          string        `json:"reason,omitempty"`
	ReadyToFinalize bool          `json:"ready_to_finalize"`
	DataAgeMS       int64         `json:"data_age_ms"`
	TargetReachable bool          `json:"target_reachable"`
	At              time.Time     `json:"at"`
	Runtime         RuntimeReport `json:"runtime"`
}

func ConfigDigest(c config.Config) string {
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Runtime counters are attributable to the selected peer and route only in
// this supported shape. Multi-route/peer evidence needs a separate contract.
func Supported(c config.Config) bool {
	if c.Noise == nil || c.TLS.MinVersion != "1.3" || len(c.Routes) != 1 || c.Transport.Fallback != "" || (c.Transport.Primary != "h2" && c.Transport.Primary != "ws") {
		return false
	}
	r := c.Routes[0]
	if c.Node.Role == "dialer" {
		return c.Peer != nil && c.Peer.AllowedIdentity != "" && r.Direction == "outbound" && r.RemoteRoute == r.ID
	}
	return c.Node.Role == "listener" && c.Server != nil && len(c.Server.AllowedPeerIdentities) == 1 && r.Direction == "inbound" &&
		len(r.AllowedPeers) == 1 && r.AllowedPeers[0] == c.Server.AllowedPeerIdentities[0]
}
