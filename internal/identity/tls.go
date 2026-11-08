package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

func LoadCertPool(path string) (*x509.CertPool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, errors.New("no CA certificate found")
	}
	return pool, nil
}

func LoadKeyPair(certFile, keyFile string) (tls.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	if block, _ := pem.Decode(keyPEM); block == nil {
		return tls.Certificate{}, errors.New("invalid private key PEM")
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

func ClientTLS(ca *x509.CertPool, cert tls.Certificate, serverName string) (*tls.Config, error) {
	if ca == nil || serverName == "" {
		return nil, errors.New("CA pool and server name are required")
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		RootCAs:            ca,
		Certificates:       []tls.Certificate{cert},
		ServerName:         serverName,
		InsecureSkipVerify: false,
		NextProtos:         []string{"h2"},
		ClientSessionCache: nil,
	}, nil
}

// RevocationChecker is the part of a RevocationSet a dialer needs.
type RevocationChecker interface {
	IsRevoked(identity, serial, fingerprint string) bool
}

// ApplyServerChecks makes a dialer refuse a listener certificate that is
// revoked, or that differs from an optional exact pin. It runs after the chain
// has been verified against the CA, on every handshake including recovery
// redials. pinSHA256 is the lowercase hex SHA-256 of the DER certificate, or
// empty for no pin. Revocation is checked against the same list format that
// listeners use; an established carrier is not cancelled, only new handshakes
// are refused.
func ApplyServerChecks(cfg *tls.Config, revocations RevocationChecker, pinSHA256 string) error {
	var pin []byte
	if pinSHA256 != "" {
		b, err := hex.DecodeString(pinSHA256)
		if err != nil || len(b) != sha256.Size {
			return errors.New("peer certificate pin must be 64 hex characters")
		}
		pin = b
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.VerifiedChains) == 0 || len(cs.PeerCertificates) == 0 {
			return errors.New("server certificate was not verified")
		}
		leaf := cs.PeerCertificates[0]
		sum := sha256.Sum256(leaf.Raw)
		if pin != nil && subtle.ConstantTimeCompare(sum[:], pin) != 1 {
			return errors.New("server certificate does not match the pinned fingerprint")
		}
		if revocations != nil {
			id := ""
			if len(leaf.URIs) == 1 {
				id = leaf.URIs[0].String()
			}
			if revocations.IsRevoked(id, leaf.SerialNumber.Text(16), hex.EncodeToString(sum[:])) {
				return errors.New("server certificate is revoked")
			}
		}
		return nil
	}
	return nil
}

func ServerTLS(ca *x509.CertPool, cert tls.Certificate, allowedURIs map[string]struct{}) (*tls.Config, error) {
	if ca == nil || len(allowedURIs) == 0 {
		return nil, errors.New("client CA pool and peer allowlist are required")
	}
	cfg := &tls.Config{
		MinVersion:             tls.VersionTLS13,
		ClientAuth:             tls.RequireAndVerifyClientCert,
		ClientCAs:              ca,
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"h2"},
		SessionTicketsDisabled: true,
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.VerifiedChains) == 0 || len(cs.PeerCertificates) == 0 {
			return errors.New("peer certificate was not verified")
		}
		leaf := cs.PeerCertificates[0]
		if len(leaf.URIs) != 1 {
			return errors.New("peer certificate must contain exactly one URI SAN identity")
		}
		if _, ok := allowedURIs[leaf.URIs[0].String()]; ok {
			return nil
		}
		return fmt.Errorf("peer identity is not authorized")
	}
	return cfg, nil
}
