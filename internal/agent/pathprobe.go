package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/pathprobetls"
)

const maxPathProbeBytes = 1 << 20

// Speed flows (path-probe-speed-v1) stream for a bounded time and are capped
// so a probe listener can never be used to absorb unbounded traffic.
const (
	maxPathProbeSpeedBytes = agentjob.PathProbeSpeedMaxBytes
	pathProbeSpeedChunk    = 64 << 10
)

type pathProbeListener struct {
	mu       sync.Mutex
	ln       net.Listener
	probeID  string
	family   string
	port     int
	expires  time.Time
	received []int64
	full     []bool
	errors   []string
	expired  bool
}

type pathProbeFlow struct {
	Requested    int64  `json:"requested"`
	BytesWritten int64  `json:"bytes_written"`
	ConnectMS    int64  `json:"connect_ms"`
	DurationMS   int64  `json:"duration_ms"`
	Connected    bool   `json:"connected"`
	Handshake    bool   `json:"handshake"`
	ACK          bool   `json:"ack"`
	Error        string `json:"error,omitempty"`
}

type pathProbeRunEvidence struct {
	ProbeID string          `json:"probe_id"`
	Family  string          `json:"family"`
	Target  string          `json:"target"`
	TLS     bool            `json:"tls,omitempty"`
	Bulk    []pathProbeFlow `json:"bulk"`
	Trickle pathProbeFlow   `json:"trickle"`
	Speed   *pathProbeSpeed `json:"speed,omitempty"`
}

// pathProbeSpeed is one time-bounded throughput flow. AckedBytes is what the
// listener says it received; a result counts only when it equals
// BytesWritten. TransferMS runs from READY until the listener's count arrives.
type pathProbeSpeed struct {
	Seconds      int    `json:"seconds"`
	BytesWritten int64  `json:"bytes_written"`
	AckedBytes   int64  `json:"acked_bytes"`
	TransferMS   int64  `json:"transfer_ms"`
	ACK          bool   `json:"ack"`
	Error        string `json:"error,omitempty"`
}

type pathProbeListenEvidence struct {
	ProbeID  string   `json:"probe_id"`
	Family   string   `json:"family,omitempty"`
	Port     int      `json:"port,omitempty"`
	Received []int64  `json:"received,omitempty"`
	Full     []bool   `json:"full,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Expired  bool     `json:"expired,omitempty"`
}

type pathProbeInventoryEvidence struct {
	IPv4 []string `json:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty"`
}

func pathProbeInventory() (string, error) {
	seen4, seen6 := map[string]bool{}, map[string]bool{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			host, _, err := net.ParseCIDR(a.String())
			if err != nil || host == nil {
				continue
			}
			if !host.IsGlobalUnicast() || host.IsLinkLocalUnicast() || host.IsLoopback() {
				continue
			}
			if v4 := host.To4(); v4 != nil {
				seen4[v4.String()] = true
				continue
			}
			if v6 := host.To16(); v6 != nil {
				seen6[v6.String()] = true
			}
		}
	}
	ev := pathProbeInventoryEvidence{}
	for ip := range seen4 {
		ev.IPv4 = append(ev.IPv4, ip)
	}
	for ip := range seen6 {
		ev.IPv6 = append(ev.IPv6, ip)
	}
	sort.Strings(ev.IPv4)
	sort.Strings(ev.IPv6)
	b, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (a *Agent) pathProbe(ctx context.Context, j agentjob.Job) (string, error) {
	switch j.Action {
	case agentjob.ActionPathProbeInventory:
		return pathProbeInventory()
	case agentjob.ActionPathProbeListen:
		return a.pathProbeListen(j)
	case agentjob.ActionPathProbeRun:
		return a.pathProbeRun(ctx, j)
	case agentjob.ActionPathProbeStop:
		return a.pathProbeStop(j), nil
	default:
		return "", fmt.Errorf("action %s has no path-probe handler", j.Action)
	}
}

func probeNetwork(family string) (network, bind string, err error) {
	switch family {
	case "4":
		return "tcp4", "0.0.0.0", nil
	case "6":
		return "tcp6", "::", nil
	default:
		return "", "", fmt.Errorf("invalid IP family %q", family)
	}
}

