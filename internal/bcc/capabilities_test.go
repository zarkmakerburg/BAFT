package bcc

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/verification"
)

func TestVerificationCapabilitiesRequireAuthenticationAndRevokeOnDowngrade(t *testing.T) {
	s := tunnelStore(t)
	n := s.st.Nodes["ex-1"]
	n.TunnelVerification = 0
	n.CapabilitiesAt = time.Time{}
	s.st.Nodes[n.ID] = n
	if _, err := s.PullJobsWithCapabilities(n.ID, "wrong", verification.Capability); err == nil {
		t.Fatal("unauthenticated capability accepted")
	}
	if s.st.Nodes[n.ID].TunnelVerification != 0 {
		t.Fatal("unauthenticated request changed capabilities")
	}
	if _, err := s.PullJobsWithCapabilities(n.ID, "tok-"+n.ID, verification.Capability); err != nil {
		t.Fatal(err)
	}
	if !supportsTunnelVerification(s.st.Nodes[n.ID], time.Now().UTC()) {
		t.Fatal("authenticated capability not recorded")
	}
	if _, err := s.PullJobsWithCapabilities(n.ID, "tok-"+n.ID, ""); err != nil {
		t.Fatal(err)
	}
	if s.st.Nodes[n.ID].TunnelVerification != 0 {
		t.Fatal("old agent retained a newer binary's claim")
	}
	again, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if again.st.Nodes[n.ID].TunnelVerification != 0 {
		t.Fatal("downgrade was not persisted")
	}
}

func TestVerificationCapabilitiesDoNotMoveToAnotherHostOrCredential(t *testing.T) {
	for _, change := range []string{"host", "token", "role"} {
		s := tunnelStore(t)
		n := s.st.Nodes["ex-1"]
		token := ""
		switch change {
		case "host":
			n.Address = "203.0.113.9:22"
		case "token":
			token = "new-token"
		case "role":
			n.Role = "worker"
		}
		if _, err := s.UpsertNode(n, token); err != nil {
			t.Fatal(err)
		}
		if s.st.Nodes[n.ID].TunnelVerification != 0 || !s.st.Nodes[n.ID].CapabilitiesAt.IsZero() {
			t.Fatalf("%s change kept an old capability", change)
		}
	}
}
