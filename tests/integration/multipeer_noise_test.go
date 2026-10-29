package integration_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/node"
	"github.com/zarkmakerburg/baft/internal/recordshape"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func TestNoiseListenerAcceptsAndSeparatesTwoIRPeers(t *testing.T) {
	certs := testPKI(t)
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ca := write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs.caDER}))
	cert := write("server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs.server.Certificate[0]}))
	der, err := x509.MarshalPKCS8PrivateKey(certs.server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tlsKey := write("server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	newTarget := func(marker byte) net.Listener {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					var request [1]byte
					for {
						if _, err := io.ReadFull(c, request[:]); err != nil {
							return
						}
						if _, err := c.Write([]byte{marker}); err != nil {
							return
						}
					}
				}(conn)
			}
		}()
		return ln
	}
	ownerTarget := newTarget('A')
	defer ownerTarget.Close()
	tenantTarget := newTarget('B')
	defer tenantTarget.Close()

	exKey, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ir1Key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ir2Key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	exPath := filepath.Join(dir, "ex-noise.json")
	ir1Path := filepath.Join(dir, "ir1-noise.json")
	ir2Path := filepath.Join(dir, "ir2-noise.json")
	for _, item := range []struct {
		path string
		key  securityinternal.KeyPair
	}{{exPath, exKey}, {ir1Path, ir1Key}, {ir2Path, ir2Key}} {
		if err := securityinternal.SaveKeyPair(item.path, item.key); err != nil {
			t.Fatal(err)
		}
	}
	exPub, _ := securityinternal.EncodePublicKey(exKey.Public)
	ir1Pub, _ := securityinternal.EncodePublicKey(ir1Key.Public)
	ir2Pub, _ := securityinternal.EncodePublicKey(ir2Key.Public)

	ex, err := config.LoadFile("../../configs/example-ex.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ex.Node.ID = "ex-shared"
	ex.Server.Listen = reserveAddress(t)
	ex.Server.ServerName = "ex.test"
	ex.Server.AllowedPeerIdentities = []string{"urn:baft:node:ir-master"}
	ex.Management.UnixSocket = filepath.Join(dir, "ex.sock")
	ex.Management.MetricsListen = reserveAddress(t)
	ex.Transport.Shards = 1
	ex.Routes[0].Target = ownerTarget.Addr().String()
	ex.Routes[0].AllowedPeers = []string{"urn:baft:node:ir-master"}
	ex.TLS = config.TLS{MinVersion: "1.3", CAFile: ca, CertFile: cert, KeyFile: tlsKey, SessionTickets: false}
	ex.Noise = &config.Noise{KeyFile: exPath, PeerPublicKey: ir1Pub, RecordShaping: recordshape.Config{}}

	// Prepare the tenant config separately so the test exercises the same live
	// reload path used by systemd ExecReload, not a listener restart.
	joinedEX, err := config.AddListenerTenant(ex, ex.Node.ID, ex.Server.Listen,
		"urn:baft:node:ir-worker", ir2Pub, "service-worker", tenantTarget.Addr().String())
	if err != nil {
		t.Fatalf("add second authenticated tenant: %v", err)
	}

	makeIR := func(id, keyPath, listen, metrics string) config.Config {
		ir, err := config.LoadFile("../../configs/example-ir.yaml")
		if err != nil {
			t.Fatal(err)
		}
		ir.Node.ID = id
		ir.Peer.Address = ex.Server.Listen
		ir.Peer.ServerName = "ex.test"
		ir.Peer.AllowedIdentity = "urn:baft:node:ex-shared"
		ir.Management.UnixSocket = filepath.Join(dir, id+".sock")
		ir.Management.MetricsListen = metrics
		ir.Transport.Shards = 1
		ir.Routes[0].Listen = listen
		ir.TLS = ex.TLS
		ir.Noise = &config.Noise{KeyFile: keyPath, PeerPublicKey: exPub, RecordShaping: recordshape.Config{}}
		return ir
	}
	ir1 := makeIR("ir-master", ir1Path, reserveAddress(t), reserveAddress(t))
	ir2 := makeIR("ir-worker", ir2Path, reserveAddress(t), reserveAddress(t))
	// The EX tenant route is named service-worker; bind the worker IR to that
	// exact route identity rather than the example config default.
	ir2.Routes[0].ID = "service-worker"
	ir2.Routes[0].RemoteRoute = "service-worker"
	if err := config.Validate(ex); err != nil {
		t.Fatalf("initial EX config: %v", err)
	}
	if err := config.Validate(joinedEX); err != nil {
		t.Fatalf("joined EX config: %v", err)
	}
	if err := config.Validate(ir1); err != nil {
		t.Fatalf("IR1 config: %v", err)
	}
	if err := config.Validate(ir2); err != nil {
		t.Fatalf("IR2 config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exRuntime := node.NewRuntime()
	exDone := make(chan error, 1)
	go func() { exDone <- exRuntime.Run(ctx, ex) }()
	waitTCP(t, ex.Server.Listen, time.Now().Add(8*time.Second))

	ir1Done := make(chan error, 1)
	go func() { ir1Done <- node.NewRuntime().Run(ctx, ir1) }()
	waitTCP(t, ir1.Routes[0].Listen, time.Now().Add(8*time.Second))

	liveConn, err := net.DialTimeout("tcp", ir1.Routes[0].Listen, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer liveConn.Close()
	_ = liveConn.SetDeadline(time.Now().Add(8 * time.Second))
	roundtripConn := func(label string, conn net.Conn, want byte) {
		t.Helper()
		if _, err := conn.Write([]byte{'x'}); err != nil {
			t.Fatalf("%s write: %v", label, err)
		}
		got := []byte{0}
		if _, err := io.ReadFull(conn, got); err != nil {
			t.Fatalf("%s read: %v", label, err)
		}
		if got[0] != want {
			t.Fatalf("%s reached wrong tenant target: got %q want %q", label, got[0], want)
		}
	}
	roundtripConn("existing tenant before reload", liveConn, 'A')

	var reload node.TenantReload
	deadline := time.Now().Add(3 * time.Second)
	for {
		reload, err = exRuntime.ReloadListener(joinedEX)
		if !errors.Is(err, node.ErrNoListener) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("add tenant with live listener reload: %v", err)
	}
	if len(reload.Added) != 1 || reload.Added[0] != "urn:baft:node:ir-worker" || reload.Routes != 2 {
		t.Fatalf("unexpected live reload receipt: %+v", reload)
	}
	// The same already-open flow must survive the tenant addition unchanged.
	roundtripConn("existing tenant after reload", liveConn, 'A')

	ir2Done := make(chan error, 1)
	go func() { ir2Done <- node.NewRuntime().Run(ctx, ir2) }()
	waitTCP(t, ir2.Routes[0].Listen, time.Now().Add(8*time.Second))
	// The local TCP listener is opened before the outbound authenticated Noise
	// session is necessarily ready. Prove the new tenant can complete a roundtrip
	// before asserting concurrent isolation; otherwise an immediate first dial can
	// race the initial handshake and report a misleading connection reset.
	tenantReadyDeadline := time.Now().Add(5 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", ir2.Routes[0].Listen, 200*time.Millisecond)
		if dialErr == nil {
			_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
			_, writeErr := conn.Write([]byte{'x'})
			got := []byte{0}
			_, readErr := io.ReadFull(conn, got)
			_ = conn.Close()
			if writeErr == nil && readErr == nil && got[0] == 'B' {
				break
			}
		}
		if time.Now().After(tenantReadyDeadline) {
			t.Fatal("new tenant route did not become ready for an authenticated roundtrip")
		}
		time.Sleep(25 * time.Millisecond)
	}

	roundtrip := func(label, addr string, want byte) {
		t.Helper()
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		roundtripConn(label, conn, want)
	}
	var wg sync.WaitGroup
	for _, tc := range []struct {
		label string
		addr  string
		want  byte
	}{
		{ir1.Node.ID, ir1.Routes[0].Listen, 'A'},
		{ir2.Node.ID, ir2.Routes[0].Listen, 'B'},
	} {
		wg.Add(1)
		go func(tc struct {
			label string
			addr  string
			want  byte
		}) {
			defer wg.Done()
			roundtrip(tc.label, tc.addr, tc.want)
		}(tc)
	}
	wg.Wait()
	t.Logf("PASS live tenant reload preserved the existing flow and isolated concurrent targets: master=%s→A worker=%s→B", ir1.Node.ID, ir2.Node.ID)

	cancel()
	for name, done := range map[string]<-chan error{"ex": exDone, "ir1": ir1Done, "ir2": ir2Done} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s runtime: %v", name, err)
			}
		case <-time.After(8 * time.Second):
			t.Fatalf("%s shutdown timed out", name)
		}
	}
}
