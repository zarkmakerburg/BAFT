//go:build linux

package ingressauthority

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"golang.org/x/sys/unix"
)

// ProvisionInstalledManagedNodePolicy is a local root-only installer action.
// It uses the single leaf wire encoder/decoder and a fixed on-node path and
// source set. It never overwrites existing policy, modifies service sources,
// asserts complete coverage, acquires a mutation lease or changes the host
// listener/firewall. BCC job parameters cannot supply policy paths or roots.
func ProvisionInstalledManagedNodePolicy(ctx context.Context, nodeID string) (ingressinventory.InstalledManagedNodePolicyV1, error) {
	var zero ingressinventory.InstalledManagedNodePolicyV1
	if ctx == nil {
		return zero, ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if os.Geteuid() != 0 {
		return zero, ErrUnverified
	}
	if !idPattern.MatchString(nodeID) {
		return zero, ErrBinding
	}
	ns, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		return zero, errors.Join(ErrUnverified, err)
	}
	p := ingressinventory.InstalledManagedNodePolicyV1{SchemaVersion: 1, InventoryVersion: ingressinventory.ManagedCoverageVersion, NodeID: nodeID, Platform: "linux", NetworkNamespace: ns, Transport: "dedicated", Manager: "systemd-socket-v1", SourceRoots: []string{"/etc/systemd/system", "/usr/lib/systemd/system"}}
	raw, err := ingressinventory.MarshalInstalledManagedNodePolicyV1(ctx, p)
	if err != nil {
		return zero, errors.Join(ErrUnverified, err)
	}
	digest := sha256.Sum256(raw)
	wanted := hex.EncodeToString(digest[:])
	directory, err := openPolicyBootstrapDirectory()
	if err != nil {
		return zero, err
	}
	defer unix.Close(directory)
	existing, err := readProvisionedPolicy(ctx, directory, nodeID, wanted)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return zero, err
	}
	temporary := ".managed-node-" + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(directory, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return zero, errors.Join(ErrUnverified, err)
	}
	defer unix.Unlinkat(directory, temporary, 0)
	file := os.NewFile(uintptr(fd), temporary)
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return zero, err
	}
	n, err := file.Write(raw)
	if err != nil {
		return zero, err
	}
	if n != len(raw) {
		return zero, io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return zero, err
	}
	var identity unix.Stat_t
	if err := unix.Fstat(fd, &identity); err != nil {
		return zero, err
	}
	if err := file.Close(); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	// No replace is essential: a competing installer or re-pair must not
	// replace the inode/identity already used by held or recoverable work.
	err = unix.Renameat2(directory, temporary, directory, "managed-node.json", unix.RENAME_NOREPLACE)
	created := err == nil
	if err != nil && !errors.Is(err, unix.EEXIST) {
		return zero, errors.Join(ErrUnverified, err)
	}
	if err := unix.Fsync(directory); err != nil {
		return zero, err
	}
	installed, err := readProvisionedPolicy(ctx, directory, nodeID, wanted)
	if err != nil {
		return zero, err
	}
	if created && (installed.ProfileDevice != uint64(identity.Dev) || installed.ProfileInode != identity.Ino) {
		return zero, ErrUnverified
	}
	return installed, nil
}

func openPolicyBootstrapDirectory() (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, errors.Join(ErrUnverified, err)
	}
	owned := true
	defer func() {
		if owned {
			_ = unix.Close(fd)
		}
	}()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Uid != 0 || st.Mode&0022 != 0 {
		return -1, ErrUnverified
	}
	for i, component := range []string{"etc", "baft", "ingress"} {
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) && i > 0 {
			if err = unix.Mkdirat(fd, component, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
				return -1, errors.Join(ErrUnverified, err)
			}
			// Persist newly-created private path components before publication.
			if err = unix.Fsync(fd); err != nil {
				return -1, err
			}
			next, err = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		}
		if err != nil {
			return -1, errors.Join(ErrUnverified, err)
		}
		_ = unix.Close(fd)
		fd = next
		if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != 0 || st.Mode&0022 != 0 || st.Mode&07000 != 0 {
			return -1, ErrUnverified
		}
		if i == 2 && st.Mode&0777 != 0700 {
			return -1, ErrUnverified
		}
	}
	owned = false
	return fd, nil
}

func readProvisionedPolicy(ctx context.Context, directory int, nodeID, wantedSHA string) (ingressinventory.InstalledManagedNodePolicyV1, error) {
	var zero ingressinventory.InstalledManagedNodePolicyV1
	p, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, nodeID)
	if err != nil {
		return zero, err
	}
	if p.ProfileSHA256 != wantedSHA {
		return zero, ErrUnverified
	}
	fd, err := unix.Openat(directory, "managed-node.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return zero, errors.Join(ErrUnverified, err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || p.ProfileDevice != uint64(st.Dev) || p.ProfileInode != st.Ino {
		return zero, ErrUnverified
	}
	if err := unix.Fsync(fd); err != nil {
		return zero, err
	}
	if err := unix.Fsync(directory); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return p, nil
}
