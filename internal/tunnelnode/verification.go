package tunnelnode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/verification"
)

// VerificationCapability checks the installed binary, not its release label.
// It runs only this fixed read-only command, with bounded output and lifetime.
func (m *Manager) VerificationCapability(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := m.System.Run(ctx, m.BaftBin, "capabilities")
	if err != nil || len(out) > 4096 {
		return false
	}
	var c struct {
		Version            int `json:"version"`
		TunnelVerification int `json:"tunnel_verification"`
	}
	return json.Unmarshal([]byte(out), &c) == nil && c.Version == 1 && c.TunnelVerification == verification.Version
}

func (m *Manager) DataEvidence(ctx context.Context, id string) (verification.Proof, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dataEvidenceLocked(ctx, id)
}

func (m *Manager) dataEvidenceLocked(ctx context.Context, id string) (verification.Proof, error) {
	p := verification.Proof{Version: verification.Version, TunnelID: id, InstanceID: m.Instance, State: verification.NotAssessed, At: m.Now().UTC()}
	t, err := m.readTxn(id)
	if err != nil {
		return p, err
	}
	p.Role, p.Generation = t.Role, t.Generation
	if t.Phase != PhaseCommitted && t.Phase != PhaseFinalizing && t.Phase != PhaseFinalized {
		p.Reason = "change is not committed"
		return p, nil
	}
	o, err := m.observeLocked(ctx, id)
	if err != nil {
		p.Reason = "instance observation unavailable"
		return p, nil
	}
	p.ConfigSHA256, p.UnitSHA256, p.MarkerSHA256 = o.ConfigSHA256, o.UnitSHA256, o.MarkerSHA256
	if o.TunnelID != t.ID || o.InstanceID != m.Instance || o.Generation != t.Generation || o.NodeGeneration != t.Generation ||
		!o.Managed || o.MarkerTunnelID != t.ID || o.MarkerGeneration != t.Generation || !o.MarkerConfigMatches || !o.MarkerUnitMatches ||
		o.ConfigSHA256 != t.InstalledConfigSHA || o.UnitSHA256 != t.InstalledUnitSHA || o.MarkerSHA256 != t.InstalledMarkerSHA {
		p.State, p.Reason = verification.Fail, "installed artifact identity changed"
		return p, nil
	}
	if _, err := m.stableSample(ctx); err != nil {
		p.Reason = "service evidence unavailable or service is not stable"
		return p, nil
	}
	cfg, err := config.LoadFile(m.liveConfig())
	if err != nil {
		p.Reason = "installed configuration unavailable"
		return p, nil
	}
	if !verification.Supported(cfg) {
		p.Reason = "runtime proof requires one Noise-authenticated route and peer"
		return p, nil
	}
	if err := validateLoopbackEndpointForVerification(cfg.Management.MetricsListen); err != nil {
		return p, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+cfg.Management.MetricsListen+"/verification", nil)
	if err != nil {
		return p, err
	}
	resp, err := client.Do(req)
	if err != nil {
		p.Reason = "runtime verification endpoint unavailable"
		return p, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.Reason = "runtime verification endpoint unsupported"
		return p, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8193))
	if err != nil || len(b) > 8192 {
		p.Reason = "runtime verification response unavailable or too large"
		return p, nil
	}
	var report verification.RuntimeReport
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		p.Reason = "runtime verification response unreadable"
		return p, nil
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		p.Reason = "runtime verification response has trailing content"
		return p, nil
	}
	p.Runtime = report
	now := m.Now().UTC()
	boot, bootErr := hex.DecodeString(report.BootID)
	if report.Version != verification.Version || !report.Supported || report.ConfigDigest != verification.ConfigDigest(cfg) || bootErr != nil || len(boot) != 16 ||
		report.GeneratedAt.IsZero() || report.GeneratedAt.After(now) || now.Sub(report.GeneratedAt) > 5*time.Second {
		p.Reason = "runtime proof does not match the current configuration, schema or clock"
		return p, nil
	}
	if report.AuthenticatedPeers < 1 || report.ReceivedBytes == 0 || report.DeliveredBytes == 0 || report.DeliveredBytes > report.ReceivedBytes ||
		report.LastDeliveredAt.IsZero() || report.LastDeliveredAt.After(report.GeneratedAt) || now.Sub(report.LastDeliveredAt) >= verification.FreshFor {
		p.Reason = "fresh authenticated application delivery has not been observed"
		return p, nil
	}
	p.DataAgeMS = now.Sub(report.LastDeliveredAt).Milliseconds()
	if t.Role == RoleEX {
		conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", cfg.Routes[0].Target)
		if err != nil {
			p.State, p.Reason = verification.Fail, "selected application target does not accept TCP connections"
			return p, nil
		}
		conn.Close()
		p.TargetReachable = true
	}
	p.State, p.At = verification.Pass, m.Now().UTC()
	p.DataAgeMS = p.At.Sub(report.LastDeliveredAt).Milliseconds()
	if p.DataAgeMS < 0 || p.DataAgeMS >= int64(verification.FreshFor/time.Millisecond) {
		p.State, p.Reason = verification.NotAssessed, "delivery evidence expired during target check"
	}
	return p, nil
}

