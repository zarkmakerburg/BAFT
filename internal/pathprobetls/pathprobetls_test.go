package pathprobetls

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"
)

// A probe flow over the derived certificate: the browser-profile client
// completes TLS 1.3 against the listener, no SNI is sent for an IP, no fixed
// BAFT header is visible on the wire, and a different probe ID is refused.
func TestProbeTLSRoundTripHidesHeaderAndPinsProbe(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srvCfg, err := ServerConfig("pp-abc123", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		tc := tls.Server(&recorder{Conn: c, w: &wire}, srvCfg)
		_ = tc.SetDeadline(time.Now().Add(5 * time.Second))
		line := make([]byte, 22)
		if _, err := io.ReadFull(tc, line); err != nil {
			done <- err
			return
		}
		if tc.ConnectionState().Version != tls.VersionTLS13 || tc.ConnectionState().ServerName != "" {
			t.Errorf("version %x sni %q", tc.ConnectionState().Version, tc.ConnectionState().ServerName)
		}
		_, err = tc.Write([]byte("READY\n"))
		done <- err
	}()
	raw, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	c, err := Client(context.Background(), raw, "pp-abc123", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("BAFTP1 pp-abc123 4096\n")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 6)
	if _, err := io.ReadFull(c, got); err != nil || string(got) != "READY\n" {
		t.Fatalf("reply %q %v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire.Bytes(), []byte("BAFT")) || bytes.Contains(wire.Bytes(), []byte("pp-abc123")) {
		t.Fatal("probe header or probe ID visible on the wire")
	}

	// A listener for another probe is not trusted.
	ln2, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	other, _ := ServerConfig("pp-other", "127.0.0.1")
	go func() {
		c, err := ln2.Accept()
		if err == nil {
			_ = tls.Server(c, other).Handshake()
			c.Close()
		}
	}()
	raw2, err := net.Dial("tcp4", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw2.Close()
	if _, err := Client(context.Background(), raw2, "pp-abc123", "127.0.0.1"); err == nil {
		t.Fatal("a listener with another probe's key was trusted")
	}
}

func TestCertificateIsDerivedFromProbeID(t *testing.T) {
	_, a, err := Certificate("pp-1", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	_, b, _ := Certificate("pp-1", "192.0.2.10")
	_, c, _ := Certificate("pp-2", "192.0.2.10")
	if !bytes.Equal(a.Raw, b.Raw) {
		t.Fatal("same probe ID derived different certificates")
	}
	if a.PublicKey.(*ecdsa.PublicKey).Equal(c.PublicKey) {
		t.Fatal("different probe IDs derived the same key")
	}
	for _, bad := range [][2]string{{"", "192.0.2.10"}, {"pp-1", "not-an-ip"}} {
		if _, _, err := Certificate(bad[0], bad[1]); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

// recorder copies what the server reads from the wire.
type recorder struct {
	net.Conn
	w *bytes.Buffer
}

func (r *recorder) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	r.w.Write(p[:n])
	return n, err
}
