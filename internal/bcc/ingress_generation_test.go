package bcc

import (
	"path/filepath"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// A new operation on a port whose previous ingress was RETIRED carries a
// strictly higher generation; an unretired or other-port operation does not
// raise it.
func TestNextIngressGenerationFollowsRetiredOperationsOnly(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.nextIngressGenerationLocked("ir-1", 2053); g != 1 {
		t.Fatalf("fresh port: %d", g)
	}
	mk := func(tun string, port int, gen uint64, retired bool) PublicIngressOperation {
		op := PublicIngressOperation{Spec: PublicIngressSpec{IRNode: "ir-1", TunnelID: tun, PublicPort: port}, Proof: PublicIngressExpectedProof{Generation: gen}}
		if retired {
			op.RetireReceipt = &ingressauthority.ReceiptV1{Phase: "retired"}
		}
		return op
	}
	if s.st.PublicIngressOperations == nil {
		s.st.PublicIngressOperations = map[string]PublicIngressOperation{}
	}
	s.st.PublicIngressOperations["a"] = mk("a", 2053, 1, true)
	s.st.PublicIngressOperations["b"] = mk("b", 2053, 3, false)
	s.st.PublicIngressOperations["c"] = mk("c", 2054, 7, true)
	if g := s.nextIngressGenerationLocked("ir-1", 2053); g != 2 {
		t.Fatalf("after one retired generation: %d", g)
	}
	s.st.PublicIngressOperations["d"] = mk("d", 2053, 2, true)
	if g := s.nextIngressGenerationLocked("ir-1", 2053); g != 3 {
		t.Fatalf("after two: %d", g)
	}
	if g := s.nextIngressGenerationLocked("ir-2", 2053); g != 1 {
		t.Fatalf("other node: %d", g)
	}
}