func (a *Agent) pathProbeListen(j agentjob.Job) (string, error) {
	p := j.Params
	port, _ := strconv.Atoi(p["port"])
	ttl, _ := strconv.Atoi(p["ttl_seconds"])
	if port < 1 || port > 65535 || ttl < 10 || ttl > 120 {
		return "", fmt.Errorf("invalid path-probe listener bounds")
	}
	network, bind, err := probeNetwork(p["family"])
	if err != nil {
		return "", err
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	var tlsCfg *tls.Config
	if p["tls"] == "1" {
		// path-probe-tls-v1: the flows arrive inside TLS 1.3 under the
		// certificate both ends derive from the probe ID.
		if tlsCfg, err = pathprobetls.ServerConfig(p["probe_id"], p["server_ip"]); err != nil {
			return "", fmt.Errorf("path probe TLS: %w", err)
		}
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return "", fmt.Errorf("path probe listen %s: %w", addr, err)
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	pl := &pathProbeListener{
		ln: ln, probeID: p["probe_id"], family: p["family"], port: port,
		expires: time.Now().UTC().Add(time.Duration(ttl) * time.Second),
	}
	a.probeMu.Lock()
	if a.probeListeners == nil {
		a.probeListeners = map[string]*pathProbeListener{}
	}
	if old := a.probeListeners[p["probe_id"]]; old != nil {
		a.probeMu.Unlock()
		_ = ln.Close()
		return "", fmt.Errorf("path probe %s already has a listener", p["probe_id"])
	}
	a.probeListeners[p["probe_id"]] = pl
	a.probeMu.Unlock()
	go a.servePathProbe(pl, time.Duration(ttl)*time.Second)
	mode := ""
	if tlsCfg != nil {
		mode = " (TLS)"
	}
	return fmt.Sprintf("path probe listener ready on %s/%s%s", addr, network, mode), nil
}

func (a *Agent) servePathProbe(pl *pathProbeListener, ttl time.Duration) {
	timer := time.AfterFunc(ttl, func() {
		pl.mu.Lock()
		pl.expired = true
		pl.mu.Unlock()
		_ = pl.ln.Close()
	})
	defer timer.Stop()
	for {
		c, err := pl.ln.Accept()
		if err != nil {
			return
		}
		go pl.handle(c)
	}
}

func readProbeLine(r io.Reader, limit int) (string, error) {
	buf := make([]byte, 0, 96)
	one := []byte{0}
	for len(buf) < limit {
		n, err := r.Read(one)
		if n == 1 {
			if one[0] == '\n' {
				return string(buf), nil
			}
			buf = append(buf, one[0])
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("probe header exceeds %d bytes", limit)
}

func (pl *pathProbeListener) record(n int64, full bool, err error) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.received = append(pl.received, n)
	pl.full = append(pl.full, full)
	if err != nil && len(pl.errors) < 8 {
		pl.errors = append(pl.errors, err.Error())
	}
}

func (pl *pathProbeListener) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(12 * time.Second))
	line, err := readProbeLine(c, 200)
	if err != nil {
		pl.record(0, false, err)
		return
	}
	f := strings.Fields(line)
	if len(f) == 3 && f[0] == "BAFTS1" && f[1] == pl.probeID {
		pl.handleSpeed(c, f[2])
		return
	}
	if len(f) != 3 || f[0] != "BAFTP1" || f[1] != pl.probeID {
		pl.record(0, false, fmt.Errorf("invalid probe header"))
		return
	}
	size, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || size < 1 || size > maxPathProbeBytes {
		pl.record(0, false, fmt.Errorf("invalid probe payload size"))
		return
	}
	if _, err := io.WriteString(c, "READY\n"); err != nil {
		pl.record(0, false, err)
		return
	}
	h := sha256.New()
	n, rerr := io.CopyN(h, c, size)
	full := rerr == nil && n == size
	pl.record(n, full, rerr)
	if !full {
		return
	}
	_, _ = fmt.Fprintf(c, "OK %d %s\n", n, hex.EncodeToString(h.Sum(nil)))
}

// handleSpeed counts (and discards) at most the announced, capped number of
// bytes until the sender half-closes, then reports the count. Speed flows are
// not recorded as integrity evidence; the sender's evidence carries them.
func (pl *pathProbeListener) handleSpeed(c net.Conn, raw string) {
	limit, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || limit < 1 || limit > maxPathProbeSpeedBytes {
		return
	}
	if _, err := io.WriteString(c, "READY\n"); err != nil {
		return
	}
	n, err := io.Copy(io.Discard, io.LimitReader(c, limit+1))
	if err != nil || n > limit {
		return
	}
	_, _ = fmt.Fprintf(c, "OK %d\n", n)
}

func (a *Agent) pathProbeRun(ctx context.Context, j agentjob.Job) (string, error) {
	p := j.Params
	network, _, err := probeNetwork(p["family"])
	if err != nil {
		return "", err
	}
	target := p["target"]
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("invalid path-probe target: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("path-probe target must be a literal enrolled-node IP")
	}
	if (p["family"] == "4" && ip.To4() == nil) || (p["family"] == "6" && (ip.To4() != nil || ip.To16() == nil)) {
		return "", fmt.Errorf("target IP does not match requested family")
	}
	if pn, err := strconv.Atoi(port); err != nil || pn < 1 || pn > 65535 {
		return "", fmt.Errorf("invalid path-probe target port")
	}
	payloadBytes, _ := strconv.Atoi(p["payload_bytes"])
	attempts, _ := strconv.Atoi(p["attempts"])
	trickleBytes, _ := strconv.Atoi(p["trickle_bytes"])
	trickleMS, _ := strconv.Atoi(p["trickle_ms"])
	if payloadBytes < 1024 || payloadBytes > maxPathProbeBytes || attempts < 1 || attempts > 5 ||
		trickleBytes < 1024 || trickleBytes > 64<<10 || trickleMS < 10 || trickleMS > 1000 {
		return "", fmt.Errorf("invalid path-probe run bounds")
	}
	ev := pathProbeRunEvidence{ProbeID: p["probe_id"], Family: p["family"], Target: target, TLS: p["tls"] == "1"}
	pd := probeDialer{network: network, target: target, probeID: p["probe_id"], tls: ev.TLS}
	for i := 0; i < attempts; i++ {
		ev.Bulk = append(ev.Bulk, runPathProbeFlow(ctx, pd, payloadBytes, 0))
	}
	ev.Trickle = runPathProbeFlow(ctx, pd, trickleBytes, time.Duration(trickleMS)*time.Millisecond)
	if raw, ok := p["speed_seconds"]; ok {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 1 || seconds > 8 {
			return "", fmt.Errorf("invalid path-probe speed bounds")
		}
		ev.Speed = runPathProbeSpeed(ctx, pd, time.Duration(seconds)*time.Second, maxPathProbeSpeedBytes)
		ev.Speed.Seconds = seconds
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// probeDialer opens one probe flow: a TCP connect (timed alone, so connect
// RTTs stay comparable) and, for path-probe-tls-v1, the TLS 1.3 handshake
// under the probe-derived certificate.
type probeDialer struct {
	network, target, probeID string
	tls                      bool
}

func (pd probeDialer) dial(ctx context.Context) (c net.Conn, connectMS int64, connected bool, err error) {
	start := time.Now()
	d := net.Dialer{Timeout: 3 * time.Second}
	raw, err := d.DialContext(ctx, pd.network, pd.target)
	if err != nil {
		return nil, time.Since(start).Milliseconds(), false, err
	}
	connectMS = time.Since(start).Milliseconds()
	if !pd.tls {
		return raw, connectMS, true, nil
	}
	host, _, _ := net.SplitHostPort(pd.target)
	tc, err := pathprobetls.Client(ctx, raw, pd.probeID, host)
	if err != nil {
		_ = raw.Close()
		return nil, connectMS, true, err
	}
	return tc, connectMS, true, nil
}

func runPathProbeFlow(parent context.Context, pd probeDialer, size int, pause time.Duration) pathProbeFlow {
	probeID := pd.probeID
	out := pathProbeFlow{Requested: int64(size)}
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	start := time.Now()
	c, connectMS, connected, err := pd.dial(ctx)
	out.ConnectMS, out.Connected = connectMS, connected
	if err != nil {
		out.Error = err.Error()
		out.DurationMS = time.Since(start).Milliseconds()
		return out
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		out.Error = err.Error()
		return out
	}
	sum := sha256.Sum256(payload)
	if _, err := fmt.Fprintf(c, "BAFTP1 %s %d\n", probeID, size); err != nil {
		out.Error = err.Error()
		return out
	}
	ready, err := readProbeLine(c, 32)
	if err != nil || ready != "READY" {
		if err != nil {
			out.Error = err.Error()
		} else {
			out.Error = "invalid probe readiness acknowledgement"
		}
		out.DurationMS = time.Since(start).Milliseconds()
		return out
	}
	out.Handshake = true
	chunk := size
	if pause > 0 {
		chunk = 1024
	}
	for off := 0; off < size; {
		end := off + chunk
		if end > size {
			end = size
		}
		n, err := c.Write(payload[off:end])
		out.BytesWritten += int64(n)
		off += n
		if err != nil {
			out.Error = err.Error()
			break
		}
		if pause > 0 && off < size {
			select {
			case <-ctx.Done():
				out.Error = ctx.Err().Error()
				out.DurationMS = time.Since(start).Milliseconds()
				return out
			case <-time.After(pause):
			}
		}
	}
	line, err := readProbeLine(c, 256)
	if err == nil {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "OK" && f[1] == strconv.Itoa(size) && f[2] == hex.EncodeToString(sum[:]) {
			out.ACK = true
		} else {
			err = fmt.Errorf("invalid probe acknowledgement")
		}
	}
	if err != nil && out.Error == "" {
		out.Error = err.Error()
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out
}

// runPathProbeSpeed streams random data to the probe listener until the time
// budget or the byte cap is reached, half-closes, and verifies that the
// listener counted exactly what was written.
func runPathProbeSpeed(parent context.Context, pd probeDialer, budget time.Duration, max int64) *pathProbeSpeed {
	probeID := pd.probeID
	out := &pathProbeSpeed{}
	ctx, cancel := context.WithTimeout(parent, budget+6*time.Second)
	defer cancel()
	c, _, _, err := pd.dial(ctx)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	chunk := make([]byte, pathProbeSpeedChunk)
	if _, err := rand.Read(chunk); err != nil {
		out.Error = err.Error()
		return out
	}
	if _, err := fmt.Fprintf(c, "BAFTS1 %s %d\n", probeID, max); err != nil {
		out.Error = err.Error()
		return out
	}
	if ready, err := readProbeLine(c, 32); err != nil || ready != "READY" {
		out.Error = "invalid speed readiness acknowledgement"
		if err != nil {
			out.Error = err.Error()
		}
		return out
	}
	start := time.Now()
	for time.Since(start) < budget && out.BytesWritten < max {
		if ctx.Err() != nil {
			out.Error = ctx.Err().Error()
			break
		}
		part := chunk
		if rest := max - out.BytesWritten; rest < int64(len(part)) {
			part = part[:rest]
		}
		n, err := c.Write(part)
		out.BytesWritten += int64(n)
		if err != nil {
			out.Error = err.Error()
			break
		}
	}
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	line, err := readProbeLine(c, 64)
	out.TransferMS = time.Since(start).Milliseconds()
	if err != nil {
		if out.Error == "" {
			out.Error = err.Error()
		}
		return out
	}
	f := strings.Fields(line)
	if len(f) != 2 || f[0] != "OK" {
		if out.Error == "" {
			out.Error = "invalid speed acknowledgement"
		}
		return out
	}
	acked, perr := strconv.ParseInt(f[1], 10, 64)
	if perr != nil {
		out.Error = "invalid speed acknowledgement"
		return out
	}
	out.AckedBytes = acked
	out.ACK = out.Error == "" && acked == out.BytesWritten
	return out
}

func (a *Agent) pathProbeStop(j agentjob.Job) string {
	id := j.Params["probe_id"]
	a.probeMu.Lock()
	pl := a.probeListeners[id]
	if pl != nil {
		delete(a.probeListeners, id)
	}
	a.probeMu.Unlock()
	if pl == nil {
		b, _ := json.Marshal(pathProbeListenEvidence{ProbeID: id, Expired: true})
		return string(b)
	}
	_ = pl.ln.Close()
	pl.mu.Lock()
	ev := pathProbeListenEvidence{
		ProbeID: pl.probeID, Family: pl.family, Port: pl.port,
		Received: append([]int64(nil), pl.received...),
		Full:     append([]bool(nil), pl.full...),
		Errors:   append([]string(nil), pl.errors...), Expired: pl.expired,
	}
	pl.mu.Unlock()
	b, _ := json.Marshal(ev)
	return string(b)
}
