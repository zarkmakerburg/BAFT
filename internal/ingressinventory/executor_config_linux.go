//go:build linux

package ingressinventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func loadInstalledManagedExecutorConfig(ctx context.Context, nodeID string) (InstalledManagedExecutorConfig, error) {
	var cfg InstalledManagedExecutorConfig
	if ctx == nil || os.Geteuid() != 0 {
		return cfg, ErrUnsupportedManagedNode
	}
	if err := ctx.Err(); err != nil {
		return cfg, err
	}
	policy, err := LoadInstalledManagedNodePolicy(ctx, nodeID)
	if err != nil {
		return cfg, err
	}
	raw, err := readProtectedInstalledPolicy(InstalledManagedExecutorPath)
	if err != nil {
		return cfg, err
	}
	// Canonical encoding rejects duplicate, case-aliased, extra, metadata and
	// null fields using one wire definition. Installer writes json.Marshal.
	cfg, err = parseInstalledManagedExecutorConfig(raw.content, nodeID, policy.NetworkNamespace)
	if err != nil {
		return InstalledManagedExecutorConfig{}, err
	}
	cfg.ProfileDevice, cfg.ProfileInode = raw.dev, raw.ino
	sum := sha256.Sum256(raw.content)
	cfg.ProfileSHA256 = hex.EncodeToString(sum[:])
	cfg.JournalDevice, cfg.JournalInode, err = protectedSourceRootIdentity(InstalledManagedMutationJournalPath)
	if err != nil {
		return InstalledManagedExecutorConfig{}, err
	}
	info, err := os.Lstat(InstalledManagedMutationJournalPath)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return InstalledManagedExecutorConfig{}, ErrUnsupportedManagedNode
	}
	if _, _, err := protectedSourceRootIdentity("/usr/sbin"); err != nil {
		return InstalledManagedExecutorConfig{}, err
	}
	fd, err := unix.Open(InstalledManagedNFTBinary, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return InstalledManagedExecutorConfig{}, err
	}
	f := os.NewFile(uintptr(fd), "installed-nft")
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG ||
		st.Mode&0022 != 0 || st.Mode&0111 == 0 || st.Size < 1 || st.Size > 16<<20 {
		return InstalledManagedExecutorConfig{}, ErrUnsupportedManagedNode
	}
	binary, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if err != nil || len(binary) > 16<<20 {
		return InstalledManagedExecutorConfig{}, ErrUnsupportedManagedNode
	}
	digest := sha256.Sum256(binary)
	cfg.NFTDevice, cfg.NFTInode, cfg.NFTSHA256 = uint64(st.Dev), st.Ino, hex.EncodeToString(digest[:])
	// Recheck directory and profile after all reads; inode replacement cannot
	// turn a previously trusted observation into a different installed source.
	again, err := readProtectedInstalledPolicy(InstalledManagedExecutorPath)
	if err != nil || again.dev != raw.dev || again.ino != raw.ino || !bytes.Equal(again.content, raw.content) {
		return InstalledManagedExecutorConfig{}, errors.Join(ErrCoverageIncomplete, err)
	}
	dev, ino, err := protectedSourceRootIdentity(InstalledManagedMutationJournalPath)
	if err != nil || dev != cfg.JournalDevice || ino != cfg.JournalInode {
		return InstalledManagedExecutorConfig{}, ErrCoverageIncomplete
	}
	return cfg, ctx.Err()
}
