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
// to mutate the host; see MintIngressLease for the single-use mint path.
func (a *Agent) CheckSignedHeldIngressAdmission(ctx context.Context, env release.Envelope, command ingressauthority.CommandV1, guard *ingressauthority.PendingGuard) error {
	if _, err := a.admitSignedIngress(ctx, env, command, guard); err != nil {
		return err
	}
	// This is only admission evidence; no reusable approved token escapes.
	return guard.CheckHeldCommand(ctx, command)
}

// MintIngressLease is the only production source of a valid
// *ingressauthority.Lease. It verifies the actual BCC signed envelope against
// this Agent's enrolled pinned key and node, correlates it with the canonical
// command, then asks the held PendingGuard (OS lock, durable claim, pinned
// installed policy, recorded intent, lifecycle state) to mint a single-use
// lease. Replay, expiry, wrong node, generation/binding drift, closed guard
// and unknown owner all fail closed. It performs no host mutation and the
// returned lease is revoked when the guard closes or state becomes uncertain.
func (a *Agent) MintIngressLease(ctx context.Context, env release.Envelope, command ingressauthority.CommandV1, guard *ingressauthority.PendingGuard) (*ingressauthority.Lease, error) {
	verified, err := a.admitSignedIngress(ctx, env, command, guard)
	if err != nil {
		return nil, err
	}
	return guard.MintLease(ctx, verified, command)
}

func (a *Agent) admitSignedIngress(ctx context.Context, env release.Envelope, command ingressauthority.CommandV1, guard *ingressauthority.PendingGuard) (agentjob.VerifiedJob, error) {
	if a == nil || ctx == nil || ctx.Err() != nil || guard == nil {
		return agentjob.VerifiedJob{}, ingressauthority.ErrUnverified
	}
	now := a.cfg.Now
	if now == nil {
		now = time.Now
	}
	verified, err := (agentjob.Verifier{BCCKey: a.cfg.BCCJobKey, NodeID: a.cfg.NodeID}).VerifyAdmitted(env, now())
	if err != nil {
		return agentjob.VerifiedJob{}, err
	}
	job := verified.Job()
	if command.Action != agentjob.ActionPublicIngressApply && command.Action != agentjob.ActionPublicIngressRetire {
		return agentjob.VerifiedJob{}, ingressauthority.ErrBinding
	}
	if command.Binding.NodeID != a.cfg.NodeID {
		return agentjob.VerifiedJob{}, ingressauthority.ErrBinding
	}
	if err := signedJobMatchesCommand(job, command); err != nil {
		return agentjob.VerifiedJob{}, err
	}
	if err := command.Validate(now()); err != nil {
		return agentjob.VerifiedJob{}, err
	}
	return verified, nil
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
