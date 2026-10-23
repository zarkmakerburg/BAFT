package agent

import (
	"errors"
	"net"
	"strconv"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// ingressV1Command maps a previously verified signed job into the shared
// immutable resource binding. This pure conversion is NOT signature admission
// and never creates a durable lease, a listener, or a receipt.
func ingressV1Command(j agentjob.Job) (ingressauthority.CommandV1, error) {
	if j.Params["ingress_protocol"] != ingressauthority.ProtocolV1 {
		return ingressauthority.CommandV1{}, errors.New("not operational ingress v1")
	}
	if err := j.Validate(); err != nil {
		return ingressauthority.CommandV1{}, err
	}
	port, err := strconv.Atoi(j.Params["public_port"])
	if err != nil {
		return ingressauthority.CommandV1{}, err
	}
	generation, err := strconv.ParseUint(j.Params["generation"], 10, 64)
	if err != nil {
		return ingressauthority.CommandV1{}, err
	}
	endpoint := j.Params["listen_endpoint"]
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return ingressauthority.CommandV1{}, err
	}
	spec := ingressmanifest.Spec{
		TunnelID: j.Params["tunnel_id"], InstanceID: j.Params["instance_id"], Generation: generation,
		Config: ingressmanifest.Config{
			Listen: endpoint, Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000,
		},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		return ingressauthority.CommandV1{}, err
	}
	if proof.ManifestSHA256 != j.Params["manifest_digest"] ||
		proof.ConfigSHA256 != j.Params["config_sha256"] {
		return ingressauthority.CommandV1{}, errors.New("signed ingress digests differ from canonical manifest/config")
	}
	binding := ingressauthority.BindingV1{
		NodeID: j.NodeID, OperationID: j.Params["operation_id"], LeaseID: j.Params["lease_id"],
		Port: uint16(port), ConfigPort: uint16(port), ListenEndpoint: endpoint,
		LocalTarget: j.Params["local_target"], Transport: j.Params["transport"],
		PlanSHA256: j.Params["plan_sha256"], ListenerID: j.Params["listener_id"],
		FirewallID: j.Params["firewall_id"], Spec: spec, Proof: proof,
	}
	command := ingressauthority.CommandV1{
		IngressProtocol: ingressauthority.ProtocolV1, JobID: j.JobID, Action: j.Action,
		IssuedAt: j.IssuedAt, ExpiresAt: j.ExpiresAt, Binding: binding,
	}
	if err := binding.Validate(); err != nil {
		return ingressauthority.CommandV1{}, err
	}
	return command, nil
}
