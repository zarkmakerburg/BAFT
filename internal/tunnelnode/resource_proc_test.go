//go:build linux

package tunnelnode

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// A long-lived controlled child distinguishes missing/unmapped procfs access
// from a BAFT lifecycle failure. Do not disable the production ownership gate.
func requireProcessResourceEvidence(t *testing.T) {
	t.Helper()
	cmd := exec.Command("cat")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); cmd.Wait() }()
	_, err = serviceSocketInodes(context.Background(), strconv.Itoa(cmd.Process.Pid))
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		if os.Getenv("BAFT_REQUIRE_RESOURCE_EVIDENCE") == "1" {
			t.Fatalf("required process-resource acceptance environment is unavailable: %v", err)
		}
		t.Skipf("environment cannot inspect controlled child process namespace/FD ownership: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcTCPParsingDoesNotInferUnknownAsFree(t *testing.T) {
	header := "sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode\n"
	row := func(addr, state, inode string) string {
		return "0: " + addr + " 00000000:0000 " + state + " 0:0 0:0 0 0 0 " + inode + "\n"
	}
	for _, tc := range []struct {
		addr string
		v6   bool
		want string
	}{
		{"0100007F:1F90", false, "127.0.0.1:8080"},
		{"00000000:1F90", false, "0.0.0.0:8080"},
		{"00000000000000000000000001000000:1F90", true, "[::1]:8080"},
		{"00000000000000000000000000000000:1F90", true, "[::]:8080"},
	} {
		got, err := parseProcTCP([]byte(header+row(tc.addr, "0A", "123")), tc.v6)
		if err != nil || len(got) != 1 || got[0].Address != tc.want || got[0].Inode != 123 {
			t.Fatalf("unexpected inventory: %+v %v", got, err)
		}
	}
	for _, b := range []string{"", header + "short\n", header + row("0100007F:1F90", "XX", "123"), header + row("0100007F:0000", "0A", "123"), header + row("0100007F:1F90", "0A", "0"), header + row("bad:1F90", "0A", "123")} {
		if _, err := parseProcTCP([]byte(b), false); err == nil {
			t.Fatal("invalid evidence was interpreted as complete inventory")
		}
	}
	got, err := parseProcTCP([]byte(header+row("0100007F:1F90", "01", "123")), false)
	if err != nil || len(got) != 0 {
		t.Fatal("established TCP connection was counted as listener")
	}
}

func TestPassiveInventoryFindsLiveSocketAndExactOwner(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sockets, err := hostTCPInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owned, err := serviceSocketInodes(context.Background(), strconv.Itoa(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sockets {
		if s.Address == l.Addr().String() {
			if !owned[s.Inode] {
				t.Fatal("live socket was not attributed to its actual FD owner")
			}
			return
		}
	}
	t.Fatal("live listener absent from passive inventory")
}

func TestResourceBindOverlapHandlesWildcardAndAddressFamilies(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.0.0.0:8080", "[::1]:8080", true},
		{"[::]:8080", "127.0.0.1:8080", true},
		{"127.0.0.1:08080", "127.0.0.1:8080", true},
		{"127.0.0.1:8080", "127.0.0.2:8080", false},
		{"[::1]:8080", "127.0.0.1:8080", false},
		{"[::1]:8080", "[::1]:8081", false},
	} {
		got, err := resourceBindsOverlap(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%s/%s: %v %v", tc.a, tc.b, got, err)
		}
	}
	if _, err := resourceBindsOverlap("localhost:8080", "127.0.0.1:8080"); err == nil {
		t.Fatal("unknown bind address accepted")
	}
	if _, err := serviceSocketInodes(context.Background(), "1/../../self"); err == nil || strings.Contains(err.Error(), "readlink") {
		t.Fatal("untrusted PID reached filesystem traversal")
	}
}

func TestResourceUnixListenerDoesNotConfuseAcceptedClientsWithBindOwner(t *testing.T) {
	header := "Num RefCount Protocol Flags Type St Inode Path\n"
	listener := "0: 00000002 00000000 00010000 0001 01 100 /var/lib/baft/admin.sock\n"
	client := "1: 00000003 00000000 00000000 0001 03 200 /var/lib/baft/admin.sock\n"
	for _, b := range []string{header + listener, header + client + listener, header + listener + client} {
		n, err := parseProcUnixListener([]byte(b), "/var/lib/baft/admin.sock")
		if err != nil || n != 100 {
			t.Fatalf("accepted client changed bind ownership: %d %v", n, err)
		}
	}
	for _, b := range []string{header + client, header + listener + strings.ReplaceAll(listener, "100 ", "300 "), header + strings.ReplaceAll(listener, "00010000", "bad-flags")} {
		if _, err := parseProcUnixListener([]byte(b), "/var/lib/baft/admin.sock"); err == nil {
			t.Fatal("unavailable/ambiguous listener evidence was accepted")
		}
	}
}

func TestResourceUnixListenerEvidenceWithActiveManagementClient(t *testing.T) {
	path := t.TempDir() + "/admin.sock"
	l, err := net.Listen("unix", path)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			if os.Getenv("BAFT_REQUIRE_RESOURCE_EVIDENCE") == "1" {
				t.Fatalf("required Unix client acceptance environment unavailable: %v", err)
			}
			t.Skipf("environment cannot create Unix sockets: %v", err)
		}
		t.Fatal(err)
	}
	defer l.Close()
	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	accepted, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	ctx := context.Background()
	inode, err := hostUnixSocketInode(ctx, path)
	if err != nil {
		t.Fatalf("active management client confused listener identity: %v", err)
	}
	owned, err := serviceSocketInodes(ctx, strconv.Itoa(os.Getpid()))
	if err != nil || !owned[inode] {
		t.Fatalf("listener owner unavailable with client connected: %v", err)
	}
}
