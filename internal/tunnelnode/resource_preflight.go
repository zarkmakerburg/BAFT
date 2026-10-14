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
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

// TCPBinding is a listening socket in the local network namespace. Inode is
// used internally to prove service ownership; no process arguments are read.
type TCPBinding struct {
	Address string
	Inode   uint64
}

func configurationBinds(c config.Config) []string {
	binds := []string{}
	if c.Server != nil {
		binds = append(binds, c.Server.Listen)
	}
	for _, r := range c.Routes {
		if r.Listen != "" {
			binds = append(binds, r.Listen)
		}
	}
	if c.Management.MetricsListen != "" {
		binds = append(binds, c.Management.MetricsListen)
	}
	return binds
}

func parseResourceBind(addr string) (net.IP, int, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, errors.New("resource bind is not an IP:port")
	}
	ip := net.ParseIP(host)
	if host == "" {
		ip = net.IPv4zero
	}
	n, e := strconv.Atoi(port)
	if ip == nil || e != nil || n < 1 || n > 65535 {
		return nil, 0, errors.New("invalid resource bind IP or port")
	}
	return ip, n, nil
}

func resourceBindsOverlap(a, b string) (bool, error) {
	ai, ap, err := parseResourceBind(a)
	if err != nil {
		return false, err
	}
	bi, bp, err := parseResourceBind(b)
	if err != nil {
		return false, err
	}
	// A wildcard can be dual-stack; without socket IPV6_V6ONLY evidence,
	// reserve its port across both families. Specific distinct IPs stay separate.
	return ap == bp && (ai.IsUnspecified() || bi.IsUnspecified() || ai.Equal(bi)), nil
}

func realDirectoryAncestors(path string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return errors.New("resource directory must be absolute")
	}
	for {
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("resource directory is unreadable")
		}
		if err == nil && !info.IsDir() {
			return errors.New("resource directory is a symlink or non-directory")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

// managedResourceConfig refuses namespace adoption by a prepare job. Existing
// artifacts must have a valid matching marker, unit, generation and instance.
func (m *Manager) managedResourceConfig() (*config.Config, error) {
	for _, dir := range []string{m.ConfigDir, m.StateDir, m.UnitDir, filepath.Join(m.StateDir, "tunnels")} {
		if err := realDirectoryAncestors(dir); err != nil {
			return nil, err
		}
	}
	for _, path := range []string{m.noiseKey(), m.pkiDir(), m.activePath(), m.generationPath()} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errors.New("resource ownership path is unreadable")
		}
		if path == m.pkiDir() && info.IsDir() {
			continue
		}
		if path != m.pkiDir() && info.Mode().IsRegular() {
			continue
		}
		return nil, errors.New("resource ownership path is a symlink or special file")
	}
	if _, err := os.Lstat(m.activePath()); err == nil {
		b, problem := regularFile(m.activePath(), 128)
		id := strings.TrimSpace(string(b))
		if problem != "" || !idRe.MatchString(id) {
			return nil, errors.New("active change receipt is unavailable or malformed")
		}
		tb, problem := regularFile(m.txnPath(id), 64<<10)
		var tx Txn
		if problem != "" || json.Unmarshal(tb, &tx) != nil || tx.ID != id || tx.Version != 1 || (tx.Role != RoleEX && tx.Role != RoleIR) {
			return nil, errors.New("active change ownership cannot be verified")
		}
		switch tx.Phase {
		case PhasePrepared, PhaseCommitted, PhaseFinalizing, PhaseFinalized, PhaseRolledBack:
		default:
			return nil, errors.New("active change phase is unknown")
		}
	}
	paths := []string{m.liveConfig(), m.unitPath(), m.markerPath()}
	count := 0
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			count++
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("instance artifact is unreadable")
		}
	}
	if count == 0 {
		return nil, nil
	}
	if count != len(paths) {
		return nil, errors.New("existing instance has incomplete ownership artifacts")
	}
	cb, cp := regularFile(m.liveConfig(), maxConfigFileBytes)
	ub, up := regularFile(m.unitPath(), maxUnitFileBytes)
	mb, mp := regularFile(m.markerPath(), maxMarkerFileBytes)
	if cp != "" || up != "" || mp != "" {
		return nil, errors.New("instance artifacts are unreadable, unbounded or not regular files")
	}
	var mk Marker
	if json.Unmarshal(mb, &mk) != nil || mk.ManagedBy != "baft" || mk.Version != 1 || !idRe.MatchString(mk.TunnelID) ||
		mk.InstanceID != m.Instance || mk.Generation < 1 || mk.Generation != m.readGeneration() ||
		mk.ConfigSHA256 != shaHex(cb) || mk.UnitSHA256 != shaHex(ub) {
		return nil, errors.New("existing instance ownership does not match config, unit, generation or identity")
	}
	c, err := decodeConfig(m.liveConfig(), cb)
	if err != nil {
		return nil, errors.New("owned instance configuration does not load")
	}
	if (mk.Role != RoleEX && mk.Role != RoleIR) || (mk.Role == RoleEX && c.Server == nil) || (mk.Role == RoleIR && c.Peer == nil) {
		return nil, errors.New("owned instance role does not match the installed configuration")
	}
	return &c, nil
}

