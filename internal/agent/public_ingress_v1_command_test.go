package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func validSignedIngressV1(t *testing.T) agentjob.Job {
	t.Helper()
	spec := ingressmanifest.Spec{TunnelID: "tunnel-1", Generation: 1,
		Config: ingressmanifest.Config{Listen: "0.0.0.0:443", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "apply-1", NodeID: "ir-1",
		Action: agentjob.ActionPublicIngressApply, IssuedAt: now.Add(-time.Second),
		ExpiresAt: now.Add(time.Minute),
		Params: map[string]string{
			"ingress_protocol": "1", "tunnel_id": spec.TunnelID, "generation": "1",
			"public_port": "443", "local_target": spec.Config.Target,
			"manifest_digest": proof.ManifestSHA256, "config_sha256": proof.ConfigSHA256,
			"listen_endpoint": spec.Config.Listen, "operation_id": "op-1", "lease_id": "lease-1",
			"plan_sha256": strings.Repeat("a", 64), "transport": "dedicated",
			"listener_id": "listener-1", "firewall_id": "firewall-1",
		},
	}
}

func TestSignedIngressV1CanonicalCommandBinding(t *testing.T) {
	j := validSignedIngressV1(t)
	command, err := ingressV1Command(j)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if command.Binding.Port != 443 || command.Binding.ConfigPort != 443 ||
		command.Binding.NodeID != j.NodeID || command.Binding.LeaseID != "lease-1" {
		t.Fatal("reviewed identity diverged from signed job")
	}
	other := j
	other.JobID = "retire-2"
	other.Action = agentjob.ActionPublicIngressRetire
	retire, err := ingressV1Command(other)
	if err != nil {
		t.Fatal(err)
	}
	if retire.Binding != command.Binding {
		t.Fatal("new signed retire job changed resource lease")
	}
}

func TestSignedIngressV1RejectsDigestAndPortMismatch(t *testing.T) {
	j := validSignedIngressV1(t)
	for _, field := range []string{"manifest_digest", "config_sha256", "public_port", "listen_endpoint", "local_target", "plan_sha256"} {
		other := j
		other.Params = make(map[string]string, len(j.Params))
		for k, v := range j.Params {
			other.Params[k] = v
		}
		if strings.Contains(field, "digest") || strings.Contains(field, "sha256") {
			other.Params[field] = strings.Repeat("b", 64)
		} else {
			other.Params[field] = "127.0.0.1:2443"
		}
		candidate, err := ingressV1Command(other)
		if field == "plan_sha256" {
			// A well-formed plan hash must be matched against an independent
			// approved review; the signed job alone cannot attest that review.
			approved, approvedErr := ingressV1Command(j)
			if err != nil || approvedErr != nil || candidate.Binding.MatchReviewed(approved.Binding) == nil {
				t.Fatal("changed plan hash was accepted as the approved review")
			}
			continue
		}
		if err == nil {
			t.Fatalf("unreviewed %s admitted", field)
		}
	}
}
