package agent

import (
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// ingressV1Command maps a previously verified signed job into the shared
// immutable resource binding. This pure conversion is NOT signature admission
// and never creates a durable lease, a listener, or a receipt.
func ingressV1Command(j agentjob.Job) (ingressauthority.CommandV1, error) {
	return ingressauthority.CommandFromJob(j)
}
