package tunnelengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

const (
	DriverContractVersion  = 1
	InventorySchemaVersion = 1
	MaxInventoryBytes      = 16 << 10
	MaxInventoryEntries    = 16
	MaxInventoryLifetime   = 5 * time.Minute
)

type Installation string

const (
	InstallationUnknown Installation = "unknown"
	InstallationAbsent  Installation = "absent"
	InstallationPresent Installation = "present"
)

var (
	inventoryIDRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	inventoryDigestRE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	inventoryVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
)

// InventoryChallenge is BCC-owned dispatch context. The caller must obtain it
// from the current signed job and authenticate the reporting node BEFORE using
// AdvanceInventory. These types do not verify signatures or authenticate ACKs.
// Generation is a persisted, monotonically increasing per-node probe generation;
// Nonce is a fresh 32-byte random challenge encoded as lower-case hex.
type InventoryChallenge struct {
	NodeID     string
	JobID      string
	Generation uint64
	Nonce      string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// EngineObservation contains installation facts only. Present is not evidence
// of process health, a working path, artifact signature verification or readiness.
type EngineObservation struct {
	Engine          string       `json:"engine"`
	ContractVersion int          `json:"contract_version"`
	Installation    Installation `json:"installation"`
	Version         string       `json:"version,omitempty"`
	ArtifactSHA256  string       `json:"artifact_sha256,omitempty"`
}

// InventoryReport intentionally has no agent timestamp, ready flag, executable
// path, command or secret. Freshness uses BCC's dispatch/receipt clocks only.
type InventoryReport struct {
	SchemaVersion int                 `json:"schema_version"`
	NodeID        string              `json:"node_id"`
	JobID         string              `json:"job_id"`
	Generation    uint64              `json:"generation"`
	Nonce         string              `json:"nonce"`
	Engines       []EngineObservation `json:"engines"`
}

// InventorySnapshot is immutable outside this package. A zero value supplies no
// evidence. It cannot be populated by unmarshalling an untrusted ready/verified
// boolean. Persistence and signed probe/ACK wiring are separate integration work.
type InventorySnapshot struct {
	nodeID     string
	jobID      string
	nonce      string
	generation uint64
	issuedAt   time.Time
	observedAt time.Time
	expiresAt  time.Time
	engines    map[string]EngineObservation
}

func (s InventorySnapshot) NodeID() string        { return s.nodeID }
func (s InventorySnapshot) Generation() uint64    { return s.generation }
func (s InventorySnapshot) ObservedAt() time.Time { return s.observedAt }
func (s InventorySnapshot) ExpiresAt() time.Time  { return s.expiresAt }

func (s InventorySnapshot) Fresh(now time.Time) bool {
	return !s.observedAt.IsZero() && !now.Before(s.observedAt) && now.Before(s.expiresAt)
}

// Observation returns a value copy; callers must separately check freshness.
func (s InventorySnapshot) Observation(engine string) (EngineObservation, bool) {
	observation, ok := s.engines[engine]
	return observation, ok
}

// AdvanceInventory validates a report against trusted CURRENT dispatch context.
// Old/repeated generations cannot refresh an existing snapshot, even if still
// inside the challenge lifetime. Errors return no candidate and never mutate
// previous. A late ACK cannot extend expiry beyond the original dispatch window.
func AdvanceInventory(previous InventorySnapshot, expected InventoryChallenge, payload []byte, receivedAt time.Time) (InventorySnapshot, error) {
	fail := func(reason string) (InventorySnapshot, error) { return InventorySnapshot{}, errors.New(reason) }
	if !inventoryIDRE.MatchString(expected.NodeID) || !inventoryIDRE.MatchString(expected.JobID) ||
		expected.Generation == 0 || !inventoryDigestRE.MatchString(expected.Nonce) {
		return fail("invalid inventory challenge binding")
	}
	if expected.IssuedAt.IsZero() || receivedAt.IsZero() || expected.ExpiresAt.IsZero() ||
		!expected.ExpiresAt.After(expected.IssuedAt) || expected.ExpiresAt.Sub(expected.IssuedAt) > MaxInventoryLifetime ||
		receivedAt.Before(expected.IssuedAt) || !receivedAt.Before(expected.ExpiresAt) {
		return fail("inventory challenge is outside its allowed lifetime")
	}
	if previous.nodeID != "" && (previous.nodeID != expected.NodeID || expected.Generation <= previous.generation ||
		expected.JobID == previous.jobID || expected.Nonce == previous.nonce || expected.IssuedAt.Before(previous.issuedAt) || receivedAt.Before(previous.observedAt)) {
		return fail("inventory update is a replay, rollback or different node")
	}
	if len(payload) == 0 || len(payload) > MaxInventoryBytes {
		return fail("inventory payload exceeds allowed bounds")
	}
	var report InventoryReport
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return fail("malformed inventory report")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fail("inventory report contains trailing data")
	}
	if report.SchemaVersion != InventorySchemaVersion || report.NodeID != expected.NodeID || report.JobID != expected.JobID ||
		report.Generation != expected.Generation || report.Nonce != expected.Nonce {
		return fail("inventory report does not match the current challenge or schema")
	}
	if len(report.Engines) > MaxInventoryEntries {
		return fail("inventory contains too many engine observations")
	}
	engines := make(map[string]EngineObservation, len(report.Engines))
	for _, observation := range report.Engines {
		// An advertised future engine is not an implemented agent contract.
		engine, err := RequireDeployable(observation.Engine)
		if err != nil || engine != observation.Engine || observation.ContractVersion != DriverContractVersion {
			return fail("unsupported inventory engine or contract version")
		}
		if _, duplicate := engines[engine]; duplicate {
			return fail("duplicate inventory engine observation")
		}
		switch observation.Installation {
		case InstallationUnknown, InstallationAbsent:
			if observation.Version != "" || observation.ArtifactSHA256 != "" {
				return fail("unconfirmed installation carries artifact claims")
			}
		case InstallationPresent:
			if len(observation.Version) > 128 || !inventoryVersionRE.MatchString(observation.Version) || !inventoryDigestRE.MatchString(observation.ArtifactSHA256) {
				return fail("present installation lacks a valid version and digest")
			}
		default:
			return fail("unknown inventory installation state")
		}
		engines[engine] = observation
	}
	return InventorySnapshot{nodeID: expected.NodeID, jobID: expected.JobID, nonce: expected.Nonce, generation: expected.Generation,
		issuedAt: expected.IssuedAt.UTC(), observedAt: receivedAt.UTC(), expiresAt: expected.ExpiresAt.UTC(), engines: engines}, nil
}

