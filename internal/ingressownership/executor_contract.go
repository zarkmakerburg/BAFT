package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// IngressBindingV1 is the proposed immutable identity of a signed Agent
// ingress operation. These are DATA FIELDS, not proof of a verified signature,
// reservation, host ownership or permission to change a firewall. Alpha owns
// job signature verification, the durable port lease and replay fencing.
type IngressBindingV1 struct {
	JobID       string
	NodeID      string
	OperationID string
	LeaseID     string
	ListenerID  string
	FirewallID  string
	Action      string    // exact Agent action: public_ingress_apply/retire
	Transport   string    // dedicated; authenticated shared transport not proven
	PlanSHA256  string    // independently signed reviewed-plan digest; NOT the manifest digest
	ExpiresAt   time.Time // signed Agent job expiry; Alpha must reverify the envelope
	PublicPort  uint16
	ConfigPort  uint16 // user-facing configuration port must equal the public listener
	Spec        ingressmanifest.Spec
	Proof       ingressmanifest.Proof
}

const (
	IngressApplyAction  = "public_ingress_apply"
	IngressRetireAction = "public_ingress_retire"
	IngressDedicated    = "dedicated"
)

var (
	ErrExecutorTrustUnavailable = errors.New("trusted ingress executor lease is unavailable")
	ErrExecutorAdmissionExpired = errors.New("trusted ingress execution window has expired")
	bindingIDPattern            = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	bindingDigestPattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidateIngressBindingV1 ensures one signed instruction cannot be forwarded
// to Delta under an alias port, scope or manifest digest. It is only a PURE
// validation gate and MUST NOT be used as an authorization check on its own.
func ValidateIngressBindingV1(b IngressBindingV1) error {
	for _, id := range []string{b.JobID, b.NodeID, b.OperationID, b.LeaseID, b.ListenerID, b.FirewallID} {
		if !bindingIDPattern.MatchString(id) {
			return ErrInvalid
		}
	}
	// Signed executor input carries explicit effective defaults rather than
	// an alternate raw spelling of the same normalized manifest digest.
	if b.Spec.Config.MaxClients == 0 || b.Spec.Config.DialTimeoutMS == 0 {
		return ErrInvalid
	}
	if b.ExpiresAt.IsZero() || !bindingDigestPattern.MatchString(b.PlanSHA256) ||
		(b.Action != IngressApplyAction && b.Action != IngressRetireAction) ||
		b.Transport != IngressDedicated ||
		b.PublicPort == 0 || b.PublicPort == 22 || b.ConfigPort != b.PublicPort {
		return ErrInvalid
	}
	// Reject noncanonical endpoint aliases before they can become distinct
	// action/receipt bytes for the same kernel numeric port.
	listen, err := canonicalEndpoint(b.Spec.Config.Listen)
	if err != nil || listen != b.Spec.Config.Listen {
		return ErrInvalid
	}
	target, err := canonicalEndpoint(b.Spec.Config.Target)
	if err != nil || target != b.Spec.Config.Target {
		return ErrInvalid
	}
	_, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return ErrInvalid
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port != int(b.PublicPort) {
		return ErrInvalid
	}
	// Preview performs the same canonical spec validation as the prepared
	// manifest and binds tunnel+instance+generation to both SHA-256 digests.
	proof, err := ingressmanifest.Preview(b.Spec)
	if err != nil || proof != b.Proof {
		return ErrInvalid
	}
	return nil
}

// IngressLeaseAuthority is the *required trust input* from Alpha's signed
// Agent admission. Its implementation must hold a live, exclusive,
// cross-process per-(node,TCP,numeric-port) lease across kernel effects and
// durable commit, and verify stopped-service reservations, generation,
// reviewed plan and replay identity. An arbitrary JSON token or a Boolean
// preflight response is NOT an implementation of this authority.
//
// Until Alpha has supplied and pinned a trusted, durable implementation,
// this interface is NOT wired to an operational host-mutation path.
type IngressLeaseAuthority interface {
	CheckLockedIngress(context.Context, IngressBindingV1) error
}

// CheckIngressLeaseV1 is a fail-closed *pre-operation* gate. An owner of the
// Agent execution path still must hold and recheck the same authoritative
// lock until a durable terminal journal transition; a successful check alone
// never authorizes a listener/firewall mutation or constitutes a receipt.
func CheckIngressLeaseV1(ctx context.Context, authority IngressLeaseAuthority, binding IngressBindingV1) error {
	if ctx == nil || authority == nil {
		return ErrExecutorTrustUnavailable
	}
	// An interface wrapping a nil pointer is non-nil itself. Reject rather
	// than invoking a nil receiver, crashing the Agent or passing a bogus
	// authority into a future operational executor.
	value := reflect.ValueOf(authority)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		if value.IsNil() {
			return ErrExecutorTrustUnavailable
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateIngressBindingV1(binding); err != nil {
		return err
	}
	if !time.Now().Before(binding.ExpiresAt) {
		return ErrExecutorAdmissionExpired
	}
	if err := authority.CheckLockedIngress(ctx, binding); err != nil {
		return fmt.Errorf("ingress lock or signed owner is unverified: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !time.Now().Before(binding.ExpiresAt) {
		return ErrExecutorAdmissionExpired
	}
	return nil
}

// ObservedIngressV1 describes EVIDENCE that the eventual Delta executor must
// independently obtain after a kernel operation. This is an inert data shape:
// callers cannot turn these fields into an authenticated Agent/BCC receipt,
// and no public constructor here claims OwnershipVerified.
type ObservedIngressV1 struct {
	Binding        IngressBindingV1
	Listener       string // observed exact canonical endpoint
	Family         string // tcp4 or tcp6
	Firewall       FirewallObservation
	NFTChainHandle uint64
	Phase          string // recorded by trusted Agent journal, not inferred here
}
