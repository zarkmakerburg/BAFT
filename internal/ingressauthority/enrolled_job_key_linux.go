//go:build linux

package ingressauthority

import (
	"context"
	"crypto/sha256"
	"io"
	"os"

	"github.com/zarkmakerburg/baft/internal/release"
	"golang.org/x/sys/unix"
)

const enrolledJobKeyPath = "/etc/baft-agent/bcc-job.pub"

type enrolledJobKeyIdentity struct {
	Public  [32]byte
	Digest  [32]byte
	Device  uint64
	Inode   uint64
	Parents [3][2]uint64
}

// Open every path component without following a symlink. All ancestry and
// bytes must remain root-controlled; even a same-byte replaced inode revokes.
func loadEnrolledJobKey(ctx context.Context) (enrolledJobKeyIdentity, error) {
	var out enrolledJobKeyIdentity
	if ctx == nil || ctx.Err() != nil || os.Geteuid() != 0 {
		return out, ErrUnverified
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return out, ErrUnverified
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	for i, component := range []string{"", "etc", "baft-agent"} {
		if component != "" {
			next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return out, ErrUnverified
			}
			_ = unix.Close(fd)
			fd = next
		}
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 || st.Mode&07000 != 0 {
			return out, ErrUnverified
		}
		out.Parents[i] = [2]uint64{uint64(st.Dev), st.Ino}
	}
	keyFD, err := unix.Openat(fd, "bcc-job.pub", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return out, ErrUnverified
	}
	f := os.NewFile(uintptr(keyFD), enrolledJobKeyPath)
	defer f.Close()
	if unix.Fstat(keyFD, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 || st.Nlink != 1 || st.Mode&0022 != 0 || st.Mode&07000 != 0 || st.Size <= 0 || st.Size > 1024 {
		return out, ErrUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(raw) > 1024 {
		return out, ErrUnverified
	}
	key, err := release.DecodePublic(string(raw))
	if err != nil || len(key) != len(out.Public) {
		return out, ErrUnverified
	}
	copy(out.Public[:], key)
	out.Device, out.Inode, out.Digest = uint64(st.Dev), st.Ino, sha256.Sum256(raw)
	if ctx.Err() != nil {
		return enrolledJobKeyIdentity{}, ctx.Err()
	}
	return out, nil
}
