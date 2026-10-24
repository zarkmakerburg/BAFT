package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

func TestOperationalSignedIngressCannotUseLABExecutor(t *testing.T) {
	for _, action := range []string{agentjob.ActionPublicIngressApply, agentjob.ActionPublicIngressRetire} {
		j := agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "op-job", NodeID: "ir-1", Action: action,
			IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute),
			Params: map[string]string{
				"tunnel_id": "tun-1", "public_port": "443", "local_target": "127.0.0.1:1443",
				"generation": "1", "manifest_digest": strings.Repeat("a", 64),
				"listener_id": "listener-1", "firewall_id": "fw-1", "ingress_protocol": "1",
				"operation_id": "op-1", "lease_id": "lease-1", "plan_sha256": strings.Repeat("b", 64),
				"config_sha256": strings.Repeat("c", 64), "listen_endpoint": "0.0.0.0:443",
				"transport": "dedicated",
			}}
		if err := j.Validate(); err != nil {
			t.Fatal(err)
		}
		a := &Agent{}
		if _, err := a.publicIngressLAB(context.Background(), j); err == nil ||
			!strings.Contains(err.Error(), "durable authority") {
			t.Fatalf("operational v1 was not fenced from LAB executor: %v", err)
		}
	}
}
