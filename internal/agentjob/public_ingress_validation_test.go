package agentjob

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

func TestPublicIngressEndpointsBeforeSigningAndAgentExecution(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field, value string
		valid        bool
	}{
		{"public_port", "1", true},
		{"public_port", "65535", true},
		{"public_port", "0", false},
		{"public_port", "65536", false},
		{"public_port", "99999", false},
		{"local_target", "127.0.0.1:1", true},
		{"local_target", "[::1]:65535", true},
		{"local_target", "127.0.0.1:0", false},
		{"local_target", "[::1]:65536", false},
		{"local_target", "127.999.0.1:1443", false},
		{"local_target", "192.0.2.1:1443", false},
	} {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			j := job(ActionPublicIngressPreflight, map[string]string{
				"tunnel_id": "tun-test", "public_port": "54619", "local_target": "127.0.0.1:1443",
			})
			j.SchemaVersion = SchemaVersion
			j.Params[tc.field] = tc.value
			_, err := Sign(key, j)
			if (err == nil) != tc.valid {
				t.Fatalf("sign: valid=%v err=%v", tc.valid, err)
			}
			// Bypass Sign's validation to model an older BCC signing malformed
			// parameters. The agent must independently refuse these payloads.
			payload, err := json.Marshal(j)
			if err != nil {
				t.Fatal(err)
			}
			env := release.Sign(PayloadType, payload, key)
			_, err = (Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, now.Add(time.Minute))
			if (err == nil) != tc.valid {
				t.Fatalf("agent verify: valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
