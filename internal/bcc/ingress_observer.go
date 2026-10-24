package bcc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// ingressProbePrefix frames the observer payload. The nonce is fresh random
// 128-bit data per probe, so a recorded or canned reply can never verify.
const ingressProbePrefix = "BAFT-INGRESS-PROBE/1 "

const ingressProbeTimeout = 5 * time.Second

// IngressProbeResult is the strict, closed outcome of one BCC-side probe. It
// carries no free-form remote data: only booleans and a bounded reason.
type IngressProbeResult struct {
	Target          string `json:"target"`
	Connected       bool   `json:"connected"`
	PayloadVerified bool   `json:"payload_verified"`
	Failure         string `json:"failure,omitempty"`
}

// IngressObserver makes BCC's own TCP connection to the public ingress
// endpoint. It never follows redirects (it is raw TCP), dials exactly once and
// never retries, so a failure cannot be hidden. All fields are unexported: no
// configuration surface can weaken it in production.
type IngressObserver struct {
	timeout       time.Duration
	dial          func(ctx context.Context, network, addr string) (net.Conn, error)
	allowLoopback bool // tests only; unreachable outside the package
	nonce         func() (string, error)
}

// NewIngressObserver returns the production observer.
func NewIngressObserver() *IngressObserver {
	d := &net.Dialer{FallbackDelay: -1} // single attempt, no happy-eyeballs retry
	return &IngressObserver{timeout: ingressProbeTimeout, dial: d.DialContext, nonce: randomProbeNonce}
}

func randomProbeNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// publicProbeHost extracts the host of the IR node's address and refuses
// anything that is not a plausible public endpoint.
func (o *IngressObserver) publicProbeHost(address string) (string, error) {
	address = strings.TrimSpace(address)
	host := address
	if h, p, err := net.SplitHostPort(address); err == nil {
		if n, perr := strconv.Atoi(p); perr != nil || n < 1 || n > 65535 {
			return "", errors.New("IR node address has an invalid port")
		}
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" || len(host) > 253 {
		return "", errors.New("IR node address missing")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if err := o.checkProbeIP(ip); err != nil {
			return "", err
		}
		return host, nil
	}
	for i := 0; i < len(host); i++ {
		c := host[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			return "", errors.New("IR node address is not a valid host")
		}
	}
	if !o.allowLoopback && (strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost")) {
		return "", errors.New("IR node address is not public")
	}
	return host, nil
}

func (o *IngressObserver) checkProbeIP(ip netip.Addr) error {
	ip = ip.Unmap()
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || (ip.IsLoopback() && !o.allowLoopback) {
		return errors.New("IR node address is not a public unicast address")
	}
	return nil
}

// Observe probes the public ingress of an applied, receipt-backed operation
// and, only if a fresh random nonce survives a real TCP round trip, returns the
// IndependentIngressObservation built from the probe plus the authenticated
// receipt's evidence. Without a verified round trip it returns a nil
// observation: receipt data alone never yields one.
func (o *IngressObserver) Observe(ctx context.Context, s *Store, tunnelID string, now time.Time) (*IndependentIngressObservation, IngressProbeResult, error) {
	var res IngressProbeResult
	s.mu.Lock()
	ev, reason := s.ingressReceiptEvidenceLocked(tunnelID)
	var address string
	if reason == "" {
		if op, ok := s.st.PublicIngressOperations[tunnelID]; ok {
			address = s.st.Nodes[op.Spec.IRNode].Address
		}
	}
	s.mu.Unlock()
	if reason != "" {
		return nil, res, errors.New(reason)
	}
	host, err := o.publicProbeHost(address)
	if err != nil {
		res.Failure = err.Error()
		return nil, res, err
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(ev.cmd.Binding.Port)))
	res.Target = target
	nonce, err := o.nonce()
	if err != nil {
		res.Failure = "nonce unavailable"
		return nil, res, errors.New(res.Failure)
	}
	frame := []byte(ingressProbePrefix + nonce + "\n")

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	conn, err := o.dial(ctx, "tcp", target)
	if err != nil {
		res.Failure = "connect failed"
		return nil, res, errors.New(res.Failure)
	}
	defer conn.Close()
	if ta, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		if ip, ok := netip.AddrFromSlice(ta.IP); ok {
			if err := o.checkProbeIP(ip); err != nil {
				res.Failure = "resolved to a non-public address"
				return nil, res, errors.New(res.Failure)
			}
		}
	}
	res.Connected = true
	_ = conn.SetDeadline(time.Now().Add(o.timeout))
	if _, err := conn.Write(frame); err != nil {
		res.Failure = "payload write failed"
		return nil, res, errors.New(res.Failure)
	}
	got := make([]byte, len(frame))
	if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, frame) {
		res.Failure = "payload round trip not verified"
		return nil, res, errors.New(res.Failure)
	}
	res.PayloadVerified = true
	return &IndependentIngressObservation{
		Binding:            ev.cmd.Binding,
		ObservedAt:         now.UTC(),
		ListenerEndpoint:   ev.listener.Endpoint,
		SocketIdentity:     ev.listener.SocketIdentity,
		ChainHandle:        ev.fw.ChainHandle,
		RuleHandle:         ev.fw.RuleHandle,
		TCPPayloadVerified: true,
	}, res, nil
}
