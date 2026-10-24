package agent

import (
 "context"
 "strconv"
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
 if now == nil { now = time.Now }
 job, err := (agentjob.Verifier{BCCKey: a.cfg.BCCJobKey, NodeID: a.cfg.NodeID}).Verify(env, now())
 if err != nil { return err }
 if command.Action != agentjob.ActionPublicIngressApply && command.Action != agentjob.ActionPublicIngressRetire {
  return ingressauthority.ErrBinding
 }
 b := command.Binding
 if job.JobID != command.JobID || job.Action != command.Action || job.NodeID != b.NodeID ||
  !job.IssuedAt.Equal(command.IssuedAt) || !job.ExpiresAt.Equal(command.ExpiresAt) {
  return ingressauthority.ErrBinding
 }
 expected := map[string]string{
  "tunnel_id":b.Spec.TunnelID, "instance_id":b.Spec.InstanceID,
  "public_port":strconv.Itoa(int(b.Port)), "local_target":b.LocalTarget,
  "generation":strconv.FormatUint(b.Spec.Generation,10),
  "manifest_digest":b.Proof.ManifestSHA256,
  "listener_id":b.ListenerID, "firewall_id":b.FirewallID,
  "ingress_protocol":command.IngressProtocol, "operation_id":b.OperationID,
  "lease_id":b.LeaseID, "plan_sha256":b.PlanSHA256,
  "config_sha256":b.Proof.ConfigSHA256,
  "listen_endpoint":b.ListenEndpoint, "transport":b.Transport,
 }
 if len(job.Params) != len(expected) { return ingressauthority.ErrBinding }
 for k, value := range expected {
  if job.Params[k] != value { return ingressauthority.ErrBinding }
 }
 if err := command.Validate(now()); err != nil { return err }
 if err := guard.CheckHeldCommand(ctx,command); err != nil { return err }
 // This is only admission evidence; no reusable approved token escapes.
 return nil
}
