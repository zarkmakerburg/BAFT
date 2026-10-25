package tunnelnode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zarkmakerburg/baft/internal/config"
)

// requireOwnedInstanceNamespace refuses to pair into a scoped instance whose
// config or unit exists but was not written (and left unmodified) by BAFT for
// this very instance. The scoped namespace is created by BAFT alone, so a file
// there without a matching ownership marker is foreign or edited and must not
// be replaced. The default instance is exempt: it legitimately adopts the
// installer's files.
func (m *Manager) requireOwnedInstanceNamespace() error {
	if m.Instance == "" {
		return nil
	}
	cfg, cfgErr := os.ReadFile(m.liveConfig())
	unit, unitErr := os.ReadFile(m.unitPath())
	if errors.Is(cfgErr, os.ErrNotExist) && errors.Is(unitErr, os.ErrNotExist) {
		if _, err := os.Lstat(m.markerPath()); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("instance %s has an ownership marker without its config or unit; reconciliation required", m.Instance)
	}
	for _, e := range []error{cfgErr, unitErr} {
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return fmt.Errorf("instance %s: cannot read installed files: %w", m.Instance, e)
		}
	}
	raw, err := os.ReadFile(m.markerPath())
	if err != nil {
		return fmt.Errorf("instance %s has files BAFT does not own (no ownership marker); refusing to replace them", m.Instance)
	}
	var mk Marker
	if err := json.Unmarshal(raw, &mk); err != nil || mk.ManagedBy != "baft" || mk.InstanceID != m.Instance {
		return fmt.Errorf("instance %s ownership marker is not BAFT's for this instance; refusing to replace its files", m.Instance)
	}
	sum := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	if cfgErr == nil && sum(cfg) != mk.ConfigSHA256 {
		return fmt.Errorf("instance %s config changed since BAFT wrote it; refusing to replace it", m.Instance)
	}
	if unitErr == nil && sum(unit) != mk.UnitSHA256 {
		return fmt.Errorf("instance %s unit changed since BAFT wrote it; refusing to replace it", m.Instance)
	}
	return nil
}

type listenerUse struct {
	owner string
	host  string
	port  int
}

func splitListen(owner, hostport string) (listenerUse, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return listenerUse{}, false
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return listenerUse{}, false
	}
	return listenerUse{owner: owner, host: host, port: n}, true
}

func wildcardHost(h string) bool {
	ip := net.ParseIP(h)
	return h == "" || (ip != nil && ip.IsUnspecified())
}

// requireFreeListeners fails closed when a scoped instance would bind a
// listener that the primary or a sibling instance has configured. Binding
// would only be caught at service start, and a stopped sibling would silently
// lose its port to the newcomer. Unreadable or unverifiable configs refuse.
func (m *Manager) requireFreeListeners(want ...string) error {
	if m.Instance == "" {
		return nil
	}
	root := filepath.Dir(filepath.Dir(m.ConfigDir))
	paths := map[string]string{"primary": filepath.Join(root, "baft.yaml")}
	entries, err := os.ReadDir(filepath.Join(root, "instances"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot inspect sibling instances: %w", err)
	}
	for _, e := range entries {
		if e.Name() != m.Instance {
			paths[e.Name()] = filepath.Join(root, "instances", e.Name(), "baft.yaml")
		}
	}
	var used []listenerUse
	for owner, path := range paths {
		cfg, err := config.LoadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil && owner == "primary" {
			// The installer's default service is not ours to parse; if it
			// holds a port its own start fails and rolls back. Siblings are
			// BAFT-written and stay fail-closed.
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot verify listeners of %s: %w", owner, err)
		}
		if cfg.Server != nil {
			if u, ok := splitListen(owner, cfg.Server.Listen); ok {
				used = append(used, u)
			}
		}
		for _, r := range cfg.Routes {
			if r.Listen != "" {
				if u, ok := splitListen(owner, r.Listen); ok {
					used = append(used, u)
				}
			}
		}
	}
	for _, w := range want {
		wu, ok := splitListen("", w)
		if !ok {
			return fmt.Errorf("invalid listener %q", w)
		}
		for _, u := range used {
			if u.port == wu.port && (u.host == wu.host || wildcardHost(u.host) || wildcardHost(wu.host)) {
				return fmt.Errorf("listener %s conflicts with %s (%s:%d)", w, u.owner, u.host, u.port)
			}
		}
	}
	return nil
}

// acquireMetricsClaim reserves the instance's metrics port. When this instance
// already owns the claim and a BAFT-written config of this same instance is
// installed on that port (a re-pair or update of a live instance), the claim
// is reused as-is: its running service holds the port, so neither the
// loopback probe nor a fresh claim applies, and created is false so no later
// step releases it. Anything else falls through to a normal exclusive claim.
func (m *Manager) acquireMetricsClaim(port int) (created bool, err error) {
	path := filepath.Join(filepath.Dir(filepath.Dir(m.ConfigDir)), ".metrics-claims", strconv.Itoa(port))
	if b, readErr := os.ReadFile(path); readErr == nil && string(b) == m.Instance+"\n" {
		if m.requireOwnedInstanceNamespace() == nil {
			if cfg, loadErr := config.LoadFile(m.liveConfig()); loadErr == nil {
				if _, p, splitErr := net.SplitHostPort(cfg.Management.MetricsListen); splitErr == nil && p == strconv.Itoa(port) {
					return false, nil
				}
			}
		}
	}
	if _, err := m.ClaimMetricsPort(port); err != nil {
		return false, err
	}
	return true, nil
}
