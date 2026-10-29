package tunnelnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zarkmakerburg/baft/internal/config"
)

// TenantJoinReceipt is returned to BCC after the existing EX listener accepts
// a new authenticated peer. The key is public material; the one-time PSK is
// never returned.
type TenantJoinReceipt struct {
	OwnerTunnelID string `json:"owner_tunnel_id"`
	NodeID        string `json:"node_id"`
	Listen        string `json:"listen"`
	PeerID        string `json:"peer_id"`
	PeerPublicKey string `json:"peer_public_key"`
	Generation    int    `json:"generation"`
	ConfigSHA256  string `json:"config_sha256"`
	UnitSHA256    string `json:"unit_sha256"`
	MarkerSHA256  string `json:"marker_sha256"`
}

// PrepareEXJoin generates a one-time pairing code for a new IR while leaving
// the already-running EX listener, unit and firewall untouched.
func (m *Manager) PrepareEXJoin(ctx context.Context, id, ownerID string, p ExParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	unlock, err := m.lockLifecycle(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	if !idRe.MatchString(ownerID) {
		return "", errors.New("tunnelnode: invalid shared listener owner ID")
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	if p.MetricsListen == "" {
		p.MetricsListen = m.MetricsListen
	}
	if m.Instance != "" && p.MetricsListen != m.MetricsListen {
		return "", errors.New("tunnelnode: tenant metrics address differs from scoped allocation")
	}
	if err := refuseProtectedPorts(p.MetricsListen, p.Port); err != nil {
		return "", err
	}
	if err := m.requireOwnedInstanceNamespace(); err != nil {
		return "", err
	}
	cfg, _, cfgBytes, unitBytes, markerBytes, err := m.readSharedListenerOwner(ownerID)
	if err != nil {
		return "", err
	}
	if cfg.Server == nil || cfg.Noise == nil || cfg.Node.Role != "listener" || portOf(cfg.Server.Listen) != p.Port {
		return "", errors.New("tunnelnode: requested port is not the owner's active Noise listener")
	}
	if cfg.Server.ServerName == "" || cfg.Noise.RecordShaping.Enabled != p.RecordShaping {
		return "", errors.New("tunnelnode: pairing server name or record shaping differs from the shared listener")
	}
	if _, err := os.Stat(m.noiseKey()); err != nil {
		return "", errors.New("tunnelnode: shared listener Noise key is missing")
	}
	caPath := filepath.Join(m.pkiDir(), "ca.pem")
	if _, err := os.Stat(caPath); err != nil {
		return "", errors.New("tunnelnode: shared listener CA is missing")
	}

	p.JoinOwnerID = ownerID
	t, err := m.begin(id, RoleEX)
	if err != nil {
		return "", err
	}
	t.Port, t.JoinOwnerID = p.Port, ownerID
	t.JoinConfigSHA, t.JoinUnitSHA, t.JoinMarkerSHA = shaHex(cfgBytes), shaHex(unitBytes), shaHex(markerBytes)
	if err := m.writeTxn(t); err != nil {
		m.abort(ctx, t)
		return "", err
	}
	if err := writeJSON(filepath.Join(m.stage(id), "ex-params.json"), p, 0o600); err != nil {
		m.abort(ctx, t)
		return "", err
	}
	identity := "urn:baft:node:" + cfg.Node.ID
	args := []string{
		"ex-code", "--key", m.noiseKey(),
		"--address", net.JoinHostPort(p.PublicAddress, strconv.Itoa(p.Port)),
		"--server-name", cfg.Server.ServerName, "--identity", identity, "--ca-file", caPath,
		"--psk-out", filepath.Join(m.stage(id), "psk"),
		"--pending-out", filepath.Join(m.stage(id), "pending.json"), "--ttl", "15m",
	}
	if p.RecordShaping {
		args = append(args, "--record-shaping")
	}
	out, err := m.System.Run(ctx, m.PairBin, args...)
	if err != nil {
		m.abort(ctx, t)
		return "", fmt.Errorf("baft-pair ex-code for shared listener: %v: %s", err, tailOf(out))
	}
	return strings.TrimSpace(out), m.writeTxn(t)
}

// CommitEXJoin verifies the IR reply, adds only that peer to the existing
// listener, and returns the authenticated tenant identity/key to BCC.
func (m *Manager) CommitEXJoin(ctx context.Context, id, reply string) (string, string, error) {
	m.mu.Lock()
	unlock, err := m.lockLifecycle(ctx)
	if err != nil {
		m.mu.Unlock()
		return "", "", err
	}
	t, err := m.requirePhase(id, RoleEX, PhasePrepared)
	if err != nil {
		unlock()
		m.mu.Unlock()
		return "", "", err
	}
	if t.JoinOwnerID == "" {
		unlock()
		m.mu.Unlock()
		return "", "", errors.New("tunnelnode: transaction is not a shared-listener join")
	}
	var p ExParams
	if err := readJSON(filepath.Join(m.stage(id), "ex-params.json"), &p); err != nil || p.JoinOwnerID != t.JoinOwnerID {
		unlock()
		m.mu.Unlock()
		return "", "", errors.New("tunnelnode: shared join parameters are missing or inconsistent")
	}
	// Refuse to accept a reply if the physical owner drifted since Prepare.
	ownerCfg, _, cfgBytes, unitBytes, markerBytes, err := m.readSharedListenerOwner(t.JoinOwnerID)
	if err != nil || shaHex(cfgBytes) != t.JoinConfigSHA || shaHex(unitBytes) != t.JoinUnitSHA || shaHex(markerBytes) != t.JoinMarkerSHA {
		unlock()
		m.mu.Unlock()
		return "", "", errors.New("tunnelnode: shared listener changed after pairing was prepared")
	}
	stage := m.stage(id)
	acceptOut, err := m.System.Run(ctx, m.PairBin, "ex-accept", "--reply", reply,
		"--pending", filepath.Join(stage, "pending.json"), "--psk-file", filepath.Join(stage, "psk"),
		"--key", m.noiseKey(), "--target", p.Target, "--route-id", p.RouteID,
		"--metrics-listen", p.MetricsListen, "--unix-socket", filepath.Join(m.StateDir, "admin.sock"),
		"--config-out", filepath.Join(stage, "baft.new.yaml"), "--join", m.liveConfig())
	if err != nil {
		unlock()
		m.mu.Unlock()
		return "", "", fmt.Errorf("baft-pair ex-accept --join: %v: %s", err, redact(tailOf(out), reply))
	}
	if out, err := m.System.Run(ctx, m.BaftBin, "config", "validate", "--file", filepath.Join(stage, "baft.new.yaml")); err != nil {
		unlock()
		m.mu.Unlock()
		return "", "", fmt.Errorf("staged joined config is invalid: %v: %s", err, tailOf(out))
	}
	staged, err := config.LoadFile(filepath.Join(stage, "baft.new.yaml"))
	if err != nil || staged.Server == nil || staged.Noise == nil || staged.Node.Role != "listener" || staged.Noise.AllowedPeerPublicKeys[peerID] == "" {
		unlock()
		m.mu.Unlock()
		return "", "", errors.New("tunnelnode: ex-accept did not produce verifiable tenant key evidence")
	}
	var added []string
	for id, key := range staged.Noise.AllowedPeerPublicKeys {
		if key == "" || ownerCfg.Noise.AllowedPeerPublicKeys[id] != "" {
			continue
		}
		if _, ok := ownerCfg.Noise.AllowedPeerPublicKeys[id]; !ok {
			added = append(added, id)
		}
	}
	if len(added) != 1 {
		unlock()
		m.mu.Unlock()
		return "", "", fmt.Errorf("tunnelnode: ex-accept must add exactly one new authenticated tenant (added %d); output: %s", len(added), tailOf(acceptOut))
	}
	peerID := added[0]
	peerKey := staged.Noise.AllowedPeerPublicKeys[peerID]
	foundIdentity := false
	for _, id := range staged.Server.AllowedPeerIdentities {
		if id == peerID { foundIdentity = true; break }
	}
	if !foundIdentity {
		unlock()
		m.mu.Unlock()
		return "", "", errors.New("tunnelnode: added tenant key has no matching authenticated identity")
	}
	// Release the manager lock before entering the common durable tenant
	// transaction, which reacquires both the process and cross-process locks.
	unlock()
	m.mu.Unlock()

	ev, err := m.ApplyTenantMutation(ctx, TenantMutation{
		ID: id, Operation: "add", ExpectedOwnerTunnelID: t.JoinOwnerID,
		NodeID: staged.Node.ID, Listen: staged.Server.Listen,
		ExpectedConfigSHA256: t.JoinConfigSHA, ExpectedUnitSHA256: t.JoinUnitSHA,
		ExpectedMarkerSHA256: t.JoinMarkerSHA, PeerID: peerID, PeerPublicKey: peerKey,
		RouteID: p.RouteID, Target: p.Target,
	})
	if err != nil {
		_, _ = m.Rollback(ctx, id)
		return "", "", err
	}
	// Reuse the same durable before-images for normal tunnel rollback, and
	// update this logical transaction without claiming physical listener ownership.
	rollbackJoin := func() {
		jBytes, readErr := os.ReadFile(m.tenantMutationPath(id))
		if readErr == nil {
			var j tenantMutationJournal
			if json.Unmarshal(jBytes, &j) == nil {
				_ = m.rollbackTenantMutation(ctx, &j)
			}
		}
		_, _, _ = m.Rollback(ctx, id)
	}
	mutationDir := m.tenantMutationDir()
	for source, dest := range map[string]string{
		"old-config": m.backup(id, "baft.yaml"), "old-unit": m.backup(id, "unit"),
		"old-marker": m.backup(id, "marker.json"), "old-owner-txn": m.backup(id, "owner-txn.json"),
	} {
		b, err := os.ReadFile(filepath.Join(mutationDir, id+"."+source))
		if err != nil {
			rollbackJoin()
			return "", "", err
		}
		if err := writeFile(dest, b, 0o600); err != nil {
			rollbackJoin()
			return "", "", err
		}
	}
	current, err := m.readTxn(id)
	if err != nil {
		rollbackJoin()
		return "", "", err
	}
	current.HadConfig, current.HadUnit, current.HadMarker = true, true, true
	current.WasActive = true
	if enabled, _ := m.System.Systemctl(ctx, "is-enabled", m.Service); enabled == "enabled" {
		current.WasEnabled = true
	}
	current.Generation, current.PreviousGeneration = ev.Generation, ev.Generation-1
	current.InstalledConfigSHA, current.InstalledUnitSHA, current.InstalledMarkerSHA = ev.AfterConfigSHA, ev.AfterUnitSHA, ev.AfterMarkerSHA
	current.ConfigWritten, current.UnitChanged = true, true
	current.PeerID, current.PeerPublicKey = peerID, peerKey
	current.TenantRouteID, current.TenantTarget = p.RouteID, p.Target
	current.Phase = PhaseCommitted
	if err := m.writeTxn(current); err != nil {
		rollbackJoin()
		return "", "", err
	}
	receipt, err := json.Marshal(TenantJoinReceipt{
		OwnerTunnelID: ev.OwnerTunnelID, NodeID: staged.Node.ID, Listen: staged.Server.Listen, PeerID: peerID, PeerPublicKey: peerKey,
		Generation: ev.Generation, ConfigSHA256: ev.AfterConfigSHA, UnitSHA256: ev.AfterUnitSHA, MarkerSHA256: ev.AfterMarkerSHA,
	})
	if err != nil {
		rollbackJoin()
		return "", "", err
	}
	return "shared listener tenant committed", string(receipt), nil
}

func (m *Manager) readSharedListenerOwner(ownerID string) (config.Config, Marker, []byte, []byte, []byte, error) {
	var cfg config.Config
	var marker Marker
	cfgBytes, err := os.ReadFile(m.liveConfig())
	if err != nil {
		return cfg, marker, nil, nil, nil, err
	}
	markerBytes, err := os.ReadFile(m.markerPath())
	if err != nil {
		return cfg, marker, nil, nil, nil, err
	}
	unitBytes, err := os.ReadFile(m.unitPath())
	if err != nil {
		return cfg, marker, nil, nil, nil, err
	}
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		return cfg, marker, nil, nil, nil, err
	}
	cfg, err = config.LoadFile(m.liveConfig())
	if err != nil {
		return cfg, marker, nil, nil, nil, err
	}
	owner, ownerErr := m.readTxn(ownerID)
	if ownerErr != nil || owner.ID != ownerID || owner.Role != RoleEX || owner.Phase != PhaseFinalized ||
		marker.ManagedBy != "baft" || marker.Version != 1 || marker.TunnelID != ownerID ||
		marker.InstanceID != m.Instance || marker.Role != RoleEX ||
		marker.ConfigSHA256 != shaHex(cfgBytes) || marker.UnitSHA256 != shaHex(unitBytes) ||
		owner.InstalledConfigSHA != shaHex(cfgBytes) || owner.InstalledUnitSHA != shaHex(unitBytes) ||
		owner.InstalledMarkerSHA != shaHex(markerBytes) || owner.Generation != marker.Generation ||
		m.readGeneration() != marker.Generation {
		return config.Config{}, Marker{}, nil, nil, nil, errors.New("tunnelnode: physical shared listener ownership proof does not match")
	}
	return cfg, marker, cfgBytes, unitBytes, markerBytes, nil
}
