package bcc

import (
	"testing"
	"time"
)

// BCC reviews a plan twice (preview and deploy) and binds approval to its
// hash. Multiple active sibling records must never make the hash depend on
// Go's randomized map iteration order.
func TestScopedEditPlanStableAndDoesNotReplaceLegacy(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now().UTC()
	legacy := activeLegacyForCoexistence(t, s, now)
	scopedReq := testCoexistenceRequest()
	scoped, err := s.CreateTunnel(scopedReq, now)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	scoped.Phase = TunnelActive
	s.st.Tunnels[scoped.ID] = scoped
	for _, id := range []string{"ir-1", "ex-1", "ex-2"} {
		n := s.st.Nodes[id]
		n.AgentSeen = now
		s.st.Nodes[id] = n
	}
	s.mu.Unlock()

	edit := scopedReq
	edit.Port = 2053
	edit.Target = "127.0.0.1:3443"

	first, err := s.BuildPlan(edit, now)
	if err != nil {
		t.Fatal(err)
	}
	if !first.OK || first.Replaces != scoped.ID || first.Request.InstanceID != scoped.InstanceID {
		t.Fatalf("wrong edited scope: %+v", first)
	}
	for i := 0; i < 150; i++ {
		p, err := s.BuildPlan(edit, now)
		if err != nil {
			t.Fatal(err)
		}
		if p.Hash != first.Hash || p.Replaces != scoped.ID {
			t.Fatalf("preview hash is non-deterministic at iteration %d: first=%s next=%s", i, first.Hash, p.Hash)
		}
	}
	updated, _, err := s.CreateTunnelFromPlan(edit, first.Hash, now)
	if err != nil {
		t.Fatalf("reviewed edit must be deployable: %v", err)
	}
	if updated.InstanceID != scoped.InstanceID || updated.Port != 2053 ||
		updated.Target != "127.0.0.1:3443" {
		t.Fatalf("edited values/scope lost: %+v", updated)
	}
	if old, _ := s.GetTunnel(scoped.ID); old.Phase != TunnelActive {
		t.Fatalf("old scoped instance was replaced before activation: %s", old.Phase)
	}
	if old, _ := s.GetTunnel(legacy.ID); old.Phase != TunnelActive {
		t.Fatalf("legacy was interrupted during scoped edit: %s", old.Phase)
	}

	s.mu.Lock()
	updated.Phase = TunnelFinalizingEX
	s.st.Tunnels[updated.ID] = updated
	s.nextAfterFinalizeLocked(&updated, now)
	s.st.Tunnels[updated.ID] = updated
	oldScoped := s.st.Tunnels[scoped.ID]
	oldLegacy := s.st.Tunnels[legacy.ID]
	edited := s.st.Tunnels[updated.ID]
	s.mu.Unlock()
	if edited.Phase != TunnelActive || oldScoped.Phase != TunnelSuperseded ||
		oldLegacy.Phase != TunnelActive {
		t.Fatalf("finalized scope wrong: edited=%s prior-scoped=%s legacy=%s", edited.Phase, oldScoped.Phase, oldLegacy.Phase)
	}
}
