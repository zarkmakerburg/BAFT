// Package dnsfailover keeps one DNS name pointing only at reachable IR entry
// servers, using the free Cloudflare DNS API (no paid load balancer).
//
// Safety rules:
//   - Only A/AAAA records carrying the managed comment are created or deleted;
//     a foreign record with the same name stops reconciliation.
//   - When no target is healthy the record set is left unchanged: the prober's
//     own network is the likelier fault than every IR at once.
//   - A target changes state only after FailAfter / RecoverAfter consecutive
//     probe results, so one lost probe never moves traffic.
package dnsfailover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// ManagedComment marks the records this package owns.
const ManagedComment = "managed-by:baft-dns-failover"

const defaultAPIBase = "https://api.cloudflare.com/client/v4"

// Target is one IR entry server.
type Target struct {
	Name    string `json:"name"`
	Address string `json:"address"` // the IP published in DNS
	Probe   string `json:"probe"`   // host:port checked with a TCP connect
}

// Config is the on-disk configuration.
type Config struct {
	ZoneID          string   `json:"zone_id,omitempty"` // found from record when empty
	Record          string   `json:"record"`
	TokenFile       string   `json:"token_file"`
	TTL             int      `json:"ttl"`
	IntervalSeconds int      `json:"interval_seconds"`
	ProbeTimeoutMS  int      `json:"probe_timeout_ms"`
	FailAfter       int      `json:"fail_after"`
	RecoverAfter    int      `json:"recover_after"`
	Targets         []Target `json:"targets"`
}

// LoadConfig reads and validates a JSON config file and applies defaults.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, c.normalize()
}

func (c *Config) normalize() error {
	if c.TTL == 0 {
		c.TTL = 60
	}
	if c.IntervalSeconds == 0 {
		c.IntervalSeconds = 10
	}
	if c.ProbeTimeoutMS == 0 {
		c.ProbeTimeoutMS = 3000
	}
	if c.FailAfter == 0 {
		c.FailAfter = 3
	}
	if c.RecoverAfter == 0 {
		c.RecoverAfter = 3
	}
	switch {
	case c.Record == "" || c.TokenFile == "":
		return errors.New("record and token_file are required")
	case strings.Count(strings.Trim(c.Record, "."), ".") < 1:
		return errors.New("record must be a full DNS name such as t.example.com")
	case c.TTL != 1 && (c.TTL < 30 || c.TTL > 86400):
		return errors.New("ttl must be 1 (auto) or 30..86400")
	case c.IntervalSeconds < 2 || c.IntervalSeconds > 3600:
		return errors.New("interval_seconds must be 2..3600")
	case c.ProbeTimeoutMS < 100 || c.ProbeTimeoutMS > 30000:
		return errors.New("probe_timeout_ms must be 100..30000")
	case c.FailAfter < 1 || c.RecoverAfter < 1:
		return errors.New("fail_after and recover_after must be at least 1")
	case len(c.Targets) < 2:
		return errors.New("at least two targets are required for failover")
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		ip := net.ParseIP(t.Address)
		if t.Name == "" || ip == nil {
			return fmt.Errorf("target %q: name and a literal IP address are required", t.Name)
		}
		if _, _, err := net.SplitHostPort(t.Probe); err != nil {
			return fmt.Errorf("target %q: probe must be host:port", t.Name)
		}
		if seen[ip.String()] {
			return fmt.Errorf("duplicate target address %s", ip)
		}
		seen[ip.String()] = true
	}
	return nil
}

// Prober reports whether a target answers.
type Prober func(ctx context.Context, t Target, timeout time.Duration) bool

