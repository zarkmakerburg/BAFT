package routes

import "testing"

func peerSet(ids ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func TestSameRouteIDScopedPerTenant(t *testing.T) {
	tbl, err := New([]Route{
		{ID: "port-443", Target: "127.0.0.1:1001", AllowedPeers: peerSet("a")},
		{ID: "port-443", Target: "127.0.0.1:1002", AllowedPeers: peerSet("b")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := tbl.Resolve("a", "port-443"); err != nil || got != "127.0.0.1:1001" {
		t.Fatalf("a: %q %v", got, err)
	}
	if got, err := tbl.Resolve("b", "port-443"); err != nil || got != "127.0.0.1:1002" {
		t.Fatalf("b: %q %v", got, err)
	}
	if _, err := tbl.Resolve("c", "port-443"); err == nil || err.Error() != "ROUTE_DENIED" {
		t.Fatalf("c must be denied, got %v", err)
	}
	if _, err := tbl.Resolve("a", "nope"); err == nil || err.Error() != "ROUTE_NOT_FOUND" {
		t.Fatalf("unknown route must be not found, got %v", err)
	}
}

func TestOverlappingDuplicateRouteStillRejected(t *testing.T) {
	_, err := New([]Route{
		{ID: "x", Target: "127.0.0.1:1001", AllowedPeers: peerSet("a", "b")},
		{ID: "x", Target: "127.0.0.1:1002", AllowedPeers: peerSet("b")},
	})
	if err == nil {
		t.Fatal("overlapping duplicate route must be rejected")
	}
}

func TestReplaceIsAtomicAndKeepsOldOnError(t *testing.T) {
	tbl, _ := New([]Route{{ID: "x", Target: "127.0.0.1:1001", AllowedPeers: peerSet("a")}})
	if err := tbl.Replace([]Route{{ID: "x", Target: "bad", AllowedPeers: peerSet("a")}}); err == nil {
		t.Fatal("invalid replacement must fail")
	}
	if got, err := tbl.Resolve("a", "x"); err != nil || got != "127.0.0.1:1001" {
		t.Fatalf("old table must survive failed replace: %q %v", got, err)
	}
	if err := tbl.Replace([]Route{{ID: "y", Target: "127.0.0.1:1003", AllowedPeers: peerSet("b")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tbl.Resolve("a", "x"); err == nil {
		t.Fatal("removed route must be gone")
	}
	if got, _ := tbl.Resolve("b", "y"); got != "127.0.0.1:1003" {
		t.Fatalf("got %q", got)
	}
}
