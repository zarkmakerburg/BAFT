package tunnelnode

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

// Use the same real account/group access semantics as install.sh. The helper
// drops its own credentials only; it creates no file and prints no material.
// DAC/ACL access is not proof of systemd confinement or a running candidate.
const serviceAccessProgram = `import os, pwd, sys
try:
    pw = pwd.getpwnam(sys.argv[1])
    os.initgroups(pw.pw_name, pw.pw_gid)
    os.setgid(pw.pw_gid)
    os.setuid(pw.pw_uid)
except (KeyError, OSError):
    raise SystemExit(2)
flags = {"r": os.R_OK, "x": os.X_OK, "wx": os.W_OK | os.X_OK}
for i in range(2, len(sys.argv), 2):
    if not os.access(sys.argv[i+1], flags[sys.argv[i]]):
        print((i-2)//2)
        raise SystemExit(1)
`

type serviceAccessCheck struct{ kind, path, label string }

func (m *Manager) checkServiceAccess(ctx context.Context, user string, checks []serviceAccessCheck) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	args := []string{"-c", serviceAccessProgram, user}
	for _, check := range checks {
		args = append(args, check.kind, check.path)
	}
	out, err := m.System.Run(ctx, "python3", args...)
	if ctx.Err() != nil {
		return errors.New("candidate service account access check exceeded its deadline or was cancelled")
	}
	if err == nil {
		return nil
	}
	if n, parseErr := strconv.Atoi(strings.TrimSpace(out)); parseErr == nil && n >= 0 && n < len(checks) {
		return fmt.Errorf("candidate service account cannot access %s; repair host permissions before activation", checks[n].label)
	}
	return errors.New("candidate service account access is unavailable; verify account, groups and python3 before activation")
}

func (m *Manager) prepareServiceUser(ctx context.Context, required bool) (string, error) {
	if !required || m.User == "" {
		return "", nil
	} // old protocol / ownership-free test adapter
	if err := m.checkServiceAccess(ctx, m.User, nil); err != nil {
		return "", err
	}
	return m.User, nil
}

func (m *Manager) verifyServiceAccess(ctx context.Context, t Txn, cfg config.Config) error {
	if t.PreparedServiceUser == "" {
		return nil
	} // older in-flight receipt
	if t.PreparedServiceUser != m.User {
		return errors.New("candidate service account changed since preparation")
	}
	checks := []serviceAccessCheck{
		{"x", m.BaftBin, "BAFT executable"},
		{"r", m.noiseKey(), "Noise key"},
		{"wx", m.StateDir, "instance state directory"},
		{"x", m.ConfigDir, "instance configuration directory"},
	}
	if t.Role == RoleEX {
		for _, name := range []string{"ca.pem", "server.pem", "server.key"} {
			checks = append(checks, serviceAccessCheck{"r", filepath.Join(m.preparedPKIDir(t), name), "prepared TLS " + name})
		}
	} else {
		checks = append(checks, serviceAccessCheck{"r", cfg.TLS.CAFile, "prepared peer CA"})
	}
	return m.checkServiceAccess(ctx, t.PreparedServiceUser, checks)
}
