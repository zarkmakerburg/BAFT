//go:build linux

package ingressownership

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// listenWithForensics is net.Listen plus, ONLY when the bind fails with
// EADDRINUSE, a read-only snapshot of observable kernel facts for the numeric
// port: every /proc/net/tcp{,6} row (state + socket inode) and which of THIS
// process's open file descriptors reference those inodes. It never sleeps,
// retries, or alters the outcome; it records evidence so an intermittent
// post-Retire EADDRINUSE names its real holder (leaked/duplicated FD of ours,
// another process's listener, or only non-listening connection state) instead
// of an assumed cause.
func listenWithForensics(t *testing.T, network, address string) (net.Listener, error) {
	t.Helper()
	ln, err := net.Listen(network, address)
	if err != nil && errorsIsAddrInUse(err) {
		t.Logf("EADDRINUSE forensics for %s %s:\n%s", network, address, tcpPortForensics(address))
	}
	return ln, err
}

func errorsIsAddrInUse(err error) bool {
	for err != nil {
		if e, ok := err.(syscall.Errno); ok {
			return e == syscall.EADDRINUSE
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func tcpPortForensics(address string) string {
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "unparseable address: " + err.Error()
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "unparseable port: " + err.Error()
	}
	var out strings.Builder
	inodes := map[string]string{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		raw, err := os.ReadFile(table)
		if err != nil {
			fmt.Fprintf(&out, "%s: %v\n", table, err)
			continue
		}
		for _, line := range strings.Split(string(raw), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) < 10 {
				continue
			}
			addr := strings.Split(f[1], ":")
			if len(addr) != 2 {
				continue
			}
			if n, err := strconv.ParseUint(addr[1], 16, 16); err != nil || int(n) != port {
				continue
			}
			fmt.Fprintf(&out, "%s local=%s remote=%s state=%s uid=%s inode=%s\n", table, f[1], f[2], f[3], f[7], f[9])
			inodes[f[9]] = f[3]
		}
	}
	fds, err := filepath.Glob("/proc/self/fd/*")
	if err != nil {
		fmt.Fprintf(&out, "fd scan: %v\n", err)
	}
	for _, fd := range fds {
		target, err := os.Readlink(fd)
		if err != nil || !strings.HasPrefix(target, "socket:[") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
		if state, ok := inodes[inode]; ok {
			fmt.Fprintf(&out, "this process fd %s -> socket inode %s (tcp state %s)\n", filepath.Base(fd), inode, state)
		}
	}
	if out.Len() == 0 {
		return "no /proc/net/tcp row for the port; holder is not an observable TCP socket in this netns"
	}
	return out.String()
}
