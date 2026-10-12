package bcc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Only public literal IPs are sent to the country service. Neither credentials,
// node names, management hostnames nor private/Tailscale addresses leave BCC.
func locationIP(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	a, err := netip.ParseAddr(value)
	if err != nil || a.Zone() != "" {
		return ""
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return ""
	}
	if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
		return ""
	}
	for _, p := range locationExcludedPrefixes {
		if p.Contains(a) {
			return ""
		}
	}
	return a.String()
}

var locationExcludedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2001:2::/48"),
}

func nodeLocationIP(n Node) string {
	// Use the entered management IP first; fall back to a public path address
	// when management uses a private network.
	for _, value := range []string{n.Address, n.PathIPv4, n.PathIPv6} {
		if ip := locationIP(value); ip != "" {
			return ip
		}
	}
	return ""
}

func automaticCountry(n Node) bool {
	// Old persisted country selections have no source; treat them as manual.
	return n.CountrySource != "manual" && (n.Country == "" || n.CountrySource == "ip")
}

// ConfigureLocationLookup selects a country.is-compatible HTTPS service.
// The default is keyless api.country.is. "off" disables automatic lookups.
// This is configured before serving requests, like the other BCC subsystems.
func (s *Server) ConfigureLocationLookup(base string) error {
	if base == "off" {
		s.locationLookup = nil
		return nil
	}
	if base == "" {
		base = "https://api.country.is"
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("BAFT_BCC_GEOIP_URL must be an HTTPS base URL or off")
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	s.locationLookup = countryLookup(client, strings.TrimRight(u.String(), "/"))
	return nil
}

func countryLookup(client *http.Client, base string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, ip string) (string, error) {
		if locationIP(ip) != ip {
			return "", errors.New("location requires a public literal IP")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+url.PathEscape(ip), nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("country service status %d", resp.StatusCode)
		}
		var out struct {
			IP      string `json:"ip"`
			Country string `json:"country"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
		if err != nil || len(body) > 4096 {
			return "", errors.New("invalid country service response size")
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", errors.New("invalid country service response")
		}
		got, err := netip.ParseAddr(out.IP)
		if err != nil || got.Unmap().String() != ip {
			return "", errors.New("country response belongs to a different IP")
		}
		code, ok := normalizeCountry(out.Country)
		if !ok || code == "" {
			return "", errors.New("country service returned no supported map location")
		}
		return code, nil
	}
}

// StartLocationLoop fills new and existing unlocated nodes in the background,
// independently of SSH/registration/overview requests. Failed lookups retry
// after an hour; IP changes can retry immediately. Successful values persist.
func (s *Server) StartLocationLoop(ctx context.Context) {
	if s.locationLookup == nil {
		return
	}
	type attempt struct {
		ip string
		at time.Time
	}
	attempts := map[string]attempt{}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	rate := time.NewTicker(time.Second)
	defer rate.Stop()
	for {
		nodes := s.store.ListNodes()
		present := map[string]bool{}
		for _, n := range nodes {
			present[n.ID] = true
			ip := nodeLocationIP(n)
			if n.Revoked || !automaticCountry(n) || ip == "" || (n.Country != "" && n.CountryIP == ip) {
				continue
			}
			a := attempts[n.ID]
			if a.ip == ip && !n.UpdatedAt.After(a.at) && s.now().Sub(a.at) < time.Hour {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-rate.C:
			}
			attempts[n.ID] = attempt{ip: ip, at: s.now()}
			lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			code, err := s.locationLookup(lookupCtx, ip)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("BCC: IP location unavailable for node %q: %v", n.ID, err)
				continue
			}
			s.mutationMu.Lock()
			changed, err := s.store.SetNodeCountryFromIP(n.ID, ip, code, s.now())
			if changed && err == nil {
				err = s.FlushSecurityAuditIntents()
			}
			s.mutationMu.Unlock()
			if err != nil {
				log.Printf("BCC: could not persist IP location for node %q: %v", n.ID, err)
			}
		}
		for id := range attempts {
			if !present[id] {
				delete(attempts, id)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// ResetNodeCountry returns to the automatic default, also after an explicit
// manual "no location" choice. The next background pass fills the country.
func (s *Store) ResetNodeCountry(id string, now time.Time, audit AuditEntry) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.st.Nodes[id]
	if !ok {
		return Node{}, errors.New("node not found")
	}
	n.Country, n.CountrySource, n.CountryIP = "", "", ""
	if err := s.commitCountryLocked(n, now, audit); err != nil {
		return Node{}, err
	}
	return publicNode(n), nil
}

// SetNodeCountryFromIP rechecks both the IP and override under the store lock:
// a lookup that finishes after an edit must never overwrite the newer choice.
func (s *Store) SetNodeCountryFromIP(id, ip, country string, now time.Time) (bool, error) {
	code, ok := normalizeCountry(country)
	if !ok || code == "" || locationIP(ip) != ip {
		return false, errors.New("invalid IP location")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.st.Nodes[id]
	if !ok || n.Revoked || !automaticCountry(n) || nodeLocationIP(n) != ip {
		return false, nil
	}
	if n.Country == code && n.CountrySource == "ip" && n.CountryIP == ip {
		return false, nil
	}
	n.Country, n.CountrySource, n.CountryIP = code, "ip", ip
	audit := AuditEntry{Actor: "system", Details: map[string]any{"country": code, "source": "ip", "ip": ip, "approximate": true}}
	if err := s.commitCountryLocked(n, now, audit); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) commitCountryLocked(n Node, now time.Time, audit AuditEntry) error {
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	n.UpdatedAt = now.UTC()
	s.st.Nodes[n.ID] = n
	audit.Timestamp, audit.Action, audit.Target, audit.Outcome = n.UpdatedAt, "node.location", n.ID, "success"
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}
