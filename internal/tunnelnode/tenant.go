package tunnelnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

// TenantMutation is a signed, compare-and-swap request to change one tenant on
// a BAFT-owned EX listener. The caller must provide the exact config and marker
// digests observed during review; stale plans fail closed.
type TenantMutation struct {
	ID                   string
	Operation            string // "add" or "remove"
	NodeID               string
	Listen               string
	ExpectedConfigSHA256 string
	ExpectedMarkerSHA256 string
	PeerID               string
	PeerPublicKey        string
	RouteID              string
	Target               string // required only for add
}

type TenantMutationEvidence struct {
	ID              string `json:"id"`
	Operation       string `json:"operation"`
	NodeID          string `json:"node_id"`
	Generation      int    `json:"generation"`
	BeforeConfigSHA string `json:"before_config_sha256"`
	AfterConfigSHA  string `json:"after_config_sha256"`
	BeforeMarkerSHA string `json:"before_marker_sha256"`
	AfterMarkerSHA  string `json:"after_marker_sha256"`
}

type tenantMutationJournal struct {
	ID            string                `json:"id"`
	RequestSHA    string                `json:"request_sha256"`
	Phase         string                `json:"phase"`
	Operation     string                `json:"operation"`
	NodeID        string                `json:"node_id"`
	OldGeneration int                   `json:"old_generation"`
	Evidence      TenantMutationEvidence `json:"evidence"`
}

func (m *Manager) tenantMutationDir() string {
	return filepath.Join(m.StateDir, "tenant-mutations")
}
func (m *Manager) tenantMutationPath(id string) string {
	return filepath.Join(m.tenantMutationDir(), id+".json")
}
func (m *Manager) tenantMutationBlob(id, suffix string) string {
	return filepath.Join(m.tenantMutationDir(), id+"."+suffix)
}

