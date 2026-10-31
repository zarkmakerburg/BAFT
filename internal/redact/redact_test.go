package redact

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const agentToken = "tok_9f3KqZ2vLw8RmX4bN7cJ"

func TestTextRemovesExactValuesAndKnownShapes(t *testing.T) {
	pem := "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIMuF0Q\n-----END PRIVATE KEY-----"
	cases := []struct {
		name, in string
		secrets  []string
	}{
		{"exact token in error", "post failed: token " + agentToken + " rejected", []string{agentToken}},
		{"bearer header echoed", "HTTP 401: Authorization: Bearer abcdEFGH1234.zz", []string{"abcdEFGH1234.zz"}},
		{"pem key in command output", "baft doctor: exit 1\n" + pem + "\nmore", []string{"MC4CAQAwBQYDK2VwBCIEIMuF0Q", "BEGIN PRIVATE KEY"}},
		{"truncated pem without end", "out:\n-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIBc8", []string{"MHcCAQEEIBc8"}},
		{"pairing code", "ir-apply failed: BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA", []string{"BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA"}},
		{"reply code", "ex-accept: BAFTREPLY1:UVJTVFVWV1hZWmFiY2RlZg", []string{"BAFTREPLY1:UVJTVFVWV1hZWmFiY2RlZg"}},
		{"url userinfo", "Get \"https://admin:hunter22@bcc.example:8443/api\": EOF", []string{"admin:hunter22", "hunter22"}},
		{"json secret assignment", `{"password":"s3cretvalue","ok":true}`, []string{"s3cretvalue"}},
		{"env secret assignment", "BAFT_API_KEY=k_0123456789 ./run", []string{"k_0123456789"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.in, agentToken)
			for _, s := range tc.secrets {
				if strings.Contains(got, s) {
					t.Fatalf("secret %q survived: %q", s, got)
				}
			}
			if !strings.Contains(got, "redacted") {
				t.Fatalf("no redaction marker in %q", got)
			}
		})
	}
}

func TestTextPreservesOrdinaryDiagnostics(t *testing.T) {
	// Identifiers, digests and ordinary words that merely mention a keyword
	// must survive: operators and BCC rely on them for correlation.
	in := `ingress admission: generation drift job=job-7 manifest_digest=` + strings.Repeat("ab", 32) +
		` token_present=true lease_id=lease-3 public_port=54619 secret rotation pending`
	if got := Text(in, agentToken); got != in {
		t.Fatalf("ordinary diagnostic changed:\n in: %q\nout: %q", in, got)
	}
}

func TestTextIgnoresShortOrEmptyExactValues(t *testing.T) {
	in := "connection refused by peer"
	if got := Text(in, "", "e", "refused"[:4]); got != in {
		t.Fatalf("short exact values must not erase text: %q", got)
	}
}

func TestTextRemovesLongestOverlappingSecretFirst(t *testing.T) {
	long := agentToken + "-suffix-secret"
	got := Text("value="+long, agentToken, long)
	if strings.Contains(got, "suffix") {
		t.Fatalf("overlapping secret partially revealed: %q", got)
	}
}

func TestBoundKeepsValidUTF8AndMarksTruncation(t *testing.T) {
	in := strings.Repeat("خطای اتصال ", 200) // Persian, multi-byte
	got := Bound(in, 101)
	if len(got) > 101 {
		t.Fatalf("len %d > bound", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("split a UTF-8 sequence: %q", got)
	}
	if !strings.HasSuffix(got, TruncationMarker) {
		t.Fatalf("missing truncation marker: %q", got)
	}
	if Bound("short", 100) != "short" {
		t.Fatal("short text must be unchanged")
	}
	if Bound("anything", 0) != "" {
		t.Fatal("non-positive bound must return empty")
	}
	if tiny := Bound(strings.Repeat("x", 50), 4); len(tiny) > 4 || !utf8.ValidString(tiny) {
		t.Fatalf("tiny bound: %q", tiny)
	}
	if got := Bound("bad\xffbyte", 100); !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 not repaired: %q", got)
	}
}

func TestDiagnosticRedactsBeforeTruncating(t *testing.T) {
	// The secret straddles the byte bound. Truncating first would leave a
	// partial token prefix that no longer matches the exact value.
	prefix := strings.Repeat("x", 40)
	in := prefix + agentToken + " tail"
	got := Diagnostic(in, 50, agentToken)
	if strings.Contains(got, agentToken[:6]) {
		t.Fatalf("partial secret leaked across bound: %q", got)
	}
	if len(got) > 50 {
		t.Fatalf("len %d > 50", len(got))
	}
}

func TestApplyCountsByRuleName(t *testing.T) {
	counts := map[string]int{}
	out := Apply("Bearer abcdefgh12 and BAFTPAIR1:QUJDREVGR0hJSktMTU5PUA and Bearer zyxwvuts98", counts)
	if counts["bearer-token"] != 2 || counts["pairing-code"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
	if strings.Contains(out, "abcdefgh12") || strings.Contains(out, "zyxwvuts98") {
		t.Fatalf("not redacted: %q", out)
	}
	if Apply("nothing secret here", counts) != "nothing secret here" || len(counts) != 2 {
		t.Fatalf("clean text changed counts: %v", counts)
	}
}

func FuzzDiagnosticNeverLeaksExactSecret(f *testing.F) {
	f.Add("prefix ", " suffix", 64)
	f.Add("", "", 1)
	f.Add("خطا ", "\xff", 30)
	f.Fuzz(func(t *testing.T, before, after string, max int) {
		if max < 0 || max > 4096 {
			return
		}
		got := Diagnostic(before+agentToken+after, max, agentToken)
		if strings.Contains(got, agentToken) {
			t.Fatalf("exact secret leaked: %q", got)
		}
		if len(got) > max || (max > 0 && !utf8.ValidString(got)) {
			t.Fatalf("bound violated: len=%d max=%d", len(got), max)
		}
	})
}
