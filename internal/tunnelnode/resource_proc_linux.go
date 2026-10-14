//go:build linux

package tunnelnode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func boundedProcFile(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(b)) > max {
		return nil, errors.New("proc resource evidence is unreadable or too large")
	}
	return b, nil
}

// Linux prints native-endian 32-bit words, including four words for IPv6.
func procTCPAddress(s string, ipv6 bool) (string, error) {
	f := strings.Split(s, ":")
	width := 8
	if ipv6 {
		width = 32
	}
	if len(f) != 2 || len(f[0]) != width || len(f[1]) != 4 {
		return "", errors.New("invalid proc TCP endpoint")
	}
	b, err := hex.DecodeString(f[0])
	if err != nil {
		return "", err
	}
	for i := 0; i < len(b); i += 4 {
		n := binary.NativeEndian.Uint32(b[i : i+4])
		binary.BigEndian.PutUint32(b[i:i+4], n)
	}
	p, err := strconv.ParseUint(f[1], 16, 16)
	if err != nil || p == 0 {
		return "", errors.New("invalid proc TCP port")
	}
	return net.JoinHostPort(net.IP(b).String(), strconv.Itoa(int(p))), nil
}

func parseProcTCP(b []byte, ipv6 bool) ([]TCPBinding, error) {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	if !scanner.Scan() || !strings.Contains(scanner.Text(), "local_address") {
		return nil, errors.New("missing TCP inventory header")
	}
	out := []TCPBinding{}
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) < 10 {
			return nil, errors.New("malformed TCP inventory row")
		}
		state, err := strconv.ParseUint(f[3], 16, 8)
		if err != nil {
			return nil, errors.New("malformed TCP inventory state")
		}
		if state != 10 {
			continue
		} // TCP_LISTEN only; never treat a connection as a bind.
		addr, err := procTCPAddress(f[1], ipv6)
		if err != nil {
			return nil, err
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			return nil, errors.New("listening socket identity unavailable")
		}
		out = append(out, TCPBinding{Address: addr, Inode: inode})
		if len(out) > 8192 {
			return nil, errors.New("TCP inventory exceeds bound")
		}
	}
	return out, scanner.Err()
}

func hostTCPInventory(ctx context.Context) ([]TCPBinding, error) {
	out := []TCPBinding{}
	for _, family := range []struct {
		path string
		v6   bool
	}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := boundedProcFile(family.path, 1<<20)
		if err != nil {
			// Missing/hidden IPv6 evidence is not proof of a free dual-stack
			// port; disabling IPv6 addresses does not prove socket absence.
			return nil, err
		}
		sockets, err := parseProcTCP(b, family.v6)
		if err != nil {
			return nil, err
		}
		out = append(out, sockets...)
		if len(out) > 8192 {
			return nil, errors.New("TCP inventory exceeds bound")
		}
	}
	return out, ctx.Err()
}

func hostUnixSocketInode(ctx context.Context, path string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	b, err := boundedProcFile("/proc/net/unix", 1<<20)
	if err != nil {
		return 0, err
	}
	return parseProcUnixListener(b, path)
}

func parseProcUnixListener(b []byte, path string) (uint64, error) {
	s := bufio.NewScanner(bytes.NewReader(b))
	if !s.Scan() || !strings.Contains(s.Text(), "Inode") {
		return 0, errors.New("Unix inventory header unavailable")
	}
	var found uint64
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 7 {
			return 0, errors.New("malformed Unix inventory")
		}
		if len(f) < 8 || strings.Join(f[7:], " ") != path {
			continue
		}
		flags, ef := strconv.ParseUint(f[3], 16, 32)
		kind, ek := strconv.ParseUint(f[4], 16, 16)
		_, es := strconv.ParseUint(f[5], 16, 8)
		if ef != nil || ek != nil || es != nil {
			return 0, errors.New("invalid Unix socket flags/type/state")
		}
		// Accepted stream connections can retain the listener's pathname.
		// Only the SOCK_STREAM socket with SO_ACCEPTCON owns the listen bind.
		if kind != 1 || flags&0x10000 == 0 {
			continue
		}
		if found != 0 {
			return 0, errors.New("ambiguous Unix socket path")
		}
		n, err := strconv.ParseUint(f[6], 10, 64)
		if err != nil || n == 0 {
			return 0, errors.New("Unix socket inode unavailable")
		}
		found = n
	}
	if err := s.Err(); err != nil {
		return 0, err
	}
	if found == 0 {
		return 0, errors.New("Unix socket absent from inventory")
	}
	return found, nil
}

func procStartTime(base string) (string, error) {
	b, err := boundedProcFile(filepath.Join(base, "stat"), 4096)
	if err != nil {
		return "", err
	}
	end := strings.LastIndex(string(b), ") ")
	if end < 0 {
		return "", errors.New("process identity unavailable")
	}
	f := strings.Fields(string(b[end+2:]))
	if len(f) < 20 {
		return "", errors.New("process identity unavailable")
	}
	n, err := strconv.ParseUint(f[19], 10, 64)
	if err != nil || n == 0 {
		return "", errors.New("process start identity unavailable")
	}
	return f[19], nil
}

func serviceSocketInodes(ctx context.Context, pidText string) (map[uint64]bool, error) {
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return nil, errors.New("invalid service PID")
	}
	base := filepath.Join("/proc", strconv.Itoa(pid))
	if pid == os.Getpid() {
		base = "/proc/self"
	}
	selfNS, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		return nil, err
	}
	pNS, err := os.Readlink(filepath.Join(base, "ns/net"))
	if err != nil {
		return nil, fmt.Errorf("service network namespace unavailable: %w", err)
	}
	if pNS != selfNS {
		return nil, errors.New("service is in another network namespace")
	}
	start, err := procStartTime(base)
	if err != nil {
		return nil, err
	}
	dir, err := os.Open(filepath.Join(base, "fd"))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4097)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("service FD evidence exceeds bound")
	}
	out := map[uint64]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		link, err := os.Readlink(filepath.Join(base, "fd", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // closed FD cannot prove ownership
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
			continue
		}
		inode, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), 10, 64)
		if err != nil || inode == 0 {
			return nil, errors.New("invalid service socket identity")
		}
		out[inode] = true
	}
	after, err := procStartTime(base)
	afterNS, nsErr := os.Readlink(filepath.Join(base, "ns/net"))
	if err != nil || nsErr != nil || after != start || afterNS != selfNS {
		return nil, fmt.Errorf("service process or namespace changed while reading sockets")
	}
	return out, nil
}
