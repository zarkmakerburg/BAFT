package agentjob

import (
	"strings"
	"testing"
)

func validIngressV1Job(action string) Job {
	j := job(action, map[string]string{
		"tunnel_id": "tunnel-1", "public_port": "443", "local_target": "127.0.0.1:1443",
		"generation": "1", "manifest_digest": strings.Repeat("a", 64),
		"listener_id": "listener-1", "firewall_id": "firewall-1",
		"ingress_protocol": "1", "operation_id": "op-1", "lease_id": "lease-1",
		"plan_sha256": strings.Repeat("b", 64), "config_sha256": strings.Repeat("c", 64),
		"listen_endpoint": "0.0.0.0:443", "transport": "dedicated",
	})
	j.SchemaVersion = SchemaVersion
	return j
}

func TestSignedOperationalIngressV1StrictParamBoundary(t *testing.T) {
	j := validIngressV1Job(ActionPublicIngressApply)
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ingress_protocol", "operation_id", "lease_id", "plan_sha256", "config_sha256", "listen_endpoint", "transport"} {
		altered := j
		altered.Params = make(map[string]string, len(j.Params))
		for k, v := range j.Params {
			altered.Params[k] = v
		}
		delete(altered.Params, field)
		if err := altered.Validate(); err == nil {
			t.Fatalf("missing %s admitted", field)
		}
	}
}

func TestOperationalIngressV1RejectsAliasedPortAndUnprovenTransport(t *testing.T) {
	original := validIngressV1Job(ActionPublicIngressRetire)
	for _, mutation := range []struct{ name, key, value string }{
		{"changed listen", "listen_endpoint", "0.0.0.0:2053"},
		{"noncanonical listen", "listen_endpoint", "0.0.0.0:0443"},
		{"mux transport", "transport", "mux"},
		{"unknown version", "ingress_protocol", "2"},
		{"invalid lease", "lease_id", "../other"},
		{"unsupported generation", "generation", "2"},
		{"SSH port", "public_port", "22"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			other := original
			other.Params = make(map[string]string, len(original.Params))
			for k, v := range original.Params {
				other.Params[k] = v
			}
			other.Params[mutation.key] = mutation.value
			if err := other.Validate(); err == nil {
				t.Fatal("unreviewed operational mutation admitted")
			}
		})
	}
}