func validateLoopbackEndpointForVerification(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return errors.New("verification endpoint must be literal loopback")
	}
	return nil
}

// Seal verifies one node without discarding either side's rollback material.
// BCC releases backups only after BOTH nodes returned a ready proof.
func (m *Manager) Seal(ctx context.Context, id string) (verification.Proof, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := m.dataEvidenceLocked(ctx, id)
	if err != nil || p.State != verification.Pass {
		return p, err
	}
	t, err := m.readTxn(id)
	if err != nil {
		return p, err
	}
	p.ReadyToFinalize = true
	t.VerificationRequired, t.ReadyProof = true, &p
	return p, m.writeTxn(t)
}

func (m *Manager) verifySealedArtifacts(t Txn) error {
	p := t.ReadyProof
	if p == nil || p.Version != verification.Version || p.State != verification.Pass || !p.ReadyToFinalize || p.TunnelID != t.ID || p.InstanceID != m.Instance || p.Generation != t.Generation {
		return errors.New("finalize requires a sealed data-delivery proof")
	}
	if p.At.IsZero() || p.At.After(m.Now()) || p.DataAgeMS < 0 || p.DataAgeMS >= int64(verification.FreshFor/time.Millisecond) || m.Now().Sub(p.At)+time.Duration(p.DataAgeMS)*time.Millisecond >= verification.FreshFor {
		return errors.New("sealed proof expired; backups retained")
	}
	for path, want := range map[string]string{m.liveConfig(): p.ConfigSHA256, m.unitPath(): p.UnitSHA256, m.markerPath(): p.MarkerSHA256} {
		b, err := os.ReadFile(path)
		if err != nil || shaHex(b) != want {
			return errors.New("sealed artifacts changed; backups retained")
		}
	}
	return nil
}

// A durable cleanup phase prevents partial secret/backup deletion from being
// presented as a rollbackable change or acknowledged as completed cleanup.
func (m *Manager) finalizeVerified(t Txn) (string, error) {
	if t.Phase == PhaseFinalized {
		if err := m.clearActiveChange(t.ID); err != nil {
			return "", err
		}
		return "already finalized", nil
	}
	if t.Phase != PhaseCommitted && t.Phase != PhaseFinalizing {
		return "", errors.New("verified change is not committed")
	}
	if err := m.verifySealedArtifacts(t); err != nil {
		return "", err
	}
	if t.PKIReplaced {
		backup, err := snapshotPKI(m.pkiDir() + ".before-" + t.ID)
		if err != nil {
			return "", err
		}
		if len(backup) > 0 && !sameDigests(backup, t.PreviousPKI) {
			return "", errors.New("PKI backup changed before finalization; all backups retained")
		}
	}
	if t.Phase != PhaseFinalizing {
		t.Phase = PhaseFinalizing
		if err := m.writeTxn(t); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"psk", "pending.json", "pairing.pending.json", "ex-params.json"} {
		if err := removeDurable(filepath.Join(m.stage(t.ID), name)); err != nil {
			return "", fmt.Errorf("finalize cleanup incomplete: %w", err)
		}
	}
	if err := removeAllDurable(filepath.Join(m.stage(t.ID), "backup")); err != nil {
		return "", fmt.Errorf("finalize backup cleanup incomplete: %w", err)
	}
	if t.PKIReplaced {
		before := m.pkiDir() + ".before-" + t.ID
		backup, err := snapshotPKI(before)
		if err != nil {
			return "", err
		}
		if len(backup) > 0 && !sameDigests(backup, t.PreviousPKI) {
			return "", errors.New("PKI backup changed before finalization; retained")
		}
		if err := removeAllDurable(before); err != nil {
			return "", err
		}
	}
	t.Phase = PhaseFinalized
	if err := m.writeTxn(t); err != nil {
		return "", err
	}
	if err := m.clearActiveChange(t.ID); err != nil {
		return "", err
	}
	return "finalized after both-node data verification", nil
}
