package bcc

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

// jobValidity is how long a dispatched job stays runnable on the agent.
const jobValidity = time.Hour

// LoadOrCreateJobKey returns BCC's job-signing key, creating it (owner-only)
// on first use. Agents pin its public key at enrollment.
func LoadOrCreateJobKey(path string) (ed25519.PrivateKey, bool, error) {
	if st, err := os.Stat(path); err == nil {
		if st.Mode().Perm()&0o077 != 0 {
			return nil, false, fmt.Errorf("%s must not be readable or writable by group/other", path)
		}
		key, err := release.ReadPrivate(path)
		return key, false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	_, key, err := release.GenerateKey()
	if err != nil {
		return nil, false, err
	}
	if err := writeFileAtomic(path, []byte(release.EncodePrivate(key)+"\n"), 0o600); err != nil {
		return nil, false, err
	}
	return key, true, nil
}

// ConfigureJobSigning makes BCC sign every job it hands to an agent.
func (s *Server) ConfigureJobSigning(key ed25519.PrivateKey) { s.jobKey = key }

// JobPublicKey is what agents pin, or "" without job signing.
func (s *Server) JobPublicKey() string {
	if validateJobSigningKey(s.jobKey) != nil {
		return ""
	}
	return release.EncodePublic(s.jobKey.Public().(ed25519.PublicKey))
}

func validateJobSigningKey(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("BCC job-signing key is not configured")
	}
	if !bytes.Equal(key, ed25519.NewKeyFromSeed(key.Seed())) {
		return errors.New("BCC job-signing key is inconsistent")
	}
	return nil
}

// AgentJob is a job as served to an agent: the record plus its signed form,
// which is the only part an agent acts on.
type AgentJob struct {
	Job
	Signed *release.Envelope `json:"signed,omitempty"`
}

func agentAction(j Job) (string, map[string]string, error) {
	switch j.Type {
	case JobDeployBAFT:
		return agentjob.ActionUpdateBAFT, map[string]string{"version": j.Version}, nil
	case JobTunnelPrepareEX, JobTunnelPrepareIR, JobTunnelCommitEX, JobTunnelCommitIR, JobTunnelHealth, JobTunnelObserve, JobTunnelInspect, JobTunnelDiscover, JobTunnelFinalize, JobTunnelRollback, agentjob.ActionTunnelRetire,
		agentjob.ActionPathProbeInventory, agentjob.ActionPathProbeListen, agentjob.ActionPathProbeRun, agentjob.ActionPathProbeStop,
		agentjob.ActionPublicIngressPreflight, agentjob.ActionPublicIngressInspect,
		agentjob.ActionCertPrepareEX, agentjob.ActionCertTrustIR, agentjob.ActionCertVerifyEX, agentjob.ActionCertVerifyIR,
		agentjob.ActionCertActivateEX, agentjob.ActionCertConfirmIR, agentjob.ActionCertRetireIR, agentjob.ActionCertRetireEX,
		agentjob.ActionCertRollback:
		params := make(map[string]string, len(j.Params))
		for k, v := range j.Params {
			params[k] = v
		}
		return j.Type, params, nil
	}
	return "", nil, fmt.Errorf("job type %q has no agent action", j.Type)
}

func (s *Server) signAgentJobs(jobs []Job) ([]AgentJob, error) {
	if len(jobs) != 0 {
		if err := validateJobSigningKey(s.jobKey); err != nil {
			return nil, err
		}
	}
	out := make([]AgentJob, 0, len(jobs))
	now := s.now()
	for _, j := range jobs {
		aj := AgentJob{Job: j}
		if s.jobKey != nil {
			deadline := now.Add(jobValidity)
			if j.Type == agentjob.ActionPublicIngressPreflight || j.Type == agentjob.ActionPublicIngressInspect {
				if j.CreatedAt.IsZero() || !now.Before(publicIngressPreflightDeadline(j)) {
					return nil, fmt.Errorf("read-only ingress job %s expired before signing", j.ID)
				}
				if deadline.After(publicIngressPreflightDeadline(j)) {
					deadline = publicIngressPreflightDeadline(j)
				}
			}
			action, params, err := agentAction(j)
			if err != nil {
				return nil, err
			}
			env, err := agentjob.Sign(s.jobKey, agentjob.Job{
				JobID: j.ID, NodeID: j.NodeID, Action: action, Params: params,
				IssuedAt: now, ExpiresAt: deadline,
			})
			if err != nil {
				return nil, fmt.Errorf("sign job %s: %w", j.ID, err)
			}
			aj.Signed = &env
		}
		out = append(out, aj)
	}
	return out, nil
}
