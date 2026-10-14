package tunnelnode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/config"
)

// Read candidate material through bounded, non-symlink final components and
// refuse directory aliases. Hashes stay in the private transaction receipt;
// candidate keys and configuration are never returned in job output.
func preparedMaterial(path string) ([]byte, string, error) {
	if err := realDirectoryAncestors(filepath.Dir(path)); err != nil {
		return nil, "", err
	}
	f, err := openNoFollow(path)
	if err != nil {
		return nil, "", errors.New("prepared material is missing, unreadable or a symlink")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return nil, "", errors.New("prepared material is not a bounded regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, "", errors.New("prepared material read failed or exceeded its limit")
	}
	after, err := f.Stat()
	if err != nil || st.Mode() != after.Mode() || st.Size() != after.Size() || !st.ModTime().Equal(after.ModTime()) {
		return nil, "", errors.New("prepared material changed while reading")
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	current, currentOK := after.Sys().(*syscall.Stat_t)
	if !ok || !currentOK || owner.Uid != current.Uid || owner.Gid != current.Gid {
		return nil, "", errors.New("prepared material ownership is unavailable or changed")
	}
	metadata := fmt.Sprintf("%o:%d:%d\n", st.Mode(), owner.Uid, owner.Gid)
	return b, shaHex(append([]byte(metadata), b...)), nil
}

func preparedDigests(paths ...string) (map[string]string, error) {
	out := make(map[string]string, len(paths))
	for _, path := range paths {
		_, digest, err := preparedMaterial(path)
		if err != nil {
			return nil, err
		}
		out[path] = digest
	}
	return out, nil
}

func (m *Manager) pairingInputs(id string) (map[string]string, error) {
	return preparedDigests(filepath.Join(m.stage(id), "ex-params.json"), filepath.Join(m.stage(id), "pending.json"), filepath.Join(m.stage(id), "psk"), m.noiseKey())
}

func (m *Manager) verifyPairingInputs(t Txn) error {
	if len(t.PreparedPairingInputs) == 0 {
		return nil
	} // older in-flight receipt
	got, err := m.pairingInputs(t.ID)
	if err != nil || !sameDigests(got, t.PreparedPairingInputs) {
		return errors.New("commit refused: prepared EX parameters or pairing material changed")
	}
	return nil
}

func (m *Manager) configInputs(t Txn, cfg config.Config) (map[string]string, error) {
	if cfg.Noise == nil || cfg.Noise.KeyFile != m.noiseKey() || cfg.Management.UnixSocket != filepath.Join(m.StateDir, "admin.sock") {
		return nil, errors.New("prepared config does not match the instance key or management path")
	}
	paths := []string{m.noiseKey()}
	switch t.Role {
	case RoleIR:
		ca := filepath.Join(m.stage(t.ID), "peer-ca.pem")
		if cfg.Node.Role != "dialer" || cfg.Peer == nil || cfg.TLS.CAFile != ca || cfg.TLS.CertFile != "" || cfg.TLS.KeyFile != "" {
			return nil, errors.New("prepared IR config does not match its role or candidate CA")
		}
		paths = append(paths, ca)
	case RoleEX:
		if cfg.Node.Role != "listener" || cfg.Server == nil || cfg.TLS.CAFile != filepath.Join(m.pkiDir(), "ca.pem") || cfg.TLS.CertFile != filepath.Join(m.pkiDir(), "server.pem") || cfg.TLS.KeyFile != filepath.Join(m.pkiDir(), "server.key") {
			return nil, errors.New("prepared EX config does not match its role or permanent PKI paths")
		}
		// EX certificate bytes are verified separately against the candidate
		// PKI identity; its permanent paths need not exist before activation.
		if t.CandidateJobsRequired {
			path := filepath.Join(m.stage(t.ID), "ex-params.json")
			_, identity, err := preparedMaterial(path)
			if err != nil || identity != t.PreparedPairingInputs[path] {
				return nil, errors.New("prepared EX parameters changed after staging")
			}
			paths = append(paths, path)
		}
	default:
		return nil, errors.New("unknown prepared role")
	}
	return preparedDigests(paths...)
}

func (m *Manager) readPreparedConfig(t Txn) ([]byte, config.Config, map[string]string, error) {
	b, identity, err := preparedMaterial(filepath.Join(m.stage(t.ID), "baft.new.yaml"))
	if err != nil {
		return nil, config.Config{}, nil, err
	}
	if t.PreparedConfigSHA != "" && shaHex(b) != t.PreparedConfigSHA {
		return nil, config.Config{}, nil, errors.New("commit refused: prepared configuration changed")
	}
	if t.PreparedConfigIdentity != "" && identity != t.PreparedConfigIdentity {
		return nil, config.Config{}, nil, errors.New("commit refused: prepared config permissions or ownership changed")
	}
	cfg, err := config.DecodeYAML(bytes.NewReader(b))
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("prepared config is invalid: %w", err)
	}
	inputs, err := m.configInputs(t, cfg)
	if err != nil {
		return nil, cfg, nil, err
	}
	if t.PreparedConfigSHA != "" && !sameDigests(inputs, t.PreparedConfigInputs) {
		return nil, cfg, nil, errors.New("commit refused: prepared key or CA changed")
	}
	return b, cfg, inputs, nil
}

// Capture both bytes and dependencies around the installed binary's validator.
// The installer uses the returned immutable bytes rather than reading the path
// again after validation. This is not an atomic host-wide filesystem lock.
func (m *Manager) validatePreparedConfig(ctx context.Context, t Txn) ([]byte, config.Config, map[string]string, error) {
	if err := m.verifyExecutables(ctx, t); err != nil {
		return nil, config.Config{}, nil, err
	}
	b, cfg, inputs, err := m.readPreparedConfig(t)
	if err != nil {
		return nil, cfg, nil, err
	}
	if err := m.verifyServiceAccess(ctx, t, cfg); err != nil {
		return nil, cfg, nil, err
	}
	if out, err := m.System.Run(ctx, m.BaftBin, "config", "validate", "--file", filepath.Join(m.stage(t.ID), "baft.new.yaml")); err != nil {
		return nil, cfg, nil, fmt.Errorf("staged config is invalid: %v: %s", err, tailOf(out))
	}
	if err := m.verifyExecutables(ctx, t); err != nil {
		return nil, cfg, nil, err
	}
	if err := m.verifyServiceAccess(ctx, t, cfg); err != nil {
		return nil, cfg, nil, err
	}
	after, _, afterInputs, err := m.readPreparedConfig(t)
	if err != nil || !bytes.Equal(b, after) || !sameDigests(inputs, afterInputs) {
		return nil, cfg, nil, errors.New("prepared config or dependencies changed during validation")
	}
	return b, cfg, inputs, nil
}

func (m *Manager) recordPreparedConfig(ctx context.Context, t *Txn) error {
	b, _, inputs, err := m.validatePreparedConfig(ctx, *t)
	if err != nil {
		return err
	}
	t.PreparedConfigSHA, t.PreparedConfigInputs = shaHex(b), inputs
	after, identity, err := preparedMaterial(filepath.Join(m.stage(t.ID), "baft.new.yaml"))
	if err != nil || !bytes.Equal(b, after) {
		return errors.New("prepared config changed before its identity was recorded")
	}
	t.PreparedConfigIdentity = identity
	return m.writeTxn(*t)
}
