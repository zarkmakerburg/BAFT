package ingressauthority

import (
	"errors"
	"net"
	"strconv"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// CommandFromJob maps a previously verified signed job into the shared
// immutable resource binding. This pure conversion is NOT signature admission
// and never creates a durable lease, a listener, or a receipt.
func CommandFromJob(j agentjob.Job) (CommandV1, error) {
	if (j.Action != agentjob.ActionPublicIngressApply && j.Action != agentjob.ActionPublicIngressRetire) || j.Params["ingress_protocol"] != ProtocolV1 {
		return CommandV1{}, errors.New("not operational ingress v1")
	}
	if err := j.Validate(); err != nil {
		return CommandV1{}, err
	}
	port, err := strconv.Atoi(j.Params["public_port"])
	if err != nil {
		return CommandV1{}, err
	}
	generation, err := strconv.ParseUint(j.Params["generation"], 10, 64)
	if err != nil {
		return CommandV1{}, err
	}
	endpoint := j.Params["listen_endpoint"]
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return CommandV1{}, err
	}
	spec := ingressmanifest.Spec{
		TunnelID: j.Params["tunnel_id"], InstanceID: j.Params["instance_id"], Generation: generation,
		Config: ingressmanifest.Config{
			Listen: endpoint, Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000,
		},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		return CommandV1{}, err
	}
	if proof.ManifestSHA256 != j.Params["manifest_digest"] ||
		proof.ConfigSHA256 != j.Params["config_sha256"] {
		return CommandV1{}, errors.New("signed ingress digests differ from canonical manifest/config")
	}
	binding := BindingV1{
		NodeID: j.NodeID, OperationID: j.Params["operation_id"], LeaseID: j.Params["lease_id"],
		Port: uint16(port), ConfigPort: uint16(port), ListenEndpoint: endpoint,
		LocalTarget: j.Params["local_target"], Transport: j.Params["transport"],
		PlanSHA256: j.Params["plan_sha256"], ListenerID: j.Params["listener_id"],
		FirewallID: j.Params["firewall_id"], Spec: spec, Proof: proof,
	}
	command := CommandV1{
		IngressProtocol: ProtocolV1, JobID: j.JobID, Action: j.Action,
		IssuedAt: j.IssuedAt, ExpiresAt: j.ExpiresAt, Binding: binding,
	}
	if err := binding.Validate(); err != nil {
		return CommandV1{}, err
	}
	return command, nil
}

// AgentJob encodes an immutable command for BCC signing. This conversion does
// not sign, enqueue, authorize execution, or attest physical host ownership.
// Only fields represented by the signed v1 job are accepted; silently replacing
// tuning or rounding command times would detach its persisted identity.
func (c CommandV1) AgentJob() (agentjob.Job, error) {
	if err := c.Validate(c.IssuedAt); err != nil {
		return agentjob.Job{}, err
	}
	if c.IssuedAt.Nanosecond() != 0 || c.ExpiresAt.Nanosecond() != 0 {
		return agentjob.Job{}, errors.New("signed ingress command times must have whole-second precision")
	}
	b := c.Binding
	j := agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: c.JobID, NodeID: b.NodeID, Action: c.Action, IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt,
		Params: map[string]string{
			"ingress_protocol": ProtocolV1, "operation_id": b.OperationID, "lease_id": b.LeaseID, "plan_sha256": b.PlanSHA256,
			"tunnel_id": b.Spec.TunnelID, "generation": strconv.FormatUint(b.Spec.Generation, 10), "public_port": strconv.Itoa(int(b.Port)),
			"local_target": b.LocalTarget, "listen_endpoint": b.ListenEndpoint, "transport": b.Transport,
			"manifest_digest": b.Proof.ManifestSHA256, "config_sha256": b.Proof.ConfigSHA256,
			"listener_id": b.ListenerID, "firewall_id": b.FirewallID,
		}}
	if b.Spec.InstanceID != "" {
		j.Params["instance_id"] = b.Spec.InstanceID
	}
	decoded, err := CommandFromJob(j)
	if err != nil {
		return agentjob.Job{}, err
	}
	if decoded.Binding != c.Binding || decoded.JobID != c.JobID || decoded.Action != c.Action || !decoded.IssuedAt.Equal(c.IssuedAt) || !decoded.ExpiresAt.Equal(c.ExpiresAt) {
		return agentjob.Job{}, errors.New("command cannot be represented by canonical ingress v1 job")
	}
	return j, nil
}
