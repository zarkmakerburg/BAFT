//go:build linux

package ingressinventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The v1 protected installer profile is intentionally a SMALL explicit
// configuration, never a permission flag or an arbitrary complete=true.
// The importer in Alpha must independently pin/check the same installed
// identity and require coverage of every advertised source class.
type installedPolicyV1 struct {
	SchemaVersion int      `json:"schema_version"`
	NodeID        string   `json:"node_id"`
	Platform      string   `json:"platform"`
	NetNS         string   `json:"netns"`
	Transport     string   `json:"transport"`
	InventoryV1   int      `json:"inventory_version"`
	Manager       string   `json:"manager"`
	SourceRoots   []string `json:"source_roots"`
}

type installedProfileIdentity struct {
	content []byte
	dev     uint64
	ino     uint64
}

func pathComponents(path string) ([]string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrUnsupportedManagedNode
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrUnsupportedManagedNode
		}
	}
	return parts, nil
}

// The policy must be rooted in pinned, root-owned, non-writable directory
// descriptors. O_NOFOLLOW/O_NONBLOCK reject symlinks/FIFOs without blocking;
// the opened descriptor (not a path-based stat) supplies the trust identity.
func readProtectedInstalledPolicy(path string) (installedProfileIdentity, error) {
	parts, err := pathComponents(path)
	if err != nil || len(parts) < 2 {
		return installedProfileIdentity{}, ErrUnsupportedManagedNode
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return installedProfileIdentity{}, errors.Join(ErrUnsupportedManagedNode, err)
	}
	defer func() {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}()
	var stat unix.Stat_t
	for i, component := range parts {
		last := i == len(parts)-1
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if !last {
			flags |= unix.O_DIRECTORY
		}
		child, err := unix.Openat(fd, component, flags, 0)
		if err != nil {
			return installedProfileIdentity{}, errors.Join(ErrUnsupportedManagedNode, err)
		}
		unix.Close(fd)
		fd = child
		if err := unix.Fstat(fd, &stat); err != nil || stat.Uid != 0 || stat.Mode&0022 != 0 {
			return installedProfileIdentity{}, fmt.Errorf("%w: unsafe owner/mode: %s", ErrUnsupportedManagedNode, path)
		}
		if !last && stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			return installedProfileIdentity{}, ErrUnsupportedManagedNode
		}
		if last && (stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0077 != 0 || stat.Size < 1 || stat.Size > 32<<10) {
			return installedProfileIdentity{}, ErrUnsupportedManagedNode
		}
		if i == len(parts)-2 && stat.Mode&0077 != 0 {
			// Private /etc/baft/ingress/ (or a root-owned test equivalent).
			return installedProfileIdentity{}, ErrUnsupportedManagedNode
		}
	}
	f := os.NewFile(uintptr(fd), "managed-node-policy")
	fd = -1
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 32<<10+1))
	if err != nil || len(raw) > 32<<10 {
		return installedProfileIdentity{}, ErrUnsupportedManagedNode
	}
	var second unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &second); err != nil || second.Dev != stat.Dev || second.Ino != stat.Ino || second.Size != stat.Size {
		return installedProfileIdentity{}, ErrUnsupportedManagedNode
	}
	return installedProfileIdentity{raw, uint64(stat.Dev), stat.Ino}, nil
}

