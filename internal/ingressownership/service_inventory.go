package ingressownership

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ErrPersistentServiceInventoryIncomplete forbids treating a partial/unknown
// stopped-service inventory as a free-port authorization.
var ErrPersistentServiceInventoryIncomplete = errors.New("persistent service port inventory is incomplete")

// PersistentTCPPortClaim captures a TCP reservation in a systemd socket unit,
// whether or not its process/socket currently exists. Foreign owners must not
// be unlinked, overwritten, restarted, or silently ignored.
type PersistentTCPPortClaim struct {
	Port       uint16
	Unit       string
	Source     string // unit base filename or overriding drop-in path
	Definition string // normalized ListenStream value, never executable content
}

// SystemdSocketInventory is ONE READ-ONLY source of stopped service claims.
// ScopedComplete means the selected directory's socket units/drop-ins were
// exhaustively enumerated and understood. It does NOT mean that all stopped
// services on a host are known: vendor/runtime/systemd generator directories,
// service ExecStart direct binds, containers and other registries must be
// independently reconciled by the Alpha-owned durable admission authority.
type SystemdSocketInventory struct {
	Claims         []PersistentTCPPortClaim
	ScopedComplete bool
	Root           string
}

// InspectSystemdSocketUnits observes one trusted, explicit unit directory.
// Missing/untrusted/unreadable/ambiguous sources fail closed and never become
// empty-success. It DOES NOT reserve a port or prove full host completeness.
// os.OpenRoot bounds all path access; symlinks and modified files are rejected.
func InspectSystemdSocketUnits(directory string) (SystemdSocketInventory, error) {
	out := SystemdSocketInventory{Root: directory}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return out, fmt.Errorf("%w: untrusted systemd directory: %v (mode=%v)", ErrPersistentServiceInventoryIncomplete, err, func() any {
			if info == nil {
				return nil
			}
			return info.Mode()
		}())
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return out, fmt.Errorf("%w: open source: %v", ErrPersistentServiceInventoryIncomplete, err)
	}
	defer root.Close()
	directoryFile, err := root.Open(".")
	if err != nil {
		return out, fmt.Errorf("%w: reopen inventory directory: %v", ErrPersistentServiceInventoryIncomplete, err)
	}
	names, err := directoryFile.ReadDir(-1)
	_ = directoryFile.Close()
	if err != nil || len(names) > 2048 {
		return out, fmt.Errorf("%w: enumerate units: %v", ErrPersistentServiceInventoryIncomplete, err)
	}
	units := make(map[string]bool)
	for _, entry := range names {
		name := entry.Name()
		if strings.HasSuffix(name, ".socket") {
			units[name] = true
		}
	}
	for _, entry := range names {
		name := entry.Name()
		if strings.HasSuffix(name, ".socket.d") && !units[strings.TrimSuffix(name, ".d")] {
			return out, fmt.Errorf("%w: orphan socket override %q", ErrPersistentServiceInventoryIncomplete, name)
		}
	}
	sorted := make([]string, 0, len(units))
	for name := range units {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	if len(sorted) > 512 {
		return out, fmt.Errorf("%w: too many socket units", ErrPersistentServiceInventoryIncomplete)
	}
	for _, unit := range sorted {
		content, err := safeReadUnit(root, unit, 64<<10)
		if err != nil {
			return out, fmt.Errorf("%w: %s: %v", ErrPersistentServiceInventoryIncomplete, unit, err)
		}
		values, err := parseSystemdListenStream(content, nil, unit)
		if err != nil {
			return out, fmt.Errorf("%w: %s: %v", ErrPersistentServiceInventoryIncomplete, unit, err)
		}
		dropDir := unit + ".d"
		dropInfo, err := root.Lstat(dropDir)
		if err == nil {
			if !dropInfo.IsDir() || dropInfo.Mode()&os.ModeSymlink != 0 || dropInfo.Mode().Perm()&0022 != 0 {
				return out, fmt.Errorf("%w: untrusted override directory %s", ErrPersistentServiceInventoryIncomplete, dropDir)
			}
			d, openErr := root.Open(dropDir)
			if openErr != nil {
				return out, fmt.Errorf("%w: cannot open override directory %s: %v", ErrPersistentServiceInventoryIncomplete, dropDir, openErr)
			}
			dropNames, listErr := d.ReadDir(-1)
			_ = d.Close()
			if listErr != nil || len(dropNames) > 128 {
				return out, fmt.Errorf("%w: invalid override directory %s", ErrPersistentServiceInventoryIncomplete, dropDir)
			}
			var overrides []string
			for _, d := range dropNames {
				if strings.HasSuffix(d.Name(), ".conf") {
					overrides = append(overrides, path.Join(dropDir, d.Name()))
				}
			}
			sort.Strings(overrides)
			for _, drop := range overrides {
				data, err := safeReadUnit(root, drop, 64<<10)
				if err != nil {
					return out, fmt.Errorf("%w: unsafe override %s: %v", ErrPersistentServiceInventoryIncomplete, drop, err)
				}
				values, err = parseSystemdListenStream(data, values, drop)
				if err != nil {
					return out, fmt.Errorf("%w: malformed override %s: %v", ErrPersistentServiceInventoryIncomplete, drop, err)
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return out, fmt.Errorf("%w: override status %s: %v", ErrPersistentServiceInventoryIncomplete, dropDir, err)
		}
		out.Claims = append(out.Claims, values...)
		if len(out.Claims) > 2048 {
			return out, fmt.Errorf("%w: too many claimed ports", ErrPersistentServiceInventoryIncomplete)
		}
	}
	// Confirm path remained the selected original directory; the opened
	// root itself never follows a replaced path outside its original tree.
	now, err := os.Lstat(directory)
	if err != nil || !os.SameFile(info, now) {
		return out, fmt.Errorf("%w: source directory replaced", ErrPersistentServiceInventoryIncomplete)
	}
	sort.Slice(out.Claims, func(i, j int) bool {
		if out.Claims[i].Port != out.Claims[j].Port {
			return out.Claims[i].Port < out.Claims[j].Port
		}
		if out.Claims[i].Unit != out.Claims[j].Unit {
			return out.Claims[i].Unit < out.Claims[j].Unit
		}
		return out.Claims[i].Source < out.Claims[j].Source
	})
	out.ScopedComplete = true
	return out, nil
}

func safeReadUnit(root *os.Root, name string, max int64) ([]byte, error) {
	// os.OpenRoot bounds symlinks to the root; reject any symlink at the
	// requested file path and any file that changed identity during open.
	fi, err := root.Lstat(name)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0022 != 0 || fi.Size() > max || fi.Size() < 0 {
		return nil, fmt.Errorf("untrusted unit file: %v", err)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(fi, opened) {
		return nil, errors.New("unit file was replaced")
	}
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(raw)) > max {
		return nil, errors.New("oversized or unreadable unit")
	}
	latest, err := root.Lstat(name)
	if err != nil || !os.SameFile(fi, latest) || !opened.ModTime().Equal(latest.ModTime()) || opened.Size() != latest.Size() {
		return nil, errors.New("unit changed during inventory")
	}
	return raw, nil
}

// parseSystemdListenStream reads persistent TCP ListenStream= directives,
// including empty resets in sorted .socket.d drop-ins. Unknown address,
// service-name, quoting, escaped or malformed syntax is not interpreted as
// "no reservation"; it makes this source INCOMPLETE instead.
func parseSystemdListenStream(data []byte, inherited []PersistentTCPPortClaim, source string) ([]PersistentTCPPortClaim, error) {
	unit := source
	if i := strings.Index(unit, ".socket"); i >= 0 {
		unit = unit[:i+len(".socket")]
	}
	claims := append([]PersistentTCPPortClaim(nil), inherited...)
	if strings.IndexByte(string(data), 0) >= 0 {
		return nil, errors.New("NUL in unit")
	}
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, errors.New("malformed section")
			}
			section = line[1 : len(line)-1]
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			return nil, errors.New("unparsed unit line")
		}
		if section != "Socket" || strings.TrimSpace(kv[0]) != "ListenStream" {
			continue
		}
		v := strings.TrimSpace(kv[1])
		if v == "" {
			claims = nil // systemd empty assignment resets previous values
			continue
		}
		// Unix socket paths do not claim TCP ports. Require literal
		// absolute paths, not service names or ambiguous relative paths.
		if strings.HasPrefix(v, "/") {
			if strings.ContainsAny(v, " \t\r\\\"") || path.Clean(v) != v {
				return nil, errors.New("invalid Unix socket path")
			}
			continue
		}
		portText := v
		if strings.Contains(v, ":") {
			host, port, err := net.SplitHostPort(v)
			if err != nil || net.ParseIP(host) == nil {
				return nil, errors.New("unsupported ListenStream address")
			}
			portText = port
		}
		if portText == "" || strings.Trim(portText, "0123456789") != "" {
			return nil, errors.New("unknown ListenStream service or port")
		}
		n, err := strconv.Atoi(portText)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid ListenStream port")
		}
		claims = append(claims, PersistentTCPPortClaim{Port: uint16(n), Unit: unit, Source: source, Definition: v})
	}
	return claims, nil
}

// ReservedInThisSystemdSource is a VETO-ONLY observation. false means the
// selected source did not claim it; it NEVER authorizes host mutation or says
// all disabled/stopped services on the machine were checked.
func (s SystemdSocketInventory) ReservedInThisSystemdSource(port uint16) (bool, error) {
	if !s.ScopedComplete || port == 0 {
		return false, ErrPersistentServiceInventoryIncomplete
	}
	for _, claim := range s.Claims {
		if claim.Port == port {
			return true, nil
		}
	}
	return false, nil
}

// RefuseAdmissionFromSocketInventory is intentionally VETO-ONLY: a matching
// stopped/foreign service claim returns ErrConflict; an empty result returns
// ErrPersistentServiceInventoryIncomplete because no systemd socket directory
// scan can attest ownership of unrelated .service, runtime, container or
// Alpha's persisted BAFT claims. In particular, absence is NOT an Apply ACK.
// It has no listener/firewall mutation side effects.
func (s SystemdSocketInventory) RefuseAdmissionFromSocketInventory(port uint16) error {
	reserved, err := s.ReservedInThisSystemdSource(port)
	if err != nil {
		return err
	}
	if reserved {
		return ErrConflict
	}
	return ErrPersistentServiceInventoryIncomplete
}
