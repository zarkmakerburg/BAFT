package agentjob

import (
	"strings"
	"testing"
)

func TestSignedCandidateActivationBindsDigestAndGeneration(t *testing.T) {
	v, sign := setup(t)
	p := map[string]string{"tunnel_id": "t-1", "instance_id": "ex-bg", "config_sha256": strings.Repeat("a", 64), "generation": "0"}
	if _, err := v.Verify(sign(job(ActionTunnelCommitCandidate, p)), now); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"config_sha256", "generation", "tunnel_id"} {
		q := map[string]string{}
		for k, val := range p {
			if k != key {
				q[k] = val
			}
		}
		j := job(ActionTunnelCommitCandidate, q)
		j.SchemaVersion = SchemaVersion
		if j.Validate() == nil {
			t.Fatalf("activation accepted without %s", key)
		}
	}
	for _, gen := range []string{"-1", "01", "1000000000", "not-a-number"} {
		p["generation"] = gen
		j := job(ActionTunnelCommitCandidate, p)
		j.SchemaVersion = SchemaVersion
		if j.Validate() == nil {
			t.Fatalf("invalid generation accepted: %s", gen)
		}
	}
}
