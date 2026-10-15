package tunnelnode

import (
	"context"
	"errors"
	"time"
)

// CandidateReport proves validated private artifacts and a passive resource
// check, not a running candidate, handshake or payload. No key/config leaves
// the node. BCC binds the digest and generation into the activation job.
type CandidateReport struct {
	Version       int       `json:"version"`
	TunnelID      string    `json:"tunnel_id"`
	InstanceID    string    `json:"instance_id"`
	Role          string    `json:"role"`
	Generation    int       `json:"generation"`
	ConfigSHA256  string    `json:"config_sha256"`
	Resources     bool      `json:"resources"`
	ObservedAt    time.Time `json:"observed_at"`
	Listen        string    `json:"listen,omitempty"`
	PeerAddress   string    `json:"peer_address,omitempty"`
	Target        string    `json:"target,omitempty"`
	RouteID       string    `json:"route_id"`
	RouteListen   string    `json:"route_listen,omitempty"`
	MetricsListen string    `json:"metrics_listen"`
	RecordShaping bool      `json:"record_shaping"`
}

// StageEX completes the listener configuration while the active service and
// PKI remain untouched. Repeating the same completed request only validates
// its sealed candidate; it never consumes another pairing reply.
func (m *Manager) StageEX(ctx context.Context, id, reply string) (CandidateReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.requirePhase(id, RoleEX, PhasePrepared)
	if err != nil {
		return CandidateReport{}, err
	}
	if !t.CandidateJobsRequired {
		return CandidateReport{}, errors.New("transaction did not request separate candidate jobs")
	}
	if t.PreparedReplySHA != "" {
		if t.PreparedReplySHA != shaHex([]byte(reply)) {
			return CandidateReport{}, errors.New("staged candidate belongs to another pairing reply")
		}
	} else if err := m.completeEXConfig(ctx, &t, reply); err != nil {
		return CandidateReport{}, err
	}
	return m.candidateReport(ctx, t)
}

// ValidateCandidate repeats installed-binary validation and local resource
// checks, without generating config or operating any active service.
func (m *Manager) ValidateCandidate(ctx context.Context, id string) (CandidateReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.readTxn(id)
	if err != nil {
		return CandidateReport{}, err
	}
	return m.candidateReport(ctx, t)
}

func (m *Manager) candidateReport(ctx context.Context, t Txn) (CandidateReport, error) {
	if t.Phase != PhasePrepared || m.activeID() != t.ID || !t.CandidateJobsRequired || t.PreparedConfigSHA == "" || t.PreparedConfigIdentity == "" {
		return CandidateReport{}, errors.New("candidate configuration is not completely prepared")
	}
	if t.Role == RoleEX {
		if t.PreparedReplySHA == "" {
			return CandidateReport{}, errors.New("EX pairing reply has not been staged")
		}
		if err := m.verifyPreparedPKI(t); err != nil {
			return CandidateReport{}, err
		}
	}
	_, cfg, _, err := m.validatePreparedConfig(ctx, t)
	if err != nil {
		return CandidateReport{}, err
	}
	if t.ResourcesRequired {
		if err := m.preflightResources(ctx, configurationBinds(cfg)); err != nil {
			return CandidateReport{}, err
		}
	}
	if len(cfg.Routes) != 1 {
		return CandidateReport{}, errors.New("candidate must contain exactly one reviewed route")
	}
	r := CandidateReport{Version: 1, TunnelID: t.ID, InstanceID: m.Instance, Role: t.Role, Generation: m.readGeneration(), ConfigSHA256: t.PreparedConfigSHA, Resources: t.ResourcesRequired, ObservedAt: m.Now().UTC(), RouteID: cfg.Routes[0].ID, RouteListen: cfg.Routes[0].Listen, Target: cfg.Routes[0].Target, MetricsListen: cfg.Management.MetricsListen, RecordShaping: cfg.Noise.RecordShaping.Enabled}
	if cfg.Server != nil {
		r.Listen = cfg.Server.Listen
	}
	if cfg.Peer != nil {
		r.PeerAddress = cfg.Peer.Address
	}
	return r, nil
}

// CommitCandidate installs only the already sealed config acknowledged by
// BCC. It cannot create/accept configuration at the cutover boundary. Local
// artifact/PKI/resource checks repeat immediately before installation.
func (m *Manager) CommitCandidate(ctx context.Context, id, configSHA string, generation int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.readTxn(id)
	if err != nil {
		return "", err
	}
	if configSHA == "" || configSHA != t.PreparedConfigSHA || generation != m.readGeneration() {
		return "", errors.New("candidate digest or generation differs from the reviewed preparation")
	}
	if _, err := m.candidateReport(ctx, t); err != nil {
		return "", err
	}
	return m.install(ctx, t, t.Port)
}