// EngineSelection is the first versioned native-driver contract. Direction is
// connection initiation, not payload direction. No UDP/carrier/crypto mode is
// inferred from an engine label; new combinations require an actual adapter.
type EngineSelection struct {
	Engine          string `json:"engine"`
	ContractVersion int    `json:"contract_version"`
	Direction       string `json:"direction"`
	Network         string `json:"network"`
}

func (selection EngineSelection) Validate() error {
	engine, err := RequireDeployable(selection.Engine)
	if err != nil || engine != selection.Engine {
		return errors.New("unsupported explicit engine selection")
	}
	if selection.ContractVersion != DriverContractVersion || selection.Direction != "ir_to_ex" || selection.Network != "tcp" {
		return errors.New("unsupported engine contract, direction or network")
	}
	return nil
}

// NodeEngineRequirement must come from BCC's verified release policy and current
// persisted probe generation, never from the report being admitted. A new probe
// invalidates older evidence immediately, even before that evidence expires.
// Different nodes may require different digests
// (for example amd64 versus arm64). This does not replace signature/revocation
// checking, node authorization, port/health gates or two-node data verification.
type NodeEngineRequirement struct {
	NodeID     string
	Generation uint64
	Version    string
	SHA256     string
}

// RequirePairInstalled is a conservative building block, NOT yet a new live BCC
// deployment gate. It requires matching fresh observations on TWO distinct
// nodes and cannot activate an engine disabled by the server registry.
func RequirePairInstalled(selection EngineSelection, ir, ex InventorySnapshot, wantIR, wantEX NodeEngineRequirement, now time.Time) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	if wantIR.NodeID == wantEX.NodeID {
		return errors.New("engine inventory requires distinct endpoint nodes")
	}
	for _, side := range []struct {
		name     string
		snapshot InventorySnapshot
		want     NodeEngineRequirement
	}{{"IR", ir, wantIR}, {"EX", ex, wantEX}} {
		if !inventoryIDRE.MatchString(side.want.NodeID) || side.want.Generation == 0 || len(side.want.Version) > 128 || !inventoryVersionRE.MatchString(side.want.Version) || !inventoryDigestRE.MatchString(side.want.SHA256) {
			return fmt.Errorf("%s artifact requirement is invalid", side.name)
		}
		if side.snapshot.nodeID != side.want.NodeID || side.snapshot.generation != side.want.Generation || !side.snapshot.Fresh(now) {
			return fmt.Errorf("%s engine inventory is missing, stale or for another node", side.name)
		}
		observation, ok := side.snapshot.Observation(selection.Engine)
		if !ok || observation.Installation != InstallationPresent || observation.ContractVersion != selection.ContractVersion ||
			observation.Version != side.want.Version || observation.ArtifactSHA256 != side.want.SHA256 {
			return fmt.Errorf("%s engine installation does not match trusted artifact policy", side.name)
		}
	}
	return nil
}