// TCPProber connects to t.Probe.
func TCPProber(ctx context.Context, t Target, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp", t.Probe)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

type targetState struct {
	healthy bool
	streak  int // consecutive results contradicting healthy
}

// Controller runs the probe/reconcile loop.
type Controller struct {
	cfg     Config
	token   string
	apiBase string
	client  *http.Client
	probe   Prober
	state   map[string]*targetState
	logf    func(string, ...any)
}

// New builds a controller. All targets start healthy so a restart never
// withdraws records before they have been probed.
func New(cfg Config, token string, probe Prober, logf func(string, ...any)) *Controller {
	if probe == nil {
		probe = TCPProber
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	st := map[string]*targetState{}
	for _, t := range cfg.Targets {
		st[t.Name] = &targetState{healthy: true}
	}
	return &Controller{cfg: cfg, token: token, apiBase: defaultAPIBase,
		client: &http.Client{Timeout: 20 * time.Second}, probe: probe, state: st, logf: logf}
}

// SetAPIBase points the controller at another Cloudflare API endpoint (tests).
func (c *Controller) SetAPIBase(u string) { c.apiBase = u }

// Healthy returns the names of targets currently considered healthy.
func (c *Controller) Healthy() []string {
	var out []string
	for _, t := range c.cfg.Targets {
		if c.state[t.Name].healthy {
			out = append(out, t.Name)
		}
	}
	return out
}

// ProbeOnce probes every target once and updates hysteresis state.
func (c *Controller) ProbeOnce(ctx context.Context) {
	timeout := time.Duration(c.cfg.ProbeTimeoutMS) * time.Millisecond
	for _, t := range c.cfg.Targets {
		up := c.probe(ctx, t, timeout)
		s := c.state[t.Name]
		if up == s.healthy {
			s.streak = 0
			continue
		}
		s.streak++
		need := c.cfg.FailAfter
		if !s.healthy {
			need = c.cfg.RecoverAfter
		}
		if s.streak >= need {
			s.healthy = up
			s.streak = 0
			c.logf("target %s is now %s", t.Name, map[bool]string{true: "healthy", false: "down"}[up])
		}
	}
}

type dnsRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment"`
}

type envelope struct {
	Success bool                 `json:"success"`
	Errors  []struct{ Code int } `json:"errors"`
	Result  json.RawMessage      `json:"result"`
}

func (c *Controller) api(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.New("cloudflare request failed")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.New("cloudflare response read failed")
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("cloudflare response is not JSON (status %d)", resp.StatusCode)
	}
	if !env.Success || resp.StatusCode/100 != 2 {
		codes := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			codes = append(codes, fmt.Sprint(e.Code))
		}
		return fmt.Errorf("cloudflare rejected %s (status %d, codes %s)", method, resp.StatusCode, strings.Join(codes, ","))
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func recordType(ip string) string {
	if net.ParseIP(ip).To4() != nil {
		return "A"
	}
	return "AAAA"
}

// FindZone returns the id of the most specific zone the token can see that
// contains the record (t.a.example.com tries a.example.com, then example.com).
func (c *Controller) FindZone(ctx context.Context) (string, error) {
	labels := strings.Split(strings.Trim(strings.ToLower(c.cfg.Record), "."), ".")
	for i := 0; i+2 <= len(labels); i++ {
		name := strings.Join(labels[i:], ".")
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := c.api(ctx, http.MethodGet, "/zones?"+url.Values{"name": {name}}.Encode(), nil, &zones); err != nil {
			return "", err
		}
		for _, z := range zones {
			if strings.EqualFold(z.Name, name) && z.ID != "" {
				return z.ID, nil
			}
		}
	}
	return "", fmt.Errorf("no Cloudflare zone visible to this token contains %s", c.cfg.Record)
}

// Reconcile makes the managed records equal the healthy target set. It
// returns the published addresses after the change.
func (c *Controller) Reconcile(ctx context.Context) ([]string, error) {
	if c.cfg.ZoneID == "" {
		id, err := c.FindZone(ctx)
		if err != nil {
			return nil, err
		}
		c.cfg.ZoneID = id
	}
	zone := url.PathEscape(c.cfg.ZoneID)
	var recs []dnsRecord
	q := url.Values{"name": {c.cfg.Record}, "per_page": {"100"}}
	if err := c.api(ctx, http.MethodGet, "/zones/"+zone+"/dns_records?"+q.Encode(), nil, &recs); err != nil {
		return nil, err
	}
	have := map[string]dnsRecord{}
	for _, r := range recs {
		if r.Type != "A" && r.Type != "AAAA" && r.Type != "CNAME" {
			continue
		}
		if r.Comment != ManagedComment {
			return nil, fmt.Errorf("refusing to manage %s: a %s record not created by baft exists", c.cfg.Record, r.Type)
		}
		have[net.ParseIP(r.Content).String()] = r
	}
	want := map[string]bool{}
	for _, t := range c.cfg.Targets {
		if c.state[t.Name].healthy {
			want[net.ParseIP(t.Address).String()] = true
		}
	}
	if len(want) == 0 {
		c.logf("no target is healthy; leaving %d records unchanged", len(have))
		return sortedKeys(have), nil
	}
	// Add before removing, so the name never resolves to nothing.
	for ip := range want {
		if _, ok := have[ip]; ok {
			continue
		}
		r := dnsRecord{Type: recordType(ip), Name: c.cfg.Record, Content: ip, TTL: c.cfg.TTL, Comment: ManagedComment}
		if err := c.api(ctx, http.MethodPost, "/zones/"+zone+"/dns_records", r, nil); err != nil {
			return nil, err
		}
		c.logf("published %s", ip)
	}
	for ip, r := range have {
		if want[ip] {
			continue
		}
		if err := c.api(ctx, http.MethodDelete, "/zones/"+zone+"/dns_records/"+url.PathEscape(r.ID), nil, nil); err != nil {
			return nil, err
		}
		c.logf("withdrew %s", ip)
	}
	out := make([]string, 0, len(want))
	for ip := range want {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out, nil
}

func sortedKeys(m map[string]dnsRecord) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Run probes and reconciles until ctx ends. A failed reconcile is retried on
// the next tick.
func (c *Controller) Run(ctx context.Context) error {
	tick := time.NewTicker(time.Duration(c.cfg.IntervalSeconds) * time.Second)
	defer tick.Stop()
	for {
		c.ProbeOnce(ctx)
		if _, err := c.Reconcile(ctx); err != nil {
			c.logf("reconcile: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ReadToken reads a token file that must not be readable by group or others.
func ReadToken(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s must not be readable by group or others", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(raw))
	if tok == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return tok, nil
}