// ApplyTenantMutation applies an authenticated-tenant change as a durable
// compare-and-swap transaction. The config and ownership marker are backed up,
// the new config is validated before installation, and service failure restores
// the prior config/marker/generation. A crash-interrupted transaction is
// rolled back before a subsequent mutation can start.
func (m *Manager) ApplyTenantMutation(ctx context.Context, p TenantMutation) (TenantMutationEvidence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	unlock, err := m.lockLifecycle(ctx)
	if err != nil {
		return TenantMutationEvidence{}, err
	}
	defer unlock()

	if !idRe.MatchString(p.ID) || (p.Operation != "add" && p.Operation != "remove") {
		return TenantMutationEvidence{}, errors.New("tunnelnode: invalid tenant mutation identity or operation")
	}
	if p.NodeID == "" || p.Listen == "" || p.PeerID == "" || p.RouteID == "" ||
		p.ExpectedConfigSHA256 == "" || p.ExpectedMarkerSHA256 == "" || p.PeerPublicKey == "" {
		return TenantMutationEvidence{}, errors.New("tunnelnode: incomplete tenant mutation proof")
	}
	if p.Operation == "add" && strings.TrimSpace(p.Target) == "" {
		return TenantMutationEvidence{}, errors.New("tunnelnode: tenant add requires a target")
	}
	if p.Operation == "remove" && p.Target != "" {
		return TenantMutationEvidence{}, errors.New("tunnelnode: tenant remove must not carry a target")
	}
	if err := m.recoverTenantMutations(ctx); err != nil {
		return TenantMutationEvidence{}, err
	}

	requestBytes, err := json.Marshal(p)
	if err != nil {
		return TenantMutationEvidence{}, err
	}
	requestSHA := shaHex(requestBytes)
	journalPath := m.tenantMutationPath(p.ID)
	if existing, readErr := os.ReadFile(journalPath); readErr == nil {
		var j tenantMutationJournal
		if json.Unmarshal(existing, &j) != nil || j.ID != p.ID {
			return TenantMutationEvidence{}, errors.New("tunnelnode: corrupt tenant mutation journal")
		}
		if j.RequestSHA != requestSHA {
			return TenantMutationEvidence{}, errors.New("tunnelnode: mutation ID was already used for a different request")
		}
		if j.Phase == "committed" {
			return j.Evidence, nil
		}
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: prior mutation %s is %s", p.ID, j.Phase)
	} else if !os.IsNotExist(readErr) {
		return TenantMutationEvidence{}, readErr
	}

	currentBytes, err := os.ReadFile(m.liveConfig())
	if err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: read live config: %w", err)
	}
	markerBytes, err := os.ReadFile(m.markerPath())
	if err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: read ownership marker: %w", err)
	}
	beforeConfigSHA, beforeMarkerSHA := shaHex(currentBytes), shaHex(markerBytes)
	if beforeConfigSHA != p.ExpectedConfigSHA256 || beforeMarkerSHA != p.ExpectedMarkerSHA256 {
		return TenantMutationEvidence{}, errors.New("tunnelnode: config or ownership marker changed since the reviewed plan")
	}
	var marker Marker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: invalid ownership marker: %w", err)
	}
	if marker.ManagedBy != "baft" || marker.Version != 1 || marker.Role != RoleEX ||
		marker.InstanceID != m.Instance || marker.ConfigSHA256 != beforeConfigSHA {
		return TenantMutationEvidence{}, errors.New("tunnelnode: listener ownership marker does not match this managed instance")
	}
	unitBytes, err := os.ReadFile(m.unitPath())
	if err != nil || shaHex(unitBytes) != marker.UnitSHA256 {
		return TenantMutationEvidence{}, errors.New("tunnelnode: managed unit ownership proof is missing or stale")
	}
	if m.readGeneration() != marker.Generation {
		return TenantMutationEvidence{}, errors.New("tunnelnode: listener generation does not match ownership marker")
	}
	current, err := config.LoadFile(m.liveConfig())
	if err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: current config is invalid: %w", err)
	}
	var next config.Config
	if p.Operation == "add" {
		next, err = config.AddListenerTenant(current, p.NodeID, p.Listen, p.PeerID, p.PeerPublicKey, p.RouteID, p.Target)
	} else {
		next, err = config.RemoveListenerTenant(current, p.NodeID, p.Listen, p.PeerID, p.PeerPublicKey, p.RouteID)
	}
	if err != nil {
		return TenantMutationEvidence{}, err
	}
	nextBytes, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return TenantMutationEvidence{}, err
	}
	nextBytes = append(nextBytes, '\n')
	// Validate the exact bytes that will become live, not only the in-memory value.
	if _, err := config.DecodeJSON(strings.NewReader(string(nextBytes))); err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: generated tenant config is invalid: %w", err)
	}
	afterConfigSHA := shaHex(nextBytes)
	marker.Generation++
	marker.ConfigSHA256 = afterConfigSHA
	marker.Updated = m.Now().UTC()
	nextMarkerBytes, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return TenantMutationEvidence{}, err
	}
	nextMarkerBytes = append(nextMarkerBytes, '\n')
	afterMarkerSHA := shaHex(nextMarkerBytes)
	evidence := TenantMutationEvidence{
		ID: p.ID, Operation: p.Operation, NodeID: p.NodeID, Generation: marker.Generation,
		BeforeConfigSHA: beforeConfigSHA, AfterConfigSHA: afterConfigSHA,
		BeforeMarkerSHA: beforeMarkerSHA, AfterMarkerSHA: afterMarkerSHA,
	}
	j := tenantMutationJournal{
		ID: p.ID, RequestSHA: requestSHA, Phase: "prepared", Operation: p.Operation,
		NodeID: p.NodeID, OldGeneration: m.readGeneration(), Evidence: evidence,
	}
	if err := os.MkdirAll(m.tenantMutationDir(), 0700); err != nil {
		return TenantMutationEvidence{}, err
	}
	for suffix, data := range map[string][]byte{
		"old-config": currentBytes, "new-config": nextBytes,
		"old-marker": markerBytesOriginal(markerBytes), "new-marker": nextMarkerBytes,
	} {
		if err := writeFile(m.tenantMutationBlob(p.ID, suffix), data, 0600); err != nil {
			return TenantMutationEvidence{}, err
		}
	}
	if err := writeJSON(journalPath, j, 0600); err != nil {
		return TenantMutationEvidence{}, err
	}
	if err := m.writeConfig(nextBytes); err != nil {
		_ = m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: install tenant config: %w", err)
	}
	if err := writeFile(m.markerPath(), nextMarkerBytes, 0644); err != nil {
		_ = m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, err
	}
	if err := m.writeGeneration(marker.Generation); err != nil {
		_ = m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, err
	}
	j.Phase = "applying"
	if err := writeJSON(journalPath, j, 0600); err != nil {
		_ = m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, err
	}
	if _, err := m.System.Systemctl(ctx, "restart", m.Service); err != nil {
		rollbackErr := m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: restart after tenant mutation failed: %v (rollback: %v)", err, rollbackErr)
	}
	if err := m.waitActive(ctx); err != nil {
		rollbackErr := m.rollbackTenantMutation(ctx, &j)
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: listener did not remain active: %v (rollback: %v)", err, rollbackErr)
	}
	j.Phase = "committed"
	if err := writeJSON(journalPath, j, 0600); err != nil {
		return TenantMutationEvidence{}, fmt.Errorf("tunnelnode: mutation applied but durable receipt update failed: %w", err)
	}
	return evidence, nil
}

