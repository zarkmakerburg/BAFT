package publicingress

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func tcpListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func launch(t *testing.T, target string, limit int) (*Forwarder, string, func()) {
	t.Helper()
	ln := tcpListener(t)
	f, err := New(Config{Listen: ln.Addr().String(), Target: target, MaxClients: limit, DialTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Serve(ctx, ln) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("shutdown: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Error("forwarder shutdown timed out")
			}
		})
	}
	t.Cleanup(stop)
	return f, ln.Addr().String(), stop
}

func connect(t *testing.T, address string) *net.TCPConn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tcp := conn.(*net.TCPConn)
	if err := tcp.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tcp.Close() })
	return tcp
}

func echoBackend(t *testing.T) net.Listener {
	t.Helper()
	ln := tcpListener(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	return ln
}

func TestForwarderPreservesBinaryTCP(t *testing.T) {
	backend := echoBackend(t)
	_, address, _ := launch(t, backend.Addr().String(), 2)
	client := connect(t, address)
	payload := bytes.Repeat([]byte{0, 255, 128, 13, 10, 1, 0, 17}, 8192)
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, got) {
		t.Fatal("binary stream changed")
	}
}

func TestForwarderWaitsForResponseAfterRequestHalfClose(t *testing.T) {
	backend := tcpListener(t)
	request := bytes.Repeat([]byte("request\x00"), 8192)
	response := bytes.Repeat([]byte("response\xff"), 32768)
	finished := make(chan error, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			finished <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		got, err := io.ReadAll(conn)
		if err == nil && !bytes.Equal(got, request) {
			err = fmt.Errorf("request mismatch")
		}
		if err == nil {
			_, err = conn.Write(response)
		}
		finished <- err
	}()
	_, address, _ := launch(t, backend.Addr().String(), 1)
	client := connect(t, address)
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, response) {
		t.Fatalf("response tail truncated: got=%d want=%d", len(got), len(response))
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestForwarderRemoteHalfCloseStillAllowsClientWrites(t *testing.T) {
	backend := tcpListener(t)
	received := make(chan string, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			received <- "accept failed"
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = io.WriteString(conn, "greeting")
		_ = conn.(*net.TCPConn).CloseWrite()
		got, err := io.ReadAll(conn)
		if err != nil {
			received <- err.Error()
			return
		}
		received <- string(got)
	}()
	_, address, _ := launch(t, backend.Addr().String(), 1)
	client := connect(t, address)
	greeting, err := io.ReadAll(client)
	if err != nil || string(greeting) != "greeting" {
		t.Fatalf("greeting: %q %v", greeting, err)
	}
	if _, err := io.WriteString(client, "after EOF"); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "after EOF" {
		t.Fatalf("reverse half close lost request: %q", got)
	}
}

func TestForwarderLimitsClientsAndReleasesCapacity(t *testing.T) {
	backend := echoBackend(t)
	f, address, _ := launch(t, backend.Addr().String(), 1)
	ping := func(c net.Conn) {
		t.Helper()
		if _, err := io.WriteString(c, "ping"); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, 4)
		if _, err := io.ReadFull(c, reply); err != nil || string(reply) != "ping" {
			t.Fatalf("echo: %q %v", reply, err)
		}
	}
	first := connect(t, address)
	ping(first)
	excess := connect(t, address)
	if _, err := excess.Read(make([]byte, 1)); err == nil {
		t.Fatal("excess connection remained active")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("excess connection was queued instead of rejected")
	}
	if f.ActiveConnections() != 1 || f.RejectedConnections() != 1 {
		t.Fatalf("incorrect capacity accounting: %d/%d", f.ActiveConnections(), f.RejectedConnections())
	}
	_ = first.Close()
	deadline := time.Now().Add(time.Second)
	for f.ActiveConnections() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.ActiveConnections() != 0 {
		t.Fatal("closed client retained capacity")
	}
	ping(connect(t, address))
}