func parseInstalledPolicy(raw []byte, expectedNodeID string, actualNetNS string) (installedPolicyV1, error) {
	var p installedPolicyV1
	// encoding/json accepts duplicate and case-insensitive struct keys.
	// The installed trust contract requires one exact spelling per field.
	keys := json.NewDecoder(bytes.NewReader(raw))
	if token, err := keys.Token(); err != nil || token != json.Delim('{') {
		return p, ErrUnsupportedManagedNode
	}
	seenKeys := make(map[string]bool)
	for keys.More() {
		token, err := keys.Token()
		key, ok := token.(string)
		if err != nil || !ok || seenKeys[key] {
			return p, ErrUnsupportedManagedNode
		}
		switch key {
		case "schema_version", "node_id", "platform", "netns", "transport", "inventory_version", "manager", "source_roots":
		default:
			return p, ErrUnsupportedManagedNode
		}
		seenKeys[key] = true
		var value json.RawMessage
		if err := keys.Decode(&value); err != nil {
			return p, ErrUnsupportedManagedNode
		}
	}
	if token, err := keys.Token(); err != nil || token != json.Delim('}') {
		return p, ErrUnsupportedManagedNode
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, errors.Join(ErrUnsupportedManagedNode, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return p, ErrUnsupportedManagedNode
	}
	if p.SchemaVersion != 1 || p.InventoryV1 != ManagedCoverageVersion || p.NodeID == "" || p.NodeID != expectedNodeID || p.Platform != "linux" || p.NetNS == "" || p.NetNS != actualNetNS || p.Transport != "dedicated" || p.Manager != "systemd-socket-v1" || len(p.SourceRoots) == 0 || len(p.SourceRoots) > 32 {
		return p, ErrUnsupportedManagedNode
	}
	seen := map[string]bool{}
	for _, root := range p.SourceRoots {
		if _, err := pathComponents(root); err != nil || seen[root] {
			return p, ErrUnsupportedManagedNode
		}
		seen[root] = true
	}
	return p, nil
}

func namespaceIdentity() (string, error) {
	s, err := os.Readlink("/proc/self/ns/net")
	if err != nil || !strings.HasPrefix(s, "net:[") || !strings.HasSuffix(s, "]") {
		return "", ErrUnsupportedManagedNode
	}
	return s, nil
}

func liveTCPPort(ctx context.Context, port uint16) ([]ActiveTCPClaim, []string) {
	var out []ActiveTCPClaim
	var unknown []string
	for _, candidate := range []struct{ path, family string }{{"/proc/net/tcp", "tcp4"}, {"/proc/net/tcp6", "tcp6"}} {
		if err := ctx.Err(); err != nil {
			unknown = append(unknown, candidate.path+": context cancelled")
			break
		}
		f, err := os.Open(candidate.path)
		if err != nil {
			unknown = append(unknown, candidate.path+": unavailable")
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(f, 4<<20+1))
		_ = f.Close()
		if err != nil || len(raw) > 4<<20 {
			unknown = append(unknown, candidate.path+": unreadable/oversized")
			continue
		}
		lines := strings.Split(string(raw), "\n")
		if len(lines) == 0 || !strings.Contains(lines[0], "local_address") || !(strings.Contains(lines[0], "rem_address") || strings.Contains(lines[0], "remote_address")) {
			unknown = append(unknown, candidate.path+": unknown kernel table header")
			continue
		}
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if len(fields) < 10 {
				unknown = append(unknown, candidate.path+": malformed entry")
				break
			}
			addr := strings.Split(fields[1], ":")
			if len(addr) != 2 {
				unknown = append(unknown, candidate.path+": invalid address")
				break
			}
			n, err := strconv.ParseUint(addr[1], 16, 16)
			if err != nil {
				unknown = append(unknown, candidate.path+": invalid hex port")
				break
			}
			if n != uint64(port) {
				continue
			}
			inode, err := strconv.ParseUint(fields[9], 10, 64)
			if err != nil {
				unknown = append(unknown, candidate.path+": invalid socket inode")
				break
			}
			out = append(out, ActiveTCPClaim{Port: port, Family: candidate.family, State: fields[3], Inode: inode, Local: fields[1], Source: candidate.path})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		return out[i].Inode < out[j].Inode
	})
	return out, unknown
}

