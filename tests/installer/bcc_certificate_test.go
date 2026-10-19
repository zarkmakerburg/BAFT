package installer_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bccSSLFunctions(t *testing.T, names ...string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, name := range names {
		start := strings.Index(string(raw), "\n"+name+"() {")
		if start < 0 {
			t.Fatalf("missing function %s", name)
		}
		body := string(raw)[start+1:]
		end := strings.Index(body, "\n}")
		if end < 0 {
			t.Fatalf("unterminated function %s", name)
		}
		out.WriteString(body[:end+2])
		out.WriteByte('\n')
	}
	return out.String()
}

func bccTestCertificate(t *testing.T, dir, name string, expires time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ignored.invalid"}, DNSNames: []string{"bcc.example.test"}, IPAddresses: []net.IP{net.ParseIP("192.0.2.10")}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := filepath.Join(dir, name+".pem"), filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestBCCCertificateAcceptance(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl unavailable")
	}
	dir := t.TempDir()
	cert, key := bccTestCertificate(t, dir, "valid", time.Now().Add(72*time.Hour))
	_, otherKey := bccTestCertificate(t, dir, "other", time.Now().Add(72*time.Hour))
	expired, expiredKey := bccTestCertificate(t, dir, "expired", time.Now().Add(-time.Hour))
	soon, soonKey := bccTestCertificate(t, dir, "soon", time.Now().Add(time.Hour))
	functions := bccSSLFunctions(t, "bcc_host_kind", "bcc_cert_ok")
	for _, tc := range []struct {
		name, cert, key, host string
		ok                    bool
	}{
		{"valid-domain", cert, key, "bcc.example.test", true},
		{"valid-ip", cert, key, "192.0.2.10", true},
		{"mismatched-key", cert, otherKey, "bcc.example.test", false},
		{"wrong-san", cert, key, "other.example.test", false},
		{"wrong-ip", cert, key, "192.0.2.11", false},
		{"expired", expired, expiredKey, "bcc.example.test", false},
		{"expires-too-soon", soon, soonKey, "bcc.example.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+"\nbcc_cert_ok")
			cmd.Env = append(os.Environ(), "BAFT_BCC_TLS_CERT="+tc.cert, "BAFT_BCC_TLS_KEY="+tc.key, "BAFT_BCC_DOMAIN="+tc.host)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%v want %v: %s", err == nil, tc.ok, out)
			}
		})
	}
}

func TestBCCACMEInstallValidatesBeforeSuccess(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl unavailable")
	}
	functions := bccSSLFunctions(t, "bcc_host_kind", "bcc_cert_ok", "bcc_obtain_cert")
	for _, tc := range []struct {
		name                       string
		valid, existing, timerFail bool
		fail                       string
	}{
		{name: "valid", valid: true},
		{name: "mismatched-key"},
		{name: "existing-pair-invalid-install", existing: true},
		{name: "existing-pair-issuance-failure", existing: true, fail: "issue"},
		{name: "existing-pair-install-failure", existing: true, fail: "install"},
		{name: "timer-failure-honest", valid: true, timerFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cert, key := bccTestCertificate(t, dir, "issued", time.Now().Add(72*time.Hour))
			if !tc.valid {
				_, key = bccTestCertificate(t, dir, "wrong", time.Now().Add(72*time.Hour))
			}
			tlsDir := filepath.Join(dir, "tls")
			if err := os.Mkdir(tlsDir, 0700); err != nil {
				t.Fatal(err)
			}
			activeCert, activeKey := filepath.Join(tlsDir, "fullchain.pem"), filepath.Join(tlsDir, "key.pem")
			var oldCert, oldKey []byte
			if tc.existing {
				oldC, oldK := bccTestCertificate(t, dir, "old", time.Now().Add(-time.Hour))
				oldCert, _ = os.ReadFile(oldC)
				oldKey, _ = os.ReadFile(oldK)
				if err := os.WriteFile(activeCert, oldCert, 0640); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(activeKey, oldKey, 0600); err != nil {
					t.Fatal(err)
				}
			}
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			acme := `#!/bin/bash
if [[ " $* " == *" --install-cert "* ]]; then
 cp "$TEST_CERT" "$BAFT_BCC_TLS_CERT"
 cp "$TEST_KEY" "$BAFT_BCC_TLS_KEY"
 chmod 0644 "$BAFT_BCC_TLS_CERT" "$BAFT_BCC_TLS_KEY"
 [[ "$TEST_FAIL" != install ]] || exit 1
else
 [[ "$TEST_FAIL" != issue ]] || exit 1
fi
exit 0
`
			for name, body := range map[string]string{"acme.sh": acme, "socat": "#!/bin/sh\nexit 0\n"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			harness := `
phase(){ :; }; log(){ :; }; warn(){ echo "WARN $*"; }; ok(){ echo "SUCCESS $*"; }; die(){ echo "FAILED $*"; exit 1; }
install(){ mkdir -p "$BAFT_BCC_TLS_DIR"; }
bcc_install_acme(){ :; }; bcc_port_busy(){ return 1; }; bcc_public_ip(){ echo 192.0.2.10; }; getent(){ echo '192.0.2.10 STREAM bcc.example.test'; }
bcc_install_renew_timer(){ echo TIMER; [[ "$TEST_TIMER_FAIL" != true ]]; }
bcc_obtain_cert
`
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+harness)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TEST_CERT="+cert, "TEST_KEY="+key, "TEST_FAIL="+tc.fail, "TEST_TIMER_FAIL="+map[bool]string{true: "true", false: "false"}[tc.timerFail], "BAFT_BCC_TLS_CERT="+activeCert, "BAFT_BCC_TLS_KEY="+activeKey, "BAFT_BCC_TLS_DIR="+tlsDir, "BAFT_ACME_HOME="+bin, "BAFT_BCC_DOMAIN=bcc.example.test", "BCC_WANT_CERT=1", "ASK_TTY=0")
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.valid || strings.Contains(string(out), "SUCCESS") != tc.valid || strings.Contains(string(out), "TIMER") != tc.valid {
				t.Fatalf("valid=%v err=%v output=%s", tc.valid, err, out)
			}
			if tc.timerFail && (strings.Contains(string(out), "renews automatically") || !strings.Contains(string(out), "automatic renewal is not confirmed")) {
				t.Fatalf("misleading renewal result: %s", out)
			}
			if !tc.valid {
				for _, f := range []struct {
					path string
					old  []byte
					mode os.FileMode
				}{{activeCert, oldCert, 0640}, {activeKey, oldKey, 0600}} {
					got, readErr := os.ReadFile(f.path)
					if tc.existing {
						info, statErr := os.Stat(f.path)
						if readErr != nil || statErr != nil || string(got) != string(f.old) || info.Mode().Perm() != f.mode {
							t.Fatalf("previous file not restored: %s", f.path)
						}
					} else if !os.IsNotExist(readErr) {
						t.Fatalf("new invalid file remains: %s", f.path)
					}
				}
			}
			snapshots, _ := filepath.Glob(filepath.Join(tlsDir, ".previous.*"))
			if len(snapshots) != 0 {
				t.Fatal("certificate rollback snapshots were not cleaned")
			}
		})
	}
}
