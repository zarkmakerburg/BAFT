// Package ingressauthority defines the single shared ingress v1 trust
// boundary. This data-only boundary does not enable host mutation.
package ingressauthority

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

var ErrUnverified = errors.New("ingress authority unavailable or unverified")
var ErrBinding = errors.New("invalid operational ingress binding")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const ProtocolV1 = "1"

// BindingV1 identifies a persistent resource, NOT a signed job or a host
// ownership claim. A port reserves its numeric TCP identity across families.
type BindingV1 struct {
	NodeID         string                `json:"node_id"`
	OperationID    string                `json:"operation_id"`
	LeaseID        string                `json:"lease_id"`
	Port           uint16                `json:"public_port"`
	ConfigPort     uint16                `json:"config_port"`
	ListenEndpoint string                `json:"listen_endpoint"`
	LocalTarget    string                `json:"local_target"`
	Transport      string                `json:"transport"`
	PlanSHA256     string                `json:"plan_sha256"`
	ListenerID     string                `json:"listener_id"`
	FirewallID     string                `json:"firewall_id"`
	Spec           ingressmanifest.Spec  `json:"spec"`
	Proof          ingressmanifest.Proof `json:"proof"`
}

// CommandV1 has a separately signed job ID and expiration, independent of
// the longer-lived resource binding. Retire gets a NEW signed job identity.
type CommandV1 struct {
	IngressProtocol string    `json:"ingress_protocol"`
	JobID           string    `json:"job_id"`
	Action          string    `json:"action"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Binding         BindingV1 `json:"binding"`
}

// ReceiptV1 is inert evidence data. Its JSON cannot authenticate an Agent,
// authorize a host mutation or establish a live lease.
type ReceiptV1 struct {
	SchemaVersion     int                  `json:"schema_version"`
	JobID             string               `json:"job_id"`
	Action            string               `json:"action"`
	Binding           BindingV1            `json:"binding"`
	Phase             string               `json:"phase"`
	ListenerEvidence  ListenerEvidenceV1   `json:"listener_evidence"`
	FirewallEvidence  []FirewallEvidenceV1 `json:"firewall_evidence"`
	OwnershipVerified bool                 `json:"ownership_verified"`
	RollbackStatus    string               `json:"rollback_status,omitempty"`
	FailureCause      string               `json:"failure_cause,omitempty"`
}

// Evidence is inert data. Delta obtains actual observations, Alpha ties them
// to durable authority, and Beta authenticates the node/transport and exact
// command/binding before accepting any receipt. A boolean or JSON is not proof.
type ListenerEvidenceV1 struct {
	Endpoint       string `json:"endpoint"`
	Family         string `json:"family"`
	SocketIdentity string `json:"socket_identity"`
}

type FirewallEvidenceV1 struct {
	Family      string `json:"family"`
	Table       string `json:"table"`
	Chain       string `json:"chain"`
	ChainHandle uint64 `json:"chain_handle"`
	RuleHandle  uint64 `json:"rule_handle"`
	OwnerTag    string `json:"owner_tag"`
}

// Lease is a concrete opaque capability, not a caller-implementable interface.
// Executors must accept *Lease, never an arbitrary callback returning success.
type Lease = LiveLease

// LiveLease is an inert zero value. Only a future Alpha-owned durable
// authority can construct a valid lock-held permit. Never use as a bypass.
type LiveLease struct {
	minted  bool
	binding BindingV1
}

func (l *LiveLease) CheckLockedIngress(ctx context.Context, command CommandV1) error {
	if ctx == nil {
		return ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || !l.minted || l.binding != command.Binding {
		return ErrUnverified
	}
	if err := command.Validate(time.Now()); err != nil {
		return err
	}
	// Until the durable cross-process lock and restart fences are implemented,
	// even an internal mint flag is insufficient to permit host mutation.
	return ErrUnverified
}

func (*LiveLease) MarshalJSON() ([]byte, error) { return nil, ErrUnverified }
func (*LiveLease) UnmarshalJSON([]byte) error   { return ErrUnverified }

func canonicalEndpoint(raw string) (string, int, net.IP, error) {
	host, p, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, nil, ErrBinding
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(p)
	if err != nil || ip == nil || port < 1 || port > 65535 {
		return "", 0, nil, ErrBinding
	}
	canonical := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	if canonical != raw {
		return "", 0, nil, ErrBinding
	}
	return canonical, port, ip, nil
}

// Validate checks canonical shape only, not independent reviewed-plan trust,
// BCC signature, current resource ownership or a held OS lock.
func (b BindingV1) Validate() error {
	for _, id := range []string{b.NodeID, b.OperationID, b.LeaseID, b.ListenerID, b.FirewallID} {
		if !idPattern.MatchString(id) {
			return ErrBinding
		}
	}
	if !digestPattern.MatchString(b.PlanSHA256) || b.Transport != "dedicated" || b.Port == 0 || b.Port == 22 || b.ConfigPort != b.Port {
		return ErrBinding
	}
	if b.Spec.Config.MaxClients <= 0 || b.Spec.Config.DialTimeoutMS <= 0 {
		return ErrBinding
	}
	ep, port, _, err := canonicalEndpoint(b.ListenEndpoint)
	if err != nil || port != int(b.Port) || ep != b.Spec.Config.Listen {
		return ErrBinding
	}
	target, _, ip, err := canonicalEndpoint(b.LocalTarget)
	if err != nil || !ip.IsLoopback() || target != b.Spec.Config.Target {
		return ErrBinding
	}
	if b.Spec.Generation == 0 || b.Spec.TunnelID == "" || b.Spec.InstanceID == "default" {
		return ErrBinding
	}
	proof, err := ingressmanifest.Preview(b.Spec)
	if err != nil || proof != b.Proof {
		return ErrBinding
	}
	return nil
}

// MatchReviewed enforces an independently held, immutable review tuple.
// Calling Validate alone never proves the BCC review was authentic.
func (b BindingV1) MatchReviewed(reviewed BindingV1) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := reviewed.Validate(); err != nil {
		return err
	}
	if b != reviewed {
		return fmt.Errorf("operational ingress differs from reviewed binding: %w", ErrBinding)
	}
	return nil
}

func (c CommandV1) Validate(now time.Time) error {
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if c.IngressProtocol != ProtocolV1 || !idPattern.MatchString(c.JobID) || now.IsZero() ||
		(c.Action != "public_ingress_apply" && c.Action != "public_ingress_retire" && c.Action != "public_ingress_inspect") ||
		c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) || now.Before(c.IssuedAt) || !now.Before(c.ExpiresAt) {
		return ErrBinding
	}
	return nil
}