func (m *Manager) preflightResources(ctx context.Context, binds []string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(binds) == 0 || len(binds) > 256 {
		return errors.New("preflight: invalid number of TCP binds")
	}
	current, err := m.managedResourceConfig()
	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	for i, addr := range binds {
		if _, _, err := parseResourceBind(addr); err != nil {
			return fmt.Errorf("preflight: %w", err)
		}
		for _, other := range binds[:i] {
			if clash, _ := resourceBindsOverlap(addr, other); clash {
				return errors.New("preflight: proposed TCP binds overlap")
			}
		}
	}
	inventory := m.TCPInventory
	if inventory == nil {
		inventory = hostTCPInventory
	}
	sockets, err := inventory(ctx)
	if err != nil || len(sockets) > 8192 {
		return errors.New("preflight: complete local TCP listener inventory is unavailable; nothing installed")
	}
	var owned map[uint64]bool
	getOwned := func() (map[uint64]bool, error) {
		if owned != nil {
			return owned, nil
		}
		pid, err := m.System.Systemctl(ctx, "show", "-p", "MainPID", "--value", m.Service)
		if err != nil || strings.TrimSpace(pid) == "" {
			return nil, errors.New("preflight: current service socket ownership is unavailable")
		}
		owned, err = serviceSocketInodes(ctx, strings.TrimSpace(pid))
		if err != nil {
			return nil, fmt.Errorf("preflight: current service socket ownership is unavailable: %w", err)
		}
		after, err := m.System.Systemctl(ctx, "show", "-p", "MainPID", "--value", m.Service)
		if err != nil || strings.TrimSpace(after) != strings.TrimSpace(pid) {
			return nil, errors.New("preflight: current service changed while reading socket ownership")
		}
		return owned, nil
	}
	var currentBinds []string
	if current != nil {
		currentBinds = configurationBinds(*current)
	}
	admin := filepath.Join(m.StateDir, "admin.sock")
	if info, err := os.Lstat(admin); err == nil {
		if info.Mode()&os.ModeSocket == 0 || current == nil || current.Management.UnixSocket != admin {
			return errors.New("preflight: admin socket path is occupied without verified instance ownership")
		}
		inode, err := hostUnixSocketInode(ctx, admin)
		if err != nil {
			return errors.New("preflight: admin socket identity is unavailable or ambiguous")
		}
		owners, err := getOwned()
		if err != nil {
			return err
		}
		if !owners[inode] {
			return errors.New("preflight: admin socket is not owned by the current verified service")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("preflight: admin socket path is unreadable")
	}
	for _, sock := range sockets {
		if _, _, err := parseResourceBind(sock.Address); err != nil || sock.Inode == 0 {
			return errors.New("preflight: malformed local TCP listener inventory")
		}
		for _, addr := range binds {
			clash, _ := resourceBindsOverlap(addr, sock.Address)
			if !clash {
				continue
			}
			declared := false
			for _, existing := range currentBinds {
				if overlaps, e := resourceBindsOverlap(existing, sock.Address); e == nil && overlaps {
					declared = true
					break
				}
			}
			if !declared {
				return fmt.Errorf("preflight: TCP bind %s is occupied by an unrelated service or instance", addr)
			}
			if owned == nil {
				if _, err := getOwned(); err != nil {
					return err
				}
			}
			if !owned[sock.Inode] {
				return fmt.Errorf("preflight: TCP bind %s is not owned by the current verified service", addr)
			}
		}
	}
	return ctx.Err()
}