func sourceFingerprint(root string, claims []PersistentTCPPortClaim) (SourceIdentity, error) {
	dev, inode, err := protectedSourceRootIdentity(root)
	if err != nil {
		return SourceIdentity{}, ErrCoverageIncomplete
	}
	fi, err := os.Lstat(root)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0022 != 0 {
		return SourceIdentity{}, ErrCoverageIncomplete
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || uint64(st.Dev) != dev || st.Ino != inode {
		return SourceIdentity{}, ErrCoverageIncomplete
	}
	files, err := scanPinnedSourceFiles(root)
	if err != nil {
		return SourceIdentity{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\x00%d\x00%d\x00%d", root, st.Dev, st.Ino, fi.ModTime().UnixNano())
	for _, c := range claims {
		fmt.Fprintf(&b, "\x00%d:%s:%s:%s", c.Port, c.Unit, c.Source, c.Definition)
	}
	for _, file := range files {
		fmt.Fprintf(&b, "\x00%s:%s:%d:%d:%d", file.Path, file.SHA256, file.Device, file.Inode, file.ModUnixNS)
	}
	sum := sha256.Sum256([]byte(b.String()))
	newDev, newInode, err := protectedSourceRootIdentity(root)
	if err != nil || newDev != dev || newInode != inode {
		return SourceIdentity{}, ErrCoverageIncomplete
	}
	newfi, err := os.Lstat(root)
	if err != nil || !os.SameFile(fi, newfi) || !fi.ModTime().Equal(newfi.ModTime()) {
		return SourceIdentity{}, ErrCoverageIncomplete
	}
	return SourceIdentity{
		ID: root, Root: root, SHA256: hex.EncodeToString(sum[:]),
		Device: dev, Inode: inode, ModUnixNS: fi.ModTime().UnixNano(),
		ClaimCount: len(claims), Files: files,
	}, nil
}

// Only the fixed-path exported entrypoint uses this function. The path
// override is unexported and used by privileged isolated filesystem tests.
// The unexported path override exists solely for disposable privileged
// isolated filesystem tests. Production callers only get the fixed-path API.
func loadInstalledManagedNodePolicy(ctx context.Context, policyPath, expectedNodeID string) (InstalledManagedNodePolicyV1, error) {
	var zero InstalledManagedNodePolicyV1
	if ctx == nil || expectedNodeID == "" {
		return zero, ErrUnsupportedManagedNode
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(ErrUnsupportedManagedNode, err)
	}
	netns, err := namespaceIdentity()
	if err != nil {
		return zero, errors.Join(ErrUnsupportedManagedNode, err)
	}
	identity, err := readProtectedInstalledPolicy(policyPath)
	if err != nil {
		return zero, errors.Join(ErrUnsupportedManagedNode, err)
	}
	profile, err := parseInstalledPolicy(identity.content, expectedNodeID, netns)
	if err != nil {
		return zero, errors.Join(ErrUnsupportedManagedNode, err)
	}
	sum := sha256.Sum256(identity.content)
	return InstalledManagedNodePolicyV1{
		ProfilePath:      policyPath,
		ProfileSHA256:    hex.EncodeToString(sum[:]),
		ProfileDevice:    identity.dev,
		ProfileInode:     identity.ino,
		SchemaVersion:    profile.SchemaVersion,
		InventoryVersion: profile.InventoryV1,
		NodeID:           profile.NodeID,
		Platform:         profile.Platform,
		NetworkNamespace: netns,
		Transport:        profile.Transport,
		Manager:          profile.Manager,
		SourceRoots:      append([]string(nil), profile.SourceRoots...),
	}, nil
}

func collectFromInstalledPath(ctx context.Context, policyPath, expectedNodeID string, port uint16) (CoverageEvidence, error) {
	result := CoverageEvidence{NodeID: expectedNodeID, Port: port, PolicyPath: policyPath, InventoryV1: ManagedCoverageVersion, CollectedAt: time.Now().UTC()}
	if ctx == nil || port == 0 || expectedNodeID == "" {
		return coverageError(result, "invalid collection input")
	}
	if err := ctx.Err(); err != nil {
		return coverageError(result, "cancelled context")
	}
	profile, err := loadInstalledManagedNodePolicy(ctx, policyPath, expectedNodeID)
	if err != nil {
		return coverageError(result, "missing/untrusted or incompatible installed policy")
	}
	result.NetNS = profile.NetworkNamespace
	result.PolicyDevice, result.PolicyInode = profile.ProfileDevice, profile.ProfileInode
	result.PolicySHA256 = profile.ProfileSHA256
	result.SupportedEnvelope = true // protected profile valid, NOT global manager coverage
	source, err := InspectSystemdSocketSources(profile.SourceRoots)
	if err != nil || !source.ScopedComplete {
		return coverageError(result, "installed service source missing, ambiguous or modified")
	}
	result.Stopped = append(result.Stopped, source.Claims...)
	for _, s := range source.Sources {
		fingerprint, err := sourceFingerprint(s.Root, s.Claims)
		if err != nil {
			return coverageError(result, "installed source fingerprint invalid")
		}
		result.Sources = append(result.Sources, fingerprint)
	}
	alive, unknown := liveTCPPort(ctx, port)
	result.ActiveTCP = alive
	result.UnknownSources = append(result.UnknownSources, unknown...)
	for _, c := range source.Claims {
		if c.Port == port {
			return result, fmt.Errorf("%w: stopped/foreign systemd socket %s owns port %d", ErrClaimConflict, c.Unit, port)
		}
	}
	for _, c := range alive {
		if c.State == "0A" {
			return result, fmt.Errorf("%w: active kernel listener inode %d holds port %d", ErrClaimConflict, c.Inode, port)
		}
	}
	// A trusted policy is deliberately NOT a runtime exclusion oracle.
	// Missing independent proof of these classes still refuses mint.
	for _, category := range []string{"systemd direct ExecStart binds", "systemd runtime/generated or masked units", "container/other supervisor listeners", "other network namespaces", "Alpha durable BAFT claims"} {
		result.UnknownSources = append(result.UnknownSources, category+": independent exclusion not established")
	}
	// Reopen the installed on-node profile through pinned protected path
	// components after independently collecting source/kernel state. Detect
	// policy, inode and namespace drift instead of trusting its first read.
	fresh, err := loadInstalledManagedNodePolicy(ctx, policyPath, expectedNodeID)
	if err != nil || fresh.ProfileSHA256 != profile.ProfileSHA256 ||
		fresh.ProfileDevice != profile.ProfileDevice || fresh.ProfileInode != profile.ProfileInode ||
		fresh.NetworkNamespace != profile.NetworkNamespace ||
		!slices.Equal(fresh.SourceRoots, profile.SourceRoots) {
		return coverageError(result, "installed profile or node namespace changed during observation")
	}
	result.FreshUntil = result.CollectedAt.Add(2 * time.Second)
	return result, fmt.Errorf("%w: %s", ErrCoverageIncomplete, strings.Join(result.UnknownSources, ", "))
}

// Verified format helper for tests/inventory; IP aliases stay numeric-port
// claims and never become a false host permission.
func sameNumericTCPPort(endpoint string, port uint16) bool {
	_, v, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(v)
	return err == nil && n == int(port)
}
