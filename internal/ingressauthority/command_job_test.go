package ingressauthority

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestCommandJobSignedRoundTrip(t *testing.T) {
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, instance := range []string{"", "second-instance"} {
		for _, action := range []string{agentjob.ActionPublicIngressApply, agentjob.ActionPublicIngressRetire} {
			b := bindingFixture(t)
			b.Spec.InstanceID = instance
			b.Proof, err = ingressmanifest.Preview(b.Spec)
			if err != nil {
				t.Fatal(err)
			}
			c := CommandV1{IngressProtocol: ProtocolV1, JobID: "job-1", Action: action, IssuedAt: now, ExpiresAt: now.Add(time.Minute), Binding: b}
			job, err := c.AgentJob()
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := job.Params["instance_id"]; exists != (instance != "") {
				t.Fatal("optional instance encoding changed")
			}
			envelope, err := agentjob.Sign(key, job)
			if err != nil {
				t.Fatal(err)
			}
			verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: b.NodeID}).Verify(envelope, now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := CommandFromJob(verified)
			if err != nil || got != c {
				t.Fatalf("signed command lost persisted identity: %+v %v", got, err)
			}
			job.Params["lease_id"] = "changed-lease"
			again, err := c.AgentJob()
			if err != nil || again.Params["lease_id"] != b.LeaseID {
				t.Fatal("returned job aliases immutable command")
			}
		}
	}
}

func TestCommandJobRefusesUnrepresentableIdentity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	base := CommandV1{IngressProtocol: ProtocolV1, JobID: "job-1", Action: agentjob.ActionPublicIngressApply, IssuedAt: now, ExpiresAt: now.Add(time.Minute), Binding: bindingFixture(t)}
	cases := map[string]func(*CommandV1){
		"fractional-issued":       func(c *CommandV1) { c.IssuedAt = c.IssuedAt.Add(time.Nanosecond) },
		"fractional-expiry":       func(c *CommandV1) { c.ExpiresAt = c.ExpiresAt.Add(time.Nanosecond) },
		"too-long":                func(c *CommandV1) { c.ExpiresAt = c.IssuedAt.Add(agentjob.MaxLifetime + time.Second) },
		"inspect-is-not-mutation": func(c *CommandV1) { c.Action = agentjob.ActionPublicIngressInspect },
		"unsupported-tuning": func(c *CommandV1) {
			c.Binding.Spec.Config.MaxClients++
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
		"unsupported-generation": func(c *CommandV1) {
			c.Binding.Spec.Generation++
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
		"self-forward": func(c *CommandV1) {
			c.Binding.LocalTarget = "127.0.0.1:1443"
			c.Binding.Spec.Config.Target = c.Binding.LocalTarget
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			c := base
			alter(&c)
			if _, err := c.AgentJob(); err == nil {
				t.Fatal("unrepresentable command accepted")
			}
		})
	}
	j, err := base.AgentJob()
	if err != nil {
		t.Fatal(err)
	}
	j.Params["unknown"] = "1"
	if _, err := CommandFromJob(j); err == nil {
		t.Fatal("unknown signed parameter accepted")
	}
	delete(j.Params, "unknown")
	j.Action = agentjob.ActionPublicIngressPrepare
	if _, err := CommandFromJob(j); err == nil {
		t.Fatal("legacy action entered operational converter")
	}
}
