// Package ingressinventory contains host observations that may be consumed
// by Alpha's lower-level ingressauthority without importing ingressownership.
// None of the values in this package is a signed command or a live permit.
package ingressinventory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
)

var ErrCoverageIncomplete = errors.New("managed-node ingress inventory coverage incomplete")
var ErrUnsupportedManagedNode = errors.New("managed-node policy unsupported or untrusted")

// InstalledPolicyPath is the installer-owned, fixed local policy path. The
// exported collection entrypoint NEVER takes a BCC/API-supplied root list,
// filename, booleans or a purported complete snapshot.
const InstalledPolicyPath = "/etc/baft/ingress/managed-node.json"
const ManagedCoverageVersion = 1

// SourceIdentity records bounded source freshness/provenance, not an
// authorization to mutate. The digest changes if observed metadata/claims
// differ; it is not a replacement for a held cross-process admission lock.
// InstalledManagedNodePolicyV1 is the one decoded, independently checked
// *installed* profile value Alpha consumes. It is inert source metadata: a
// caller-modified copy cannot become a permit, and the collector does not
// accept it as an argument. Only LoadInstalledManagedNodePolicy reads the
// fixed protected profile. Alpha must check this identity under its held
// durable lock and reconcile platform/source exclusions before minting.
type InstalledManagedNodePolicyV1 struct {
	ProfilePath      string
	ProfileSHA256    string
	ProfileDevice    uint64
	ProfileInode     uint64
	SchemaVersion    int
	InventoryVersion int
	NodeID           string
	Platform         string
	NetworkNamespace string
	Transport        string
	Manager          string
	SourceRoots      []string
}

// LoadInstalledManagedNodePolicy is the sole exported decoder for Alpha;
// no caller roots, pathname, JSON or "supported=true" parameters exist.
func LoadInstalledManagedNodePolicy(ctx context.Context, expectedNodeID string) (InstalledManagedNodePolicyV1, error) {
	return loadInstalledManagedNodePolicy(ctx, InstalledPolicyPath, expectedNodeID)
}

type SourceFileIdentity struct {
	Path      string
	SHA256    string
	Device    uint64
	Inode     uint64
	ModUnixNS int64
}

type SourceIdentity struct {
	ID         string
	Root       string
	SHA256     string
	Device     uint64
	Inode      uint64
	ModUnixNS  int64
	ClaimCount int
	Files      []SourceFileIdentity // actual source bytes and root-owned file identity
}

// ActiveTCPClaim is one observed kernel entry on the requested numeric
// port, including v4/v6 and state (LISTEN=0A, TIME_WAIT=06).
type ActiveTCPClaim struct {
	Port   uint16
	Family string
	State  string
	Inode  uint64
	Local  string
	Source string
}

// CoverageEvidence is an INERT observation and never a capability. Both a
// verified installed policy and complete trustworthy host discovery are
// needed before Alpha can even evaluate minting a *Lease*. A policy cannot
// confer completeness by merely claiming "supported=true".
// PrivateLABExecutionEvidence describes independently observed singleton
// execution in a disposable loopback-only Linux network namespace. It never
// serves as authority or anticipates processes that may start later.
type PrivateLABExecutionEvidence struct {
	CurrentNetNS      string
	InitNetNS         string
	OwnPID            int
	Interfaces        []string
	Processes         []int
	ProcessStartTicks uint64
	ExecutableDevice  uint64
	ExecutableInode   uint64
	ExecutableSHA256  string
	MountNS           string
	CgroupNS          string
	CgroupMembership  string
}

func (e PrivateLABExecutionEvidence) matches(other PrivateLABExecutionEvidence) bool {
	return reflect.DeepEqual(e, other)
}

type CoverageEvidence struct {
	NodeID            string
	Port              uint16
	PolicyPath        string
	PolicySHA256      string
	PolicyDevice      uint64
	PolicyInode       uint64
	NetNS             string
	InventoryV1       int
	CollectedAt       time.Time
	FreshUntil        time.Time
	Sources           []SourceIdentity
	Stopped           []PersistentTCPPortClaim
	ActiveTCP         []ActiveTCPClaim
	LABExecution      PrivateLABExecutionEvidence
	UnknownSources    []string
	SupportedEnvelope bool // policy+platform checks; does NOT imply complete
	CoverageComplete  bool // remains false until all unsupported classes independently excluded
}

// CollectInstalledManagedTCP uses ONLY the protected on-node installed path,
// never roots or coverage supplied by BCC, signed JSON or the caller. On
// unsupported platforms or incomplete coverage it denies with explicit
// unknown sources. The method does not mutate host resources.
func CollectInstalledManagedTCP(ctx context.Context, expectedNodeID string, port uint16) (CoverageEvidence, error) {
	return collectFromInstalledPath(ctx, InstalledPolicyPath, expectedNodeID, port)
}

// ManagedLABExecutionPin is read-only and caller-mutable observation data,
// never a capability, durable lease or reason to mark an operation successful.
// Alpha must obtain and independently pin it under its held operation guard.
// Privileged host administrators who can enter a private netns are outside
// this first isolated LAB scope; arrivals detected at effect time still DENY.
type ManagedLABExecutionPin struct {
	Policy  InstalledManagedNodePolicyV1
	Sources []SourceIdentity
	Stopped []PersistentTCPPortClaim
	LAB     PrivateLABExecutionEvidence
}

// PinInstalledManagedLABExecution observes the protected fixed-path profile,
// full scoped source identities and actual executable/process/netns/cgroup.
// The only caller input is the expected enrolled node identity.
func PinInstalledManagedLABExecution(ctx context.Context, expectedNodeID string) (ManagedLABExecutionPin, error) {
	return pinInstalledManagedLABExecution(ctx, InstalledPolicyPath, expectedNodeID)
}

// CheckInstalledManagedLABAtEffect remeasures all pin inputs independently
// and denies drift. It does NOT verify a lease, waive any foreign TCP inode,
// or prevent future privileged host-root namespace entry between effects.
// Delta's concrete executor must call it under Alpha's live lease.
func CheckInstalledManagedLABAtEffect(ctx context.Context, expectedNodeID string, pin ManagedLABExecutionPin) error {
	return checkInstalledManagedLABAtEffect(ctx, InstalledPolicyPath, expectedNodeID, pin)
}

func coverageError(e CoverageEvidence, why string) (CoverageEvidence, error) {
	e.UnknownSources = append(e.UnknownSources, why)
	e.CoverageComplete = false
	return e, fmt.Errorf("%w: %s", ErrCoverageIncomplete, why)
}
