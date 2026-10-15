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
const serviceAccessProgram = `import hashlib, json, os, pwd, sys
try:
    pw = pwd.getpwnam(sys.argv[1])
    os.initgroups(pw.pw_name, pw.pw_gid)
    os.setgid(pw.pw_gid)
    os.setuid(pw.pw_uid)
except (KeyError, OSError):
    raise SystemExit(2)
identity = hashlib.sha256(json.dumps([1, os.getuid(), os.getgid(), sorted(set(os.getgroups()))], separators=(",", ":")).encode("ascii")).hexdigest()
if sys.argv[2] and identity != sys.argv[2]:
    print("identity_changed")
    raise SystemExit(3)
flags = {"r": os.R_OK, "x": os.X_OK, "wx": os.W_OK | os.X_OK}
for i in range(3, len(sys.argv), 2):
    if not os.access(sys.argv[i+1], flags[sys.argv[i]]):
        print((i-3)//2)
        raise SystemExit(1)
print(identity)
`

type serviceAccessCheck struct{ kind, path, label string }

func (m *Manager) checkServiceAccess(ctx context.Context, user string, checks []serviceAccessCheck) error {
	_, err := m.serviceAccountIdentity(ctx, user, "", checks)
	return err
}

func validServiceIdentity(s string) bool {
	return len(s) == 64 && strings.IndexFunc(s, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) < 0
}

var errServiceIdentityChanged = errors.New("candidate service account identity changed since preparation; roll back and prepare again")

// Seal the effective credentials AFTER switching, not just the account name.
// The versioned digest stays in the private receipt; no numeric identity is
// returned in candidate reports. This detects drift, not an atomic NSS lock.
func (m *Manager) serviceAccountIdentity(ctx context.Context, user, expected string, checks []serviceAccessCheck) (string, error) {
	if expected != "" && !validServiceIdentity(expected) {
		return "", errors.New("prepared service account identity is invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	args := []string{"-I", "-c", serviceAccessProgram, user, expected}
	for _, check := range checks {
		args = append(args, check.kind, check.path)
	}
	out, err := m.System.Run(ctx, "python3", args...)
	if ctx.Err() != nil {
		return "", errors.New("candidate service account access check exceeded its deadline or was cancelled")
	}
	out = strings.TrimSpace(out)
	if err == nil {
		if !validServiceIdentity(out) {
			return "", errors.New("candidate service account identity is unavailable")
		}
		if expected != "" && out != expected {
			return "", errServiceIdentityChanged
		}
		return out, nil
	}
	if out == "identity_changed" {
		return "", errServiceIdentityChanged
	}
	if n, parseErr := strconv.Atoi(out); parseErr == nil && n >= 0 && n < len(checks) {
		return "", fmt.Errorf("candidate service account cannot access %s; repair host permissions before activation", checks[n].label)
	}
	return "", errors.New("candidate service account access is unavailable; verify account, groups and python3 before activation")
}

func (m *Manager) prepareServiceUser(ctx context.Context, required bool) (string, string, error) {
	if !required || m.User == "" {
		return "", "", nil
	} // old protocol / ownership-free test adapter
	identity, err := m.serviceAccountIdentity(ctx, m.User, "", nil)
	if err != nil {
		return "", "", err
	}
	return m.User, identity, nil
}

func (m *Manager) verifyServiceAccess(ctx context.Context, t Txn, cfg config.Config) error {
	if t.PreparedServiceUser == "" {
		if t.PreparedServiceIdentity != "" {
			return errors.New("prepared service account name is missing")
		}
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
	_, err := m.serviceAccountIdentity(ctx, t.PreparedServiceUser, t.PreparedServiceIdentity, checks)
	return err
}
