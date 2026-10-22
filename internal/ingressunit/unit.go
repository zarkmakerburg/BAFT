// Package ingressunit renders a candidate systemd listener unit from a
// caller-held prepared-manifest proof. Rendering does not install or start it.
package ingressunit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

var (
	listenerIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	serviceUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	binaryPathPattern  = regexp.MustCompile(`^/[A-Za-z0-9/._-]+$`)
)

type Candidate struct {
	Name   string
	Bytes  []byte
	SHA256 string
}

func canonicalEndpoint(raw string) (string, error) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return "", errors.New("ingress endpoint is not a literal IP and port")
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(portText)
	if ip == nil || err != nil || port < 1 || port > 65535 {
		return "", errors.New("ingress endpoint is not a valid literal IP and port")
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}

// Render returns the exact bytes a future owned-resource executor may stage.
// It refuses arbitrary paths and unverified manifests before constructing any
// unit content. The listener remains disabled until a separate signed apply.
func Render(spec ingressmanifest.Spec, proof ingressmanifest.Proof, listenerID, baftBinary, serviceUser string) (Candidate, error) {
	expected, err := ingressmanifest.Preview(spec)
	if err != nil || expected != proof {
		return Candidate{}, errors.New("ingress unit requires exact prepared manifest proof")
	}
	if !listenerIDPattern.MatchString(listenerID) || !serviceUserPattern.MatchString(serviceUser) || serviceUser == "root" {
		return Candidate{}, errors.New("invalid ingress listener identity or service user")
	}
	if !filepath.IsAbs(baftBinary) || filepath.Clean(baftBinary) != baftBinary || !binaryPathPattern.MatchString(baftBinary) {
		return Candidate{}, errors.New("ingress unit requires a safe absolute BAFT binary path")
	}
	// Identity values are copied into systemd comment and description lines.
	// Reject control characters even when the manifest proof matches.
	if !listenerIDPattern.MatchString(proof.TunnelID) || !listenerIDPattern.MatchString(proof.InstanceID) {
		return Candidate{}, errors.New("ingress manifest contains unsafe unit metadata identity")
	}
	listen, err := canonicalEndpoint(spec.Config.Listen)
	if err != nil {
		return Candidate{}, err
	}
	target, err := canonicalEndpoint(spec.Config.Target)
	if err != nil {
		return Candidate{}, err
	}
	clients := spec.Config.MaxClients
	if clients == 0 {
		clients = 128
	}
	dialMS := spec.Config.DialTimeoutMS
	if dialMS == 0 {
		dialMS = 3000
	}
	// Different listeners in one tunnel generation must never resolve to
	// the same systemd unit path: that would allow one listener to replace
	// another even when both carry valid proofs.
	// Bind the unit path to the exact manifest too: a changed configuration
	// must not overwrite an earlier candidate with the same owner tuple.
	owner := sha256.Sum256([]byte(proof.TunnelID + "\x00" + proof.InstanceID + "\x00" + strconv.FormatUint(proof.Generation, 10) + "\x00" + listenerID + "\x00" + proof.ManifestSHA256))
	name := "baft-ingress-" + hex.EncodeToString(owner[:16]) + ".service"
	content := fmt.Sprintf(`# baft-ingress-schema: 1
# baft-ingress-tunnel: %s
# baft-ingress-instance: %s
# baft-ingress-generation: %d
# baft-ingress-listener-id: %s
# baft-ingress-manifest-sha256: %s
[Unit]
Description=BAFT public ingress for %s
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
ExecStart=%s ingress run --listen %s --target %s --max-clients %d --dial-timeout %s --allow-public
Restart=on-failure
RestartSec=2s
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
UMask=0077

[Install]
WantedBy=multi-user.target
`, proof.TunnelID, proof.InstanceID, proof.Generation, listenerID, proof.ManifestSHA256, proof.TunnelID,
		serviceUser, serviceUser, baftBinary, listen, target, clients, time.Duration(dialMS)*time.Millisecond)
	raw := []byte(content)
	sum := sha256.Sum256(raw)
	return Candidate{Name: name, Bytes: raw, SHA256: hex.EncodeToString(sum[:])}, nil
}

// MatchesOwnedBytes refuses to adopt a unit modified outside the exact
// candidate. A matching text file still does not prove a live process owner.
func MatchesOwnedBytes(candidate Candidate, observed []byte) bool {
	return len(candidate.Bytes) != 0 && bytes.Equal(candidate.Bytes, observed)
}
