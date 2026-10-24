package agent

import (
	"context"
	"reflect"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

// CheckSignedHeldIngressAdmission authenticates the actual BCC signed envelope
// against the Agent's enrolled pinned key and independently checks the durable
// immutable command under its OS guard. It never returns a Lease or permission
// to mutate the host. The future mint path must additionally obtain a PRIVATE
// Delta execution pin while this guard is held and enforce it at every effect.
func (a *Agent) CheckSignedHeldIngressAdmission(ctx context.Context, env release.Envelope, command ingressauthority.CommandV1, guard *ingressauthority.PendingGuard) error {
	if a == nil || ctx == nil || ctx.Err() != nil || guard == nil {
		return ingressauthority.ErrUnverified
	}
	now := a.cfg.Now
	if now == nil {
		now = time.Now
	}
	job, err := (agentjob.Verifier{BCCKey: a.cfg.BCCJobKey, NodeID: a.cfg.NodeID}).Verify(env, now())
	if err != nil {
		return err
	}
	if command.Action != agentjob.ActionPublicIngressApply && command.Action != agentjob.ActionPublicIngressRetire {
		return ingressauthority.ErrBinding
	}
	if err := signedJobMatchesCommand(job, command); err != nil {
		return err
	}
	if err := command.Validate(now()); err != nil {
		return err
	}
	if err := guard.CheckHeldCommand(ctx, command); err != nil {
		return err
	}
	// This is only admission evidence; no reusable approved token escapes.
	return nil
}

// signedJobMatchesCommand correlates the verified signed job with the canonical
// command derived from that same job by ingressV1Command, so optional params
// (instance_id) and unrecognized fields follow the one shared parser instead of
// a parallel rigid schema.
func signedJobMatchesCommand(job agentjob.Job, command ingressauthority.CommandV1) error {
	canonical, err := ingressV1Command(job)
	if err != nil {
		return ingressauthority.ErrBinding
	}
	if !canonical.IssuedAt.Equal(command.IssuedAt) || !canonical.ExpiresAt.Equal(command.ExpiresAt) {
		return ingressauthority.ErrBinding
	}
	canonical.IssuedAt, canonical.ExpiresAt = time.Time{}, time.Time{}
	command.IssuedAt, command.ExpiresAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(canonical, command) {
		return ingressauthority.ErrBinding
	}
	return nil
}
