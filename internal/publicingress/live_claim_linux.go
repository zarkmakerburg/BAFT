//go:build linux

package publicingress

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// A successful kernel bind is insufficient to claim a numeric TCP port:
// tcp4/tcp6 or two concrete IPs may legitimately coexist on the same port.
// This bounded snapshot detects *already active* foreign listeners on Linux,
// in the same network namespace, before Serve takes ownership of a socket.
//
// NOT a durable claim, atomic reservation or firewall authorization. The
// future admitted owner still needs generation-fenced orchestration and
// reconciliation of stopped services, and a foreign process could bind after
// this point on a different local address.
func verifyNoOtherLiveTCPListeners(ln net.Listener, port int) error {
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		return errors.New("cannot verify ingress listener socket identity")
	}
	copyFD, err := tcp.File()
	if err != nil {
		return fmt.Errorf("cannot duplicate ingress socket identity: %w", err)
	}
	defer copyFD.Close()
	stat, err := copyFD.Stat()
	if err != nil {
		return fmt.Errorf("cannot inspect ingress socket identity: %w", err)
	}
	info, ok := stat.Sys().(*syscall.Stat_t)
	if !ok || info.Ino == 0 {
		return errors.New("cannot establish ingress socket inode")
	}

	own := info.Ino
	seenOwn := false
	for _, table := range []struct {
		path string
		ipv6 bool
	}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
		raw, err := readBoundedSocketTable(table.path)
		if err != nil {
			return fmt.Errorf("ingress socket inventory is unavailable: %w", err)
		}
		rows, err := inspectSocketTable(raw, table.ipv6, port, own)
		if err != nil {
			return fmt.Errorf("ingress socket inventory invalid: %w", err)
		}
		if rows.other {
			return fmt.Errorf("ingress numeric port %d is already held by another live TCP listener", port)
		}
		seenOwn = seenOwn || rows.own
	}
	if !seenOwn {
		return errors.New("ingress listener identity was not observed in kernel inventory")
	}
	return nil
}

const maxPortInventoryBytes = 8 << 20

func readBoundedSocketTable(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, maxPortInventoryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(buf) > maxPortInventoryBytes {
		return nil, errors.New("ingress socket table exceeds inspection limit")
	}
	return buf, nil
}

type socketRows struct{ own, other bool }

func inspectSocketTable(raw []byte, ipv6 bool, port int, own uint64) (socketRows, error) {
	if len(raw) == 0 || len(raw) > maxPortInventoryBytes || raw[len(raw)-1] != '\n' {
		return socketRows{}, errors.New("socket inventory is missing, truncated or oversized")
	}
	lines := bytes.Split(raw, []byte{'\n'})
	header := strings.Fields(string(lines[0]))
	if len(header) < 4 || header[0] != "sl" || header[1] != "local_address" || (header[2] != "rem_address" && header[2] != "remote_address") || header[3] != "st" {
		return socketRows{}, errors.New("unexpected procfs TCP header")
	}
	if len(lines) > 65538 {
		return socketRows{}, errors.New("too many TCP rows")
	}
	var result socketRows
	addressSize := 8
	if ipv6 {
		addressSize = 32
	}
	for _, line := range lines[1:] {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		cols := strings.Fields(string(line))
		if len(cols) < 10 {
			return socketRows{}, errors.New("truncated TCP row")
		}
		state := cols[3]
		if len(state) != 2 {
			return socketRows{}, errors.New("invalid TCP state")
		}
		if _, err := strconv.ParseUint(state, 16, 8); err != nil {
			return socketRows{}, errors.New("invalid TCP state")
		}
		if state != "0A" {
			continue
		}
		local := strings.Split(cols[1], ":")
		if len(local) != 2 || len(local[0]) != addressSize || len(local[1]) != 4 {
			return socketRows{}, errors.New("malformed TCP listener endpoint")
		}
		if _, err := hex.DecodeString(local[0]); err != nil {
			return socketRows{}, errors.New("invalid listener IP")
		}
		p, err := strconv.ParseUint(local[1], 16, 16)
		if err != nil || p == 0 {
			return socketRows{}, errors.New("invalid listener port")
		}
		if int(p) != port {
			continue
		}
		inode, err := strconv.ParseUint(cols[9], 10, 64)
		if err != nil || inode == 0 {
			return socketRows{}, errors.New("invalid listener inode")
		}
		if inode == own {
			result.own = true
		} else {
			result.other = true
		}
	}
	return result, nil
}