// markerBytesOriginal returns an owned copy to make the before/after byte
// intent explicit at the transaction call site.
func markerBytesOriginal(b []byte) []byte { return append([]byte(nil), b...) }

func (m *Manager) recoverTenantMutations(ctx context.Context) error {
	entries, err := os.ReadDir(m.tenantMutationDir())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.tenantMutationDir(), entry.Name()))
		if err != nil {
			return err
		}
		var j tenantMutationJournal
		if err := json.Unmarshal(b, &j); err != nil || !idRe.MatchString(j.ID) || entry.Name() != j.ID+".json" {
			return errors.New("tunnelnode: unreadable tenant mutation journal; refusing further mutation")
		}
		switch j.Phase {
		case "committed", "rolled_back":
			continue
		case "prepared", "applying":
			if err := m.rollbackTenantMutation(ctx, &j); err != nil {
				return fmt.Errorf("tunnelnode: recover interrupted tenant mutation %s: %w", j.ID, err)
			}
		default:
			return errors.New("tunnelnode: unknown tenant mutation journal phase")
		}
	}
	return nil
}

func (m *Manager) rollbackTenantMutation(ctx context.Context, j *tenantMutationJournal) error {
	oldConfig, err := os.ReadFile(m.tenantMutationBlob(j.ID, "old-config"))
	if err != nil {
		return err
	}
	newConfig, err := os.ReadFile(m.tenantMutationBlob(j.ID, "new-config"))
	if err != nil {
		return err
	}
	oldMarker, err := os.ReadFile(m.tenantMutationBlob(j.ID, "old-marker"))
	if err != nil {
		return err
	}
	newMarker, err := os.ReadFile(m.tenantMutationBlob(j.ID, "new-marker"))
	if err != nil {
		return err
	}
	liveConfig, err := os.ReadFile(m.liveConfig())
	if err != nil {
		return err
	}
	liveMarker, err := os.ReadFile(m.markerPath())
	if err != nil {
		return err
	}
	if (shaHex(liveConfig) != shaHex(oldConfig) && shaHex(liveConfig) != shaHex(newConfig)) ||
		(shaHex(liveMarker) != shaHex(oldMarker) && shaHex(liveMarker) != shaHex(newMarker)) {
		return errors.New("live config or marker drifted outside the interrupted tenant transaction; refusing overwrite")
	}
	if err := m.writeConfig(oldConfig); err != nil {
		return err
	}
	if err := writeFile(m.markerPath(), oldMarker, 0644); err != nil {
		return err
	}
	if err := m.writeGeneration(j.OldGeneration); err != nil {
		return err
	}
	if _, err := m.System.Systemctl(ctx, "restart", m.Service); err != nil {
		return err
	}
	if err := m.waitActive(ctx); err != nil {
		return err
	}
	j.Phase = "rolled_back"
	return writeJSON(m.tenantMutationPath(j.ID), j, 0600)
}

// Compile-time keepalive for the transaction timestamps used in audit receipts.
var _ = time.Time{}
