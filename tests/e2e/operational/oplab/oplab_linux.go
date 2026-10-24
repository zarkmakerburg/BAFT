//go:build linux

// Package oplab is the shared LAB support for the operational-ingress-v1 gate
// tests. It is imported only by _test.go files.
//
// The gate runs inside a disposable private NETWORK namespace (run_lab.sh) and
// nothing may be created on the host. The operational path, however, reads a
// protected policy at the fixed path /etc/baft/ingress/managed-node.json and
// scans systemd unit source roots. oplab therefore re-executes the test
// binary in a private MOUNT namespace and gives it a tmpfs-backed copy of
// /etc and /usr/lib/systemd/system. Every write the tests make to those fixed
// paths lands in the copy and disappears with the namespace; the host
// filesystem is never modified. Linux UID 0 is never treated as isolation.
package oplab

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

const childEnv = "BAFT_OPV1_MNT_CHILD"

// Main is called from TestMain of every package that needs the fixed-path
// policy. Outside gate mode it just runs the tests (they skip when
// unprivileged). In gate mode it re-executes itself in a private mount
// namespace and relays the exit status; the child performs the mounts.
func Main(m interface{ Run() int }) int {
	if os.Getenv(childEnv) == "1" {
		if err := prepareMounts(); err != nil {
			fmt.Fprintf(os.Stderr, "oplab: cannot prepare private mount namespace: %v\n", err)
			return 1
		}
		return m.Run()
	}
	if !Gate() {
		return m.Run()
	}
	// The inventory requires the test process to be the ONLY process in the
	// network namespace, so no parent may stay behind. Unshare the mount
	// namespace on this (locked) thread and exec in place: execve keeps the
	// calling thread's namespaces and replaces the whole process.
	runtime.LockOSThread()
	if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
		fmt.Fprintf(os.Stderr, "oplab: cannot create a private mount namespace: %v\n", err)
		return 1
	}
	err := syscall.Exec("/proc/self/exe", os.Args, append(os.Environ(), childEnv+"=1"))
	fmt.Fprintf(os.Stderr, "oplab: exec in private mount namespace: %v\n", err)
	return 1
}

func prepareMounts() error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}
	// /dev/shm exists on every Linux host and in containers; the tmpfs is
	// mounted over it inside the private namespace only.
	if err := unix.Mount("oplab", "/dev/shm", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode=1777,size=256m"); err != nil {
		return fmt.Errorf("tmpfs: %w", err)
	}
	work := "/dev/shm/oplab"
	if err := os.Mkdir(work, 0755); err != nil {
		return err
	}
	if err := os.Chmod(work, 0755); err != nil {
		return err
	}
	for _, d := range []struct{ src, name string }{
		{"/etc", "etc"},
		{"/usr/lib/systemd/system", "unit"},
	} {
		if st, err := os.Stat(d.src); err != nil || !st.IsDir() {
			return fmt.Errorf("%s must exist as a directory in the LAB image/host: %v", d.src, err)
		}
		dst := filepath.Join(work, d.name)
		if err := os.Mkdir(dst, 0755); err != nil {
			return err
		}
		if err := copyTree(d.src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", d.src, err)
		}
		if err := unix.Mount(dst, d.src, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s: %w", d.src, err)
		}
	}
	return nil
}

// RequireNetns verifies the private-network-namespace prerequisites only (for
// tests that need nft and a disposable netns but not the fixed-path policy).
func RequireNetns(t testing.TB) {
	t.Helper()
	requireCommon(t, false)
}

// Require verifies the full LAB prerequisites including the private mount
// namespace that backs the fixed-path installed policy. In gate mode every
// missing prerequisite is a hard failure. Outside gate mode (developer or
// general CI runs) a genuinely missing privilege skips the test.
func Require(t testing.TB) {
	t.Helper()
	requireCommon(t, true)
}

func requireCommon(t testing.TB, mounts bool) {
	t.Helper()
	missing := func(format string, args ...any) {
		t.Helper()
		msg := fmt.Sprintf(format, args...)
		if Gate() {
			t.Fatalf("operational ingress LAB prerequisite missing: %s", msg)
		}
		t.Skipf("requires the operational-ingress-v1 private-netns LAB (run_lab.sh): %s", msg)
	}
	if os.Geteuid() != 0 {
		missing("root required")
	}
	if mounts && Gate() && os.Getenv(childEnv) != "1" {
		t.Fatal("gate test binary did not run in its private mount namespace (TestMain must call oplab.Main)")
	}
	if !privateNetns() {
		missing("a private network namespace (no interface but lo, separate from PID 1) is required")
	}
	if mounts && !Gate() && os.Getenv(childEnv) != "1" {
		missing("private mount namespace not prepared (set %s=1 and use run_lab.sh)", GateEnv)
	}
	if _, err := os.Stat(NFT); err != nil {
		missing("%s not installed (apt install nftables)", NFT)
	}
	if IPBinary() == "" {
		missing("iproute2 'ip' not installed")
	}
}

// privateNetns refuses to run mutating tests anywhere that is not a disposable
// network namespace: it must differ from PID 1's namespace when that is
// readable, and in every case it may contain no interface except lo.
func privateNetns() bool {
	self, e1 := os.Readlink("/proc/self/ns/net")
	if e1 != nil {
		return false
	}
	if init, e2 := os.Readlink("/proc/1/ns/net"); e2 == nil && init == self {
		return false
	}
	raw, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n")[2:] {
		name, _, found := strings.Cut(strings.TrimSpace(line), ":")
		if found && name != "lo" {
			return false
		}
	}
	return true
}

// IPBinary returns the iproute2 binary path, or "" when absent.
func IPBinary() string {
	for _, p := range []string{"/usr/sbin/ip", "/sbin/ip", "/bin/ip", "/usr/bin/ip"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// copyTree copies a directory tree preserving type, mode, ownership and
// symlinks (a minimal `cp -a`; the container LAB image has no coreutils).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		switch {
		case info.IsDir():
			if rel != "." {
				if err := os.Mkdir(target, 0700); err != nil {
					return err
				}
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
			return os.Lchown(target, int(st.Uid), int(st.Gid))
		case info.Mode().IsRegular():
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				in.Close()
				return err
			}
			_, err = io.Copy(out, in)
			in.Close()
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		default:
			return nil // sockets, devices and fifos are not needed
		}
		if err := os.Lchown(target, int(st.Uid), int(st.Gid)); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode().Perm()|info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky))
	})
}