func TestForwarderCancellationClosesClientsAndListener(t *testing.T) {
	backend := echoBackend(t)
	f, address, stop := launch(t, backend.Addr().String(), 2)
	client := connect(t, address)
	if _, err := io.WriteString(client, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	stop()
	if f.ActiveConnections() != 0 {
		t.Fatal("shutdown left active connections")
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client survived shutdown")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("shutdown did not close client")
	}
	// A recently closed accepted TCP connection can briefly retain the
	// address on Linux; the listener itself must become rebindable promptly.
	deadline := time.Now().Add(2 * time.Second)
	for {
		replacement, err := net.Listen("tcp4", address)
		if err == nil {
			_ = replacement.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("shutdown kept ingress port bound: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestForwarderBindConflictPreservesUnrelatedListener(t *testing.T) {
	unrelated := echoBackend(t)
	target := tcpListener(t)
	f, err := New(Config{Listen: unrelated.Addr().String(), Target: target.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Run(context.Background()); err == nil {
		t.Fatal("bound occupied port")
	}
	client := connect(t, unrelated.Addr().String())
	_, _ = io.WriteString(client, "unchanged")
	reply := make([]byte, 9)
	if _, err := io.ReadFull(client, reply); err != nil || string(reply) != "unchanged" {
		t.Fatalf("unrelated listener changed: %q %v", reply, err)
	}
}

func TestForwarderDialFailureReleasesClient(t *testing.T) {
	backend := tcpListener(t)
	target := backend.Addr().String()
	_ = backend.Close()
	f, address, _ := launch(t, target, 1)
	client := connect(t, address)
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("closed target accepted flow")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("failed dial left client hanging")
	}
	deadline := time.Now().Add(time.Second)
	for f.ActiveConnections() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.ActiveConnections() != 0 {
		t.Fatal("failed dial leaked capacity")
	}
}

func TestForwarderRejectsUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		listen, target string
		max            int
		timeout        time.Duration
	}{
		{listen: "127.0.0.1:22"}, {listen: "127.0.0.1:0"}, {listen: "127.0.0.1:65536"},
		{listen: "localhost:14443"}, {listen: ":14443"}, {listen: "224.0.0.1:14443"},
		{target: "localhost:1443"}, {target: "127.0.0.1:22"}, {target: "0.0.0.0:1443"}, {target: "192.0.2.1:1443"}, {target: "[::]:1443"}, {target: "127.0.0.1:0"},
		{target: "127.0.0.1:14443"}, {max: -1}, {max: 1025}, {timeout: time.Millisecond}, {timeout: 11 * time.Second},
	} {
		t.Run(fmt.Sprintf("%+v", tc), func(t *testing.T) {
			cfg := Config{Listen: "127.0.0.1:14443", Target: "127.0.0.1:1443", MaxClients: tc.max, DialTimeout: tc.timeout}
			if tc.listen != "" {
				cfg.Listen = tc.listen
			}
			if tc.target != "" {
				cfg.Target = tc.target
			}
			if _, err := New(cfg); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestForwarderMismatchedListenerIsUntouched(t *testing.T) {
	ln := echoBackend(t)
	f, err := New(Config{Listen: "127.0.0.1:14443", Target: "127.0.0.1:1443"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Serve(context.Background(), ln); err == nil {
		t.Fatal("accepted unrelated listener")
	}
	client := connect(t, ln.Addr().String())
	_, _ = io.WriteString(client, "x")
	if _, err := io.ReadFull(client, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
}

func TestForwarderAlreadyCanceledDoesNotBind(t *testing.T) {
	ln := tcpListener(t)
	f, err := New(Config{Listen: ln.Addr().String(), Target: "127.0.0.1:1443"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("canceled run attempted bind: %v", err)
	}
	if strings.Contains(f.cfg.Target, "localhost") {
		t.Fatal("unexpected hostname")
	}
}

func TestForwarderAcceptFailureReleasesListener(t *testing.T) {
	ln := tcpListener(t)
	address := ln.Addr().String()
	f, err := New(Config{Listen: address, Target: "127.0.0.1:1443"})
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if err := f.Serve(context.Background(), ln); err == nil {
		t.Fatal("closed listener did not fail")
	}
	replacement, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("failed serve kept port bound: %v", err)
	}
	_ = replacement.Close()
}

func TestForwarderRejectsSecondServeWithoutClosingItsListener(t *testing.T) {
	backend := echoBackend(t)
	first := tcpListener(t)
	address := first.Addr().String()
	f, err := New(Config{Listen: address, Target: backend.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Serve(ctx, first) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("first serve shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("first serve did not stop")
		}
	}()
	// A bound TCP socket accepts kernel connections before Serve starts. Wait
	// for the forwarder's ownership state before attempting the second Serve.
	deadline := time.Now().Add(3 * time.Second)
	for {
		f.mu.Lock()
		running := f.running
		f.mu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first serve did not start")
		}
		time.Sleep(time.Millisecond)
	}
	// A second Serve with the same already-owned listener must be refused,
	// without closing the listener used by the first Serve.
	if err := f.Serve(ctx, first); err == nil {
		t.Fatal("concurrent Serve accepted")
	}
	c := connect(t, address)
	defer c.Close()
	if _, err := io.WriteString(c, "ok"); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(c, response); err != nil || string(response) != "ok" {
		t.Fatalf("rejected Serve disrupted active listener: %q, %v", response, err)
	}
}

func TestForwarderDoesNotTakeOwnershipOfMismatchedListener(t *testing.T) {
	backend := echoBackend(t)
	unrelated := echoBackend(t)
	f, err := New(Config{Listen: unrelated.Addr().String(), Target: backend.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	// Supply a different listener: the forwarder must not close or alter it.
	other := tcpListener(t)
	if err := f.Serve(context.Background(), other); err == nil {
		t.Fatal("mismatched listener accepted")
	}
	// It must remain open and able to accept a new connection.
	client := connect(t, other.Addr().String())
	defer client.Close()
	if _, err := io.WriteString(client, "still-open"); err != nil {
		t.Fatal(err)
	}
	accepted, err := other.Accept()
	if err != nil {
		t.Fatalf("rejected listener was closed: %v", err)
	}
	defer accepted.Close()
	data := make([]byte, len("still-open"))
	if _, err := io.ReadFull(accepted, data); err != nil || string(data) != "still-open" {
		t.Fatalf("rejected listener was altered: %q %v", data, err)
	}
}
