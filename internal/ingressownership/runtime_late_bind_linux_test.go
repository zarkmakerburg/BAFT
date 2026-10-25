//go:build linux

package ingressownership

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// The actual runtime may have ACKed a previously exclusive live-port claim.
// If a foreign process later binds another family to the same numeric port,
// that session must become stopped (rather than healthily idempotent), and
// exact-owner retirement must release its claim without touching the foreign.
func TestRuntimeLateForeignPortOwnerStopsAndCanRetireLinux(t *testing.T) {
	backend, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	port := nonEphemeralLoopbackPort(t, backend.Addr().(*net.TCPAddr).Port)
	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	spec := ingressmanifest.Spec{
		TunnelID: "watch-owner", Generation: 1,
		Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", port), Target: backend.Addr().String()},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Apply(ctx, spec, proof); err != nil {
		t.Fatalf("initial exclusive claim: %v", err)
	}
	// A successful real binary request/reply proves Serve was actually
	// accepting traffic before the foreign port owner appeared.
	client, err := net.DialTimeout("tcp4", spec.Config.Listen, time.Second)
	if err != nil {
		t.Fatalf("initial ingress unreachable: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	want := []byte{0, 255, 1, 128, 13, 10}
	if _, err := client.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("initial ingress failed: %v", err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("initial binary stream corrupted: %v", got)
		}
	}
	foreign, err := net.Listen("tcp6", net.JoinHostPort("::1", port))
	if err != nil {
		t.Skipf("OS disallows independent families: %v", err)
	}
	defer foreign.Close()

	deadline := time.Now().Add(4 * time.Second)
	for {
		err = r.Apply(context.Background(), spec, proof)
		if errors.Is(err, ErrStopped) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected replay error during watcher transition: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("foreign late bind was still reported as healthy runtime")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := r.Retire(spec.Config.Listen, proof); err == nil || !strings.Contains(err.Error(), "already held by another live TCP listener") {
		t.Fatalf("retirement must report the real stopped-listener conflict: %v", err)
	}
	if len(r.sessions) != 0 {
		t.Fatal("exact owner retirement leaked runtime session")
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client continued after ownership failure")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("foreign port conflict failed to interrupt active client")
	}
	// Rejection/retirement must never destroy the unrelated service.
	c, err := net.DialTimeout("tcp6", foreign.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("foreign port owner disrupted: %v", err)
	}
	defer c.Close()
	_ = foreign.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	ac, err := foreign.Accept()
	if err != nil {
		t.Fatalf("foreign service can no longer accept: %v", err)
	}
	ac.Close()
	foreign.Close()

	next := spec
	next.Generation++
	nextProof, err := ingressmanifest.Preview(next)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), next, nextProof); err != nil {
		t.Fatalf("exact retire did not release claim for next generation: %v", err)
	}
	if err = r.Retire(next.Config.Listen, nextProof); err != nil {
		t.Fatal(err)
	}
}

// nonEphemeralLoopbackPort returns a currently free 127.0.0.1 port below the
// kernel's ephemeral range. The test re-binds the same endpoint across
// generations; an ephemeral port could meanwhile be taken by another test
// process's outbound connection or :0 bind under parallel `go test ./...`.
func nonEphemeralLoopbackPort(t *testing.T, avoid int) string {
	t.Helper()
	low := 32768
	if raw, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range"); err == nil {
		if fields := strings.Fields(string(raw)); len(fields) == 2 {
			if n, err := strconv.Atoi(fields[0]); err == nil {
				low = n
			}
		}
	}
	base := 20000
	if low <= base+1000 {
		base = 1024
	}
	if low <= base+1000 {
		t.Fatalf("no non-ephemeral port range below %d", low)
	}
	for range 200 {
		p := base + rand.IntN(low-base)
		if p == avoid {
			continue
		}
		ln, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			continue
		}
		ln.Close()
		return strconv.Itoa(p)
	}
	t.Fatal("no free non-ephemeral loopback port")
	return ""
}
