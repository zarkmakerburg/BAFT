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
		{"public_port", "1", false},
		{"public_port", "65535", true},
		{"public_port", "2053", true},
		{"public_port", "1023", false},
		{"public_port", "1024", true},
		{"public_port", "0", false},
		{"public_port", "65536", false},
		{"public_port", "99999", false},
		{"local_target", "127.0.0.1:1", true},
		{"local_target", "127.0.0.1:22", false},
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

func TestSignedAgentJobRejectsTrailingJSONValues(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	j := job(ActionPublicIngressPreflight, map[string]string{
		"tunnel_id": "tun-test", "public_port": "54619", "local_target": "127.0.0.1:1443",
	})
	j.SchemaVersion = SchemaVersion
	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{" {}", " null", " true", " []", " 42", " \"extra\""} {
		t.Run(suffix, func(t *testing.T) {
			env := release.Sign(PayloadType, append(append([]byte(nil), raw...), []byte(suffix)...), key)
			if _, err := (Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, now.Add(time.Minute)); err == nil {
				t.Fatal("accepted signed payload with trailing JSON value")
			}
		})
	}
}

func TestAllPublicIngressActionsRejectSelfForwarding(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{ActionPublicIngressPreflight, ActionPublicIngressInspect, ActionPublicIngressPrepare, ActionPublicIngressCancel} {
		for _, target := range []string{"127.0.0.1:54619", "127.0.0.2:54619", "[::1]:54619"} {
			t.Run(action+"/"+target, func(t *testing.T) {
				params := map[string]string{"tunnel_id": "tun-test", "public_port": "54619", "local_target": target}
				if action != ActionPublicIngressPreflight {
					params["generation"] = "1"
					params["manifest_digest"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					params["listener_id"] = "listener-test"
					params["firewall_id"] = "firewall-test"
				}
				j := job(action, params)
				j.SchemaVersion = SchemaVersion
				if _, err := Sign(key, j); err == nil {
					t.Fatal("self-forwarding signed")
				}
				raw, err := json.Marshal(j)
				if err != nil {
					t.Fatal(err)
				}
				env := release.Sign(PayloadType, raw, key)
				if _, err := (Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, now.Add(time.Minute)); err == nil {
					t.Fatal("self-forwarding accepted by agent")
				}
			})
		}
	}
}

func TestPublicIngressSignedLiveActionsRequireCanonicalParameters(t *testing.T) {
	for _, action := range []string{"public_ingress_apply", "public_ingress_retire"} {
		t.Run(action, func(t *testing.T) {
			params := map[string]string{"tunnel_id": "tun-test", "public_port": "54619", "local_target": "127.0.0.1:1443", "generation": "1", "manifest_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "listener_id": "listener-test", "firewall_id": "firewall-test"}
			j := job(action, params)
			j.SchemaVersion = SchemaVersion
			if err := j.Validate(); err != nil {
				t.Fatalf("valid live job rejected: %v", err)
			}
			for _, field := range []string{"manifest_digest", "listener_id", "firewall_id", "generation"} {
				bad := job(action, params)
				bad.SchemaVersion = SchemaVersion
				bad.Params = make(map[string]string, len(params))
				for k, v := range params {
					bad.Params[k] = v
				}
				delete(bad.Params, field)
				if err := bad.Validate(); err == nil {
					t.Fatalf("missing %s accepted", field)
				}
			}
			j.Params["local_target"] = "127.0.0.1:54619"
			if err := j.Validate(); err == nil {
				t.Fatal("self-forwarding accepted")
			}
		})
	}
}
