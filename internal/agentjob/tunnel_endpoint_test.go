package agentjob

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

func TestTunnelEndpointsBeforeSigningAndAgentExecution(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		action, field, value string
		valid                bool
	}{
		{ActionTunnelPrepareEX, "target", "192.0.2.40:1", true},
		{ActionTunnelPrepareEX, "target", "[2001:db8::40]:65535", true},
		{ActionTunnelPrepareEX, "target", "192.0.2.40:0", false},
		{ActionTunnelPrepareEX, "target", "[2001:db8::40]:65536", false},
		{ActionTunnelPrepareEX, "target", "999.1.2.3:443", false},
		{ActionTunnelPrepareEX, "target", "[::::]:443", false},
		{ActionTunnelPrepareEX, "port", "1", true},
		{ActionTunnelPrepareEX, "port", "65535", true},
		{ActionTunnelPrepareEX, "port", "0", false},
		{ActionTunnelPrepareEX, "port", "65536", false},
		{ActionTunnelPrepareEX, "metrics_listen", "[::1]:65535", true},
		{ActionTunnelPrepareEX, "metrics_listen", "127.0.0.1:0", false},
		{ActionTunnelPrepareEX, "metrics_listen", "127.0.0.1:65536", false},
		{ActionTunnelPrepareEX, "metrics_listen", "127.999.0.1:9201", false},
		{ActionTunnelPrepareIR, "route_listen", "[::1]:1", true},
		{ActionTunnelPrepareIR, "route_listen", "127.0.0.1:0", false},
		{ActionTunnelPrepareIR, "route_listen", "[::1]:65536", false},
		{ActionTunnelPrepareIR, "route_listen", "127.888.0.1:1443", false},
		{ActionTunnelPrepareIR, "metrics_listen", "127.0.0.1:1", true},
		{ActionTunnelPrepareIR, "metrics_listen", "[::1]:0", false},
		{ActionTunnelPrepareIR, "metrics_listen", "[::1]:65536", false},
	} {
		t.Run(tc.action+"/"+tc.field+"/"+tc.value, func(t *testing.T) {
			params := map[string]string{
				"tunnel_id": "tun-test", "public_address": "203.0.113.8", "port": "443",
				"target": "192.0.2.40:2443", "route_id": "service-main", "record_shaping": "false",
			}
			if tc.action == ActionTunnelPrepareIR {
				params = map[string]string{"tunnel_id": "tun-test", "route_id": "service-main", "code": "BAFTPAIR1:AAAAAAAAAAAAAAAAAAAAAAAAAAAA", "route_listen": "127.0.0.1:1443"}
			}
			params[tc.field] = tc.value
			j := job(tc.action, params)
			j.SchemaVersion = SchemaVersion
			_, signErr := Sign(key, j)
			if (signErr == nil) != tc.valid {
				t.Fatalf("sign: valid=%v err=%v", tc.valid, signErr)
			}
			// Even a correctly signed malformed payload must be refused by an
			// agent, including jobs produced by an older BCC's loose validator.
			payload, err := json.Marshal(j)
			if err != nil {
				t.Fatal(err)
			}
			env := release.Sign(PayloadType, payload, key)
			_, verifyErr := (Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, now.Add(time.Minute))
			if (verifyErr == nil) != tc.valid {
				t.Fatalf("agent verify: valid=%v err=%v", tc.valid, verifyErr)
			}
		})
	}
}
