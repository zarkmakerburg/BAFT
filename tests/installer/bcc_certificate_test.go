package installer_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
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
	return bccTestCertificateKind(t, dir, name, expires, "ec")
}

func bccTestCertificateKind(t *testing.T, dir, name string, expires time.Time, kind string) (string, string) {
	t.Helper()
	var key crypto.Signer
	var err error
	if kind == "rsa" {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ignored.invalid"}, DNSNames: []string{"bcc.example.test"}, IPAddresses: []net.IP{net.ParseIP("192.0.2.10")}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if kind == "cn-only" {
		tpl.Subject.CommonName = "bcc.example.test"
		tpl.DNSNames, tpl.IPAddresses = nil, nil
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
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
	cnOnly, cnOnlyKey := bccTestCertificateKind(t, dir, "cn-only", time.Now().Add(72*time.Hour), "cn-only")
	rsaCert, rsaKey := bccTestCertificateKind(t, dir, "rsa", time.Now().Add(72*time.Hour), "rsa")
	functions := bccSSLFunctions(t, "bcc_host_kind", "bcc_cert_ok")
	for _, tc := range []struct {
		name, cert, key, host string
		ok                    bool
	}{
		{"valid-domain", cert, key, "bcc.example.test", true},
		{"valid-ip", cert, key, "192.0.2.10", true},
		{"valid-rsa", rsaCert, rsaKey, "bcc.example.test", true},
		{"cn-only-rejected", cnOnly, cnOnlyKey, "bcc.example.test", false},
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
		answer                     string
	}{
		{name: "valid", valid: true},
		{name: "mismatched-key"},
		{name: "existing-pair-invalid-install", existing: true},
		{name: "existing-pair-issuance-failure", existing: true, fail: "issue"},
		{name: "existing-pair-install-failure", existing: true, fail: "install"},
		{name: "timer-failure-honest", valid: true, timerFail: true},
		{name: "selfsigned-eof-preserves-old-pair", existing: true, answer: "eof"},
		{name: "selfsigned-default-preserves-old-pair", existing: true, answer: "empty"},
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
				// A restrictive host umask must not change the fixture's expected modes.
				if err := os.Chmod(activeCert, 0640); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(activeKey, 0600); err != nil {
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
ui_ask_prompt(){ :; }
ask(){ [[ "$TEST_ANSWER" != eof ]] || return 1; printf -v "$1" '%s' ''; }
bcc_install_acme(){ :; }; bcc_port_busy(){ return 1; }; bcc_public_ip(){ echo 192.0.2.10; }; getent(){ echo '192.0.2.10 STREAM bcc.example.test'; }
bcc_install_renew_timer(){ echo TIMER; [[ "$TEST_TIMER_FAIL" != true ]]; }
bcc_obtain_cert
`
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+harness)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TEST_CERT="+cert, "TEST_KEY="+key, "TEST_FAIL="+tc.fail, "TEST_TIMER_FAIL="+map[bool]string{true: "true", false: "false"}[tc.timerFail], "BAFT_BCC_TLS_CERT="+activeCert, "BAFT_BCC_TLS_KEY="+activeKey, "BAFT_BCC_TLS_DIR="+tlsDir, "BAFT_ACME_HOME="+bin, "BAFT_BCC_DOMAIN=bcc.example.test", "BCC_WANT_CERT=1", "ASK_TTY=0")
			if tc.answer != "" {
				cmd.Env = append(cmd.Env, "ASK_TTY=1", "TEST_ANSWER="+tc.answer)
			}
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

func TestBCCACMEArchiveVerifiedBeforeExtraction(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar unavailable")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum unavailable")
	}
	functions := bccSSLFunctions(t, "bcc_install_acme")
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "checksum-mismatch"}[valid], func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			root := filepath.Join(source, "acme.sh-test-commit")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			fixture := `#!/bin/bash
mkdir -p "$3"
cp "$0" "$3/acme.sh"
echo EXECUTED > "$TEST_EXEC_MARKER"
`
			if err := os.WriteFile(filepath.Join(root, "acme.sh"), []byte(fixture), 0700); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(dir, "archive.tar.gz")
			if out, err := exec.Command("tar", "-czf", archive, "-C", source, "acme.sh-test-commit").CombinedOutput(); err != nil {
				t.Fatalf("fixture archive: %v %s", err, out)
			}
			raw, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(raw))
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			marker := filepath.Join(dir, "executed")
			harness := `
log(){ :; }; warn(){ echo "$*"; }
curl(){ cp "$TEST_ARCHIVE" "${@: -1}"; }
tar(){ echo EXTRACTED; command tar "$@"; }
CLEANUP=()
trap 'rm -rf -- "${CLEANUP[@]}"' EXIT
bcc_install_acme
`
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+harness)
			cmd.Env = append(os.Environ(), "BAFT_ACME_HOME="+filepath.Join(dir, "installed"), "BAFT_ACME_VERSION=3.1.1", "BAFT_ACME_COMMIT=test-commit", "BAFT_ACME_SHA256="+digest, "TEST_ARCHIVE="+archive, "TEST_EXEC_MARKER="+marker)
			out, err := cmd.CombinedOutput()
			if (err == nil) != valid || strings.Contains(string(out), "EXTRACTED") != valid {
				t.Fatalf("valid=%v err=%v output=%s", valid, err, out)
			}
			_, statErr := os.Stat(marker)
			if (statErr == nil) != valid {
				t.Fatalf("unexpected downloaded code execution: valid=%v err=%v", valid, statErr)
			}
		})
	}
}

func TestBCCUnsupportedIPCertificateDoesNotUpgradeClient(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "acme-calls")
	for name, body := range map[string]string{"acme.sh": "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_CALLS\"\nexit 0\n", "socat": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	functions := bccSSLFunctions(t, "bcc_host_kind", "bcc_cert_ok", "bcc_obtain_cert")
	harness := `
phase(){ :; }; log(){ :; }; warn(){ echo "$*"; }; die(){ echo "$*"; exit 1; }
install(){ mkdir -p "$BAFT_BCC_TLS_DIR"; }
bcc_install_acme(){ :; }; bcc_port_busy(){ return 1; }
bcc_obtain_cert
`
	cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+harness)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TEST_CALLS="+marker, "BAFT_BCC_TLS_CERT="+filepath.Join(dir, "tls/cert.pem"), "BAFT_BCC_TLS_KEY="+filepath.Join(dir, "tls/key.pem"), "BAFT_BCC_TLS_DIR="+filepath.Join(dir, "tls"), "BAFT_ACME_HOME="+bin, "BAFT_BCC_DOMAIN=192.0.2.10", "BCC_WANT_CERT=1", "ASK_TTY=0")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "automatic unverified upgrade is disabled") {
		t.Fatalf("expected actionable failure: err=%v output=%s", err, out)
	}
	calls, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "--help\n" {
		t.Fatalf("unsupported client was mutated: %s", calls)
	}
}

func TestBCCRenewTimerWriteFailureDoesNotEnableTimer(t *testing.T) {
	functions := bccSSLFunctions(t, "bcc_install_renew_timer")
	for _, blocked := range []string{"baft-bcc-cert.service", "baft-bcc-cert.timer"} {
		t.Run(blocked, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, blocked), 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dir, "systemctl-called")
			harness := `
systemctl(){ echo CALLED > "$TEST_SYSTEMCTL_MARKER"; }
# Match the production conditional caller: errexit is suppressed inside it.
if bcc_install_renew_timer; then exit 0; else exit 7; fi
`
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+functions+harness)
			cmd.Env = append(os.Environ(), "BAFT_SYSTEMD_DIR="+dir, "BAFT_ACME_HOME="+filepath.Join(dir, "acme"), "TEST_SYSTEMCTL_MARKER="+marker)
			out, err := cmd.CombinedOutput()
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 7 {
				t.Fatalf("expected rejected renewal setup: err=%v output=%s", err, out)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("systemctl ran after failed unit write: %v", err)
			}
		})
	}
}
