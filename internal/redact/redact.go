// Package redact removes secret material from diagnostic text before it
// leaves a process boundary: Agent job ACKs, operational ingress receipts and
// local logs (ADR-001 §5, v0.5.0).
//
// Two mechanisms are combined. Exact values the caller knows are secret (for
// example the Agent's own bearer token) are always removed. Known secret shapes
// (PEM private keys, BAFT pairing codes, bearer credentials, URL user
// information and key=value secret assignments) are removed even when the
// caller does not know the value, because diagnostic text often embeds
// subprocess output.
//
// The package is stdlib-only and has no state, so every boundary can share
// one rule set and one test suite.
package redact

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Placeholder replaces an exact secret value.
const Placeholder = "[redacted]"

// TruncationMarker ends text shortened by Bound.
const TruncationMarker = "…[truncated]"

// MinExactLen is the shortest exact value Text removes. Shorter values are
// ignored so that a misconfigured empty or tiny "secret" cannot erase ordinary
// diagnostic words; real BAFT tokens are far longer.
const MinExactLen = 8

// Rule is one named secret shape.
type Rule struct {
	Name string
	re   *regexp.Regexp
	to   string
}

// Rules are applied in order. Names are stable: support bundles count
// redactions by rule name.
var Rules = []Rule{
	{"pem-private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`), "[redacted private key]"},
	{"pairing-code", regexp.MustCompile(`BAFT(?:PAIR|REPLY)1:[A-Za-z0-9_-]+`), "[redacted pairing code]"},
	{"bearer-token", regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`), "${1}[redacted]"},
	{"url-userinfo", regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`), "${1}[redacted]@"},
	{"secret-assignment", regexp.MustCompile(`(?i)((?:token|password|passphrase|secret|private[_-]?key|api[_-]?key)["']?\s*[:=]\s*["']?)[^\s"',;]{4,}`), "${1}[redacted]"},
}

// Apply removes every rule match from s and, when counts is non-nil, adds the
// number of matches per rule name.
func Apply(s string, counts map[string]int) string {
	for _, r := range Rules {
		if counts != nil {
			if n := len(r.re.FindAllStringIndex(s, -1)); n > 0 {
				counts[r.Name] += n
			} else {
				continue
			}
		}
		s = r.re.ReplaceAllString(s, r.to)
	}
	return s
}

// Text removes the exact secret values and every rule match from s. Longer
// exact values are removed first so a secret that contains another secret is
// never partially revealed.
func Text(s string, exact ...string) string {
	if len(exact) > 0 {
		vals := make([]string, 0, len(exact))
		for _, v := range exact {
			if len(v) >= MinExactLen {
				vals = append(vals, v)
			}
		}
		sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
		for _, v := range vals {
			s = strings.ReplaceAll(s, v, Placeholder)
		}
	}
	return Apply(s, nil)
}

// Bound returns s shortened to at most max bytes without splitting a UTF-8
// sequence. Shortened text ends with TruncationMarker so a reader can tell the
// diagnostic is incomplete. Invalid UTF-8 is replaced so the result is always
// valid UTF-8. max <= 0 returns "".
func Bound(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= max {
		return s
	}
	keep := max - len(TruncationMarker)
	if keep <= 0 {
		return cut(TruncationMarker, max)
	}
	return cut(s, keep) + TruncationMarker
}

// Diagnostic redacts s and then bounds it, so truncation can never expose
// part of a secret that a later rule would have matched as a whole.
func Diagnostic(s string, max int, exact ...string) string {
	return Bound(Text(s, exact...), max)
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
