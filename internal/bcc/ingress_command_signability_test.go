package bcc

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// BCC must not durably review a command whose signed envelope would carry a
// different lifetime or could never be admitted by the normal Agent.
func TestStagedIngressCommandRejectsLossySigningBeforePersistence(t *testing.T) {
	for _, name := range []string{"fractional-issued", "fractional-expiry", "excessive-lifetime"} {
		t.Run(name, func(t *testing.T) {
			s, spec, cmd, _ := stagedIngressBoundary(t)
			s.mu.Lock()
			op := s.st.PublicIngressOperations[spec.TunnelID]
			op.Command = nil
			s.st.PublicIngressOperations[spec.TunnelID] = op
			if err := s.saveLocked(); err != nil {
				s.mu.Unlock()
				t.Fatal(err)
			}
			s.mu.Unlock()
			now := time.Now().UTC().Truncate(time.Second)
			cmd.IssuedAt = now.Add(-time.Second)
			cmd.ExpiresAt = now.Add(time.Minute)
			switch name {
			case "fractional-issued":
				cmd.IssuedAt = cmd.IssuedAt.Add(time.Nanosecond)
			case "fractional-expiry":
				cmd.ExpiresAt = cmd.ExpiresAt.Add(time.Nanosecond)
			case "excessive-lifetime":
				cmd.ExpiresAt = cmd.IssuedAt.Add(agentjob.MaxLifetime + time.Second)
			}
			before := stateJSONForTest(t, s)
			if err := s.StageReviewedIngressCommand(spec.TunnelID, cmd, now); err == nil {
				t.Fatal("persisted a command normal signing cannot preserve")
			}
			if string(before) != string(stateJSONForTest(t, s)) {
				t.Fatal("refused command modified state")
			}
			reopened, err := OpenStore(s.Path())
			if err != nil {
				t.Fatal(err)
			}
			op, _ = reopened.GetPublicIngressOperation(spec.TunnelID)
			if op.Command != nil || op.Dispatch != nil || len(reopened.st.Jobs) != 0 {
				t.Fatal("refused command became durable or executable")
			}
		})
	}
}

// Ported from hq's TestCommandJobRefusesUnrepresentableIdentity: tuning,
// generation and target drift that the signed v1 job schema cannot carry must
// be refused at staging, before anything becomes durable.
func TestStagedIngressCommandRejectsUnrepresentableIdentityBeforePersistence(t *testing.T) {
	cases := map[string]func(*ingressauthority.CommandV1){
		"unsupported-tuning": func(c *ingressauthority.CommandV1) {
			c.Binding.Spec.Config.MaxClients++
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
		"unsupported-generation": func(c *ingressauthority.CommandV1) {
			c.Binding.Spec.Generation++
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
		"self-forward": func(c *ingressauthority.CommandV1) {
			c.Binding.LocalTarget = c.Binding.ListenEndpoint
			c.Binding.Spec.Config.Target = c.Binding.LocalTarget
			c.Binding.Proof, _ = ingressmanifest.Preview(c.Binding.Spec)
		},
		"inspect-is-not-mutation": func(c *ingressauthority.CommandV1) { c.Action = agentjob.ActionPublicIngressInspect },
		"retire-needs-own-path":   func(c *ingressauthority.CommandV1) { c.Action = agentjob.ActionPublicIngressRetire },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			s, spec, cmd, _ := stagedIngressBoundary(t)
			s.mu.Lock()
			op := s.st.PublicIngressOperations[spec.TunnelID]
			op.Command = nil
			s.st.PublicIngressOperations[spec.TunnelID] = op
			if err := s.saveLocked(); err != nil {
				s.mu.Unlock()
				t.Fatal(err)
			}
			s.mu.Unlock()
			alter(&cmd)
			before := stateJSONForTest(t, s)
			if err := s.StageReviewedIngressCommand(spec.TunnelID, cmd, time.Now().UTC().Truncate(time.Second)); err == nil {
				t.Fatal("persisted an unrepresentable command")
			}
			if string(before) != string(stateJSONForTest(t, s)) {
				t.Fatal("refused command modified state")
			}
		})
	}
}
