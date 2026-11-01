package ingressinventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ConfigPortClaim is a TCP port that another service on the host is
// configured to listen on, read from that service's own configuration. It is
// an inert observation: BAFT refuses to take such a port, and never edits,
// stops or restarts the other service (ADR-003 §4.2).
type ConfigPortClaim struct {
	Source string // configuration file
	Tag    string // inbound tag, if any
	From   uint16 // inclusive port range
	To     uint16
}

// Covers reports whether the claim includes port.
func (c ConfigPortClaim) Covers(port uint16) bool { return port >= c.From && port <= c.To }

// xrayConfigPaths are the configuration files of Xray as installed by 3X-UI
// and by the official Xray installer. Only the host execution profile reads
// them. A var only so in-package tests can point it at fixtures.
var xrayConfigPaths = []string{
	"/usr/local/x-ui/bin/config.json",
	"/usr/local/etc/xray/config.json",
	"/etc/xray/config.json",
}

const maxXrayConfigBytes = 8 << 20

// collectXrayInboundClaims reads every present Xray configuration and returns
// the inbound port claims. A file that exists but cannot be read or parsed is
// UNKNOWN, never "no claims": the caller must treat unknown as a refusal.
// Inbounds added in a panel but not yet written to the running Xray
// configuration are not visible here; the operator acknowledgement covers
// that residual (ADR-003 §4.2 item 3).
func collectXrayInboundClaims(paths []string) (claims []ConfigPortClaim, unknown []string) {
	for _, path := range paths {
		st, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !st.Mode().IsRegular() {
			unknown = append(unknown, "xray config "+path+" present but unreadable")
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			unknown = append(unknown, "xray config "+path+" present but unreadable")
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(f, maxXrayConfigBytes+1))
		_ = f.Close()
		if err != nil || len(raw) > maxXrayConfigBytes {
			unknown = append(unknown, "xray config "+path+" unreadable or too large")
			continue
		}
		c, u := parseXrayInboundClaims(path, raw)
		claims = append(claims, c...)
		unknown = append(unknown, u...)
	}
	return claims, unknown
}

// parseXrayInboundClaims extracts inbound ports from an Xray JSON config.
// Ports may be a number, "N", "A-B" or a comma list; anything else (e.g.
// "env:...") is unknown, as is a config that is not plain JSON.
func parseXrayInboundClaims(source string, raw []byte) (claims []ConfigPortClaim, unknown []string) {
	var cfg struct {
		Inbounds []struct {
			Port   json.RawMessage `json:"port"`
			Tag    string          `json:"tag"`
			Listen string          `json:"listen"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, []string{"xray config " + source + " is not parseable JSON"}
	}
	for i, in := range cfg.Inbounds {
		tag := in.Tag
		if tag == "" {
			tag = "#" + strconv.Itoa(i)
		}
		if strings.HasPrefix(in.Listen, "/") || strings.HasPrefix(in.Listen, "@") {
			// A Unix-domain-socket inbound takes no TCP port.
			continue
		}
		ranges, err := parseXrayPort(in.Port)
		if err != nil {
			unknown = append(unknown, fmt.Sprintf("xray config %s inbound %s: %v", source, tag, err))
			continue
		}
		for _, r := range ranges {
			claims = append(claims, ConfigPortClaim{Source: source, Tag: tag, From: r[0], To: r[1]})
		}
	}
	return claims, unknown
}

func parseXrayPort(raw json.RawMessage) ([][2]uint16, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		// Xray requires a port on listening inbounds; a missing one is unknown.
		return nil, errors.New("port missing")
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		v, err := strconv.ParseUint(n.String(), 10, 16)
		if err != nil || v == 0 {
			return nil, errors.New("port out of range")
		}
		return [][2]uint16{{uint16(v), uint16(v)}}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errors.New("port is neither a number nor a string")
	}
	var out [][2]uint16
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		a, errA := strconv.ParseUint(strings.TrimSpace(lo), 10, 16)
		b := a
		var errB error
		if isRange {
			b, errB = strconv.ParseUint(strings.TrimSpace(hi), 10, 16)
		}
		if errA != nil || errB != nil || a == 0 || b < a {
			return nil, fmt.Errorf("unsupported port value %q", s)
		}
		out = append(out, [2]uint16{uint16(a), uint16(b)})
	}
	return out, nil
}
