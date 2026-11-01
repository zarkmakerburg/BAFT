// Package pathprobetls wraps a path-probe flow (connection test, discovery,
// speed flow) in TLS 1.3, so that no fixed BAFT header ("BAFTP1 <id> <n>")
// crosses the Iran<->abroad path in the clear (path-probe-tls-v1).
//
// Both agents derive the same P-256 key from the probe ID, which only BCC and
// the two agents know (it arrives in BCC-signed jobs and, with TLS, is never
// on the wire), and a self-signed certificate for the destination IP. The
// listener serves it; the source trusts only that key (verification stays on,
// no InsecureSkipVerify), so a third party cannot terminate the probe. P-256
// rather than Ed25519 because browser ClientHellos do not offer Ed25519.
//
// What a passive observer sees: a browser-fidelity ClientHello (Chrome
// profile via utls) with no SNI, as for any HTTPS-by-IP connection, the
// usual h2/http/1.1 ALPN offer, and an encrypted TLS 1.3 session; the
// certificate itself is encrypted in TLS 1.3.
package pathprobetls

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"time"

	"github.com/zarkmakerburg/baft/internal/carrier/utlsdial"
)

// Capability is what an agent advertises when it speaks TLS probe flows.
const Capability = "path-probe-tls-v1"

var alpn = []string{"h2", "http/1.1"}

// Certificate derives the probe certificate for probeID and the destination
// IP the source dials. Both sides compute the same key and certificate.
func Certificate(probeID, serverIP string) (tls.Certificate, *x509.Certificate, error) {
	ip := net.ParseIP(serverIP)
	if probeID == "" || ip == nil {
		return tls.Certificate{}, nil, errors.New("pathprobetls: probe id and server IP are required")
	}
	seed := sha256.Sum256([]byte("baft path-probe tls v1\x00" + probeID))
	key, err := deriveKey(seed)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serial := sha256.Sum256(append([]byte("serial\x00"), seed[:]...))
	tmpl := &x509.Certificate{
		SerialNumber:          new(big.Int).SetBytes(serial[:16]),
		Subject:               pkix.Name{CommonName: serverIP},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{ip},
	}
	// A nil random source makes the ECDSA signature deterministic (RFC 6979),
	// so both sides build byte-identical certificates.
	der, err := x509.CreateCertificate(nil, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf, nil
}

// deriveKey maps the seed to a valid P-256 scalar (rejection sampling on a
// counter) and builds the key pair from it.
func deriveKey(seed [32]byte) (*ecdsa.PrivateKey, error) {
	for i := byte(0); i < 16; i++ {
		d := sha256.Sum256(append(append([]byte("key\x00"), seed[:]...), i))
		priv, err := ecdh.P256().NewPrivateKey(d[:])
		if err != nil {
			continue // zero or not below the group order: next counter
		}
		pub := priv.PublicKey().Bytes() // 0x04 || X || Y
		return &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])},
			D:         new(big.Int).SetBytes(d[:]),
		}, nil
	}
	return nil, errors.New("pathprobetls: no valid key derived")
}

// ServerConfig is the listener side: TLS 1.3 only, the derived certificate.
func ServerConfig(probeID, serverIP string) (*tls.Config, error) {
	cert, _, err := Certificate(probeID, serverIP)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, NextProtos: alpn}, nil
}

// Client runs the browser-fidelity TLS handshake over an already connected
// raw TCP connection to serverIP and trusts only the derived certificate.
// The caller owns raw and must close it on error.
func Client(ctx context.Context, raw net.Conn, probeID, serverIP string) (net.Conn, error) {
	_, leaf, err := Certificate(probeID, serverIP)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return utlsdial.Handshake(ctx, raw, utlsdial.Config{
		ServerName: serverIP, RootCAs: pool, NextProtos: alpn,
		Pin: func(got *x509.Certificate) error {
			want, ok := leaf.PublicKey.(*ecdsa.PublicKey)
			if pk, isEC := got.PublicKey.(*ecdsa.PublicKey); !ok || !isEC || !pk.Equal(want) {
				return errors.New("pathprobetls: unexpected probe key")
			}
			return nil
		},
	})
}
