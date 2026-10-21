package agentjob

import (
	"strings"
	"testing"
	"time"
)

// Until ownership-checked listener, firewall and rollback executors exist,
// signed BCC jobs must not be able to request any public ingress mutation.
// This gate deliberately fails if a future change enables an action without
// first replacing this contract with an end-to-end mutation acceptance test.
func TestP0PublicIngressMutationActionsRemainDisabled(t *testing.T) {
	now := time.Now().UTC()
	for _, action := range []string{
		"public_ingress_prepare",
		"public_ingress_commit",
		"public_ingress_apply",
		"public_ingress_rollback",
		"public_ingress_retire",
		"public_ingress_remove",
		"public_ingress_reconcile",
	} {
		t.Run(action, func(t *testing.T) {
			job := Job{SchemaVersion: SchemaVersion, JobID: "ingress-mutation-test", NodeID: "ir-test", Action: action,
				Params:   map[string]string{"tunnel_id": "tun-a", "public_port": "2053", "local_target": "127.0.0.1:1443"},
				IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
			if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("unimplemented mutation action was enabled: %s err=%v", action, err)
			}
		})
	}
}
