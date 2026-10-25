package protocol

import (
	"fmt"
	"strings"
	"testing"
)

func helloWithCaps(caps string) []byte {
	return []byte(`{"protocol_min":1,"protocol_max":1,"node_id":"ir-01","boot_id":"00112233445566778899aabbccddeeff","session_id":"ffeeddccbbaa99887766554433221100","shard_id":0,"epoch":"1","mode":"new","resume_snapshot_id":null,"profile_id":"secure-fast","profile_version":1,"config_revision":"test","capabilities":` + caps + `}`)
}

// Capability tokens are peer-controlled input to a security negotiation: a
// malformed, ambiguous or oversized list is refused, never partially honoured.
func TestHelloCapabilitiesAreCanonicalOrRefused(t *testing.T) {
	good := []string{`[]`, `["noise"]`, `["noise","ws.fallback","a-b_c.9"]`}
	for _, c := range good {
		if _, err := DecodeHello(helloWithCaps(c)); err != nil {
			t.Errorf("%s refused: %v", c, err)
		}
	}
	bad := map[string]string{
		"duplicate":        `["noise","noise"]`,
		"empty token":      `[""]`,
		"uppercase":        `["Noise"]`,
		"whitespace":       `["no ise"]`,
		"control":          `["no\u0000ise"]`,
		"non ascii":        `["nöise"]`,
		"too long":         `["` + strings.Repeat("a", 33) + `"]`,
		"null":             `null`,
		"leading dash":     `["-noise"]`,
		"case alias dup":   `["noise","NOISE"]`,
		"17 distinct caps": fmt.Sprintf(`[%s]`, manyCaps(17)),
	}
	for name, c := range bad {
		if _, err := DecodeHello(helloWithCaps(c)); err == nil {
			t.Errorf("%s: %s accepted", name, c)
		}
	}
}

func manyCaps(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf(`"c%d"`, i))
	}
	return strings.Join(parts, ",")
}
