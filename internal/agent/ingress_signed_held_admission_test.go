package agent

import (
	"testing"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestSignedJobCorrelatesWithCanonicalCommandWithoutInstanceID(t *testing.T) {
	job := validSignedIngressV1(t) // optional instance_id omitted
	command, err := ingressV1Command(job)
	if err != nil {
		t.Fatal(err)
	}
	if err := signedJobMatchesCommand(job, command); err != nil {
		t.Fatalf("canonical 14-field job refused: %v", err)
	}
	job.Params["instance_id"] = "inst-1"
	proof, err := ingressmanifest.Preview(ingressmanifest.Spec{TunnelID: "tunnel-1", InstanceID: "inst-1", Generation: 1,
		Config: ingressmanifest.Config{Listen: "0.0.0.0:443", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}})
	if err != nil {
		t.Fatal(err)
	}
	job.Params["manifest_digest"], job.Params["config_sha256"] = proof.ManifestSHA256, proof.ConfigSHA256
	withInstance, err := ingressV1Command(job)
	if err != nil {
		t.Fatal(err)
	}
	if err := signedJobMatchesCommand(job, withInstance); err != nil {
		t.Fatalf("job with instance_id refused: %v", err)
	}
	if err := signedJobMatchesCommand(job, command); err == nil {
		t.Fatal("command lacking signed instance_id accepted")
	}
}

func TestSignedJobCorrelationRefusesDriftAndUnknownFields(t *testing.T) {
	job := validSignedIngressV1(t)
	command, _ := ingressV1Command(job)
	drift := command
	drift.Binding.LeaseID = "lease-other"
	if err := signedJobMatchesCommand(job, drift); err == nil {
		t.Fatal("lease drift accepted")
	}
	extra := job
	extra.Params = map[string]string{}
	for k, v := range job.Params {
		extra.Params[k] = v
	}
	extra.Params["surprise"] = "x"
	if err := signedJobMatchesCommand(extra, command); err == nil {
		t.Fatal("unrecognized signed field accepted")
	}
	retire := job
	retire.Action = agentjob.ActionPublicIngressRetire
	if err := signedJobMatchesCommand(retire, command); err == nil {
		t.Fatal("action drift accepted")
	}
}
