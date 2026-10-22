package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"strings"
	"sync"
	"testing"
)

func proof(tunnel string, generation uint64) ingressmanifest.Proof {
	return ingressmanifest.Proof{TunnelID: tunnel, Generation: generation, ConfigSHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64)}
}
func TestClaimConflictReplayAndRelease(t *testing.T) {
	r := New()
	a := proof("a", 1)
	b := proof("b", 1)
	if err := r.Claim("0.0.0.0:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("[::]:2053", b); !errors.Is(err, ErrConflict) {
		t.Fatalf("dual stack conflict: %v", err)
	}
	if err := r.Release("127.0.0.1:2053", b); !errors.Is(err, ErrUnknown) {
		t.Fatalf("foreign release: %v", err)
	}
	if err := r.Release("0.0.0.0:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("0.0.0.0:2053", a); !errors.Is(err, ErrStale) {
		t.Fatalf("ABA replay: %v", err)
	}
	if err := r.Claim("[::]:2053", b); err != nil {
		t.Fatal(err)
	}
}
func TestStaleGenerationAndConcurrentClaim(t *testing.T) {
	r := New()
	a := proof("a", 2)
	if err := r.Claim("127.0.0.1:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:3000", proof("a", 1)); !errors.Is(err, ErrStale) {
		t.Fatalf("stale: %v", err)
	}
	var wg sync.WaitGroup
	successes := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := r.Claim("127.0.0.1:4000", proof(string(rune('c'+i)), 1))
			successes <- err == nil
		}(i)
	}
	wg.Wait()
	close(successes)
	count := 0
	for ok := range successes {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent winners=%d", count)
	}
}

func TestSameOwnerCannotClaimTwoPortsOrRebindGeneration(t *testing.T) {
	r := New()
	a := proof("a", 1)
	if err := r.Claim("127.0.0.1:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:2054", a); !errors.Is(err, ErrConflict) {
		t.Fatalf("same owner second port: %v", err)
	}
	if err := r.Release("127.0.0.1:2053", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:2054", a); !errors.Is(err, ErrStale) {
		t.Fatalf("released generation rebound: %v", err)
	}
	if err := r.Claim("127.0.0.1:2054", proof("a", 2)); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryPort443OwnershipWithoutSocket(t *testing.T) {
	r := New()
	p := proof("https", 1)
	if err := r.Claim("0.0.0.0:443", p); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("[::]:443", proof("foreign", 1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("dual-stack 443 conflict: %v", err)
	}
	if err := r.Release("127.0.0.1:443", p); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:443", p); !errors.Is(err, ErrStale) {
		t.Fatalf("443 replay: %v", err)
	}
	if err := r.Claim("127.0.0.1:0", p); !errors.Is(err, ErrInvalid) {
		t.Fatalf("port zero: %v", err)
	}
}

func TestRegistryReleaseRejectsHostnameAliasWithoutLosingOwner(t *testing.T) {
	r := New()
	owner := proof("host-fence", 1)
	if err := r.Claim("127.0.0.1:2053", owner); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"localhost:2053", "example.org:2053", "127.0.0.1:0", "not-an-endpoint"} {
		if err := r.Release(bad, owner); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%q release: %v", bad, err)
		}
	}
	if err := r.Claim("[::]:2053", proof("foreign", 1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("owner was lost: %v", err)
	}
	if err := r.Release("127.0.0.1:2053", owner); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryZeroValueClaimReleaseAndStaleFence(t *testing.T) {
	var r Registry
	owner := proof("zero-registry", 1)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("[::]:443", proof("foreign", 1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign claim: %v", err)
	}
	if err := r.Release("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim("127.0.0.1:443", owner); !errors.Is(err, ErrStale) {
		t.Fatalf("ABA replay: %v", err)
	}
}

func TestRegistryRejectsOwnerTupleSeparatorCollision(t *testing.T) {
	r := New()
	for _, bad := range []ingressmanifest.Proof{
		{TunnelID: "alpha\x00beta", Generation: 1, ConfigSHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64)},
		{TunnelID: "alpha", InstanceID: "beta\x00gamma", Generation: 1, ConfigSHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64)},
		{TunnelID: "alpha", InstanceID: "default", Generation: 1, ConfigSHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64)},
	} {
		if err := r.Claim("127.0.0.1:2053", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid owner accepted: %v", err)
		}
		if err := r.Release("127.0.0.1:2053", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid release accepted: %v", err)
		}
	}
	owner := proof("alpha", 1)
	if err := r.Claim("127.0.0.1:2053", owner); err != nil {
		t.Fatal(err)
	}
	if err := r.Release("127.0.0.1:2053", owner); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsManifestInvalidOwnerIDs(t *testing.T) {
	for _, tc := range []struct{ tunnel, instance string }{
		{"invalid/path", ""}, {"ok", "bad space"}, {"ok", "default"}, {strings.Repeat("a", 65), ""}, {"ok", strings.Repeat("b", 41)},
	} {
		p := proof(tc.tunnel, 1)
		p.InstanceID = tc.instance
		r := New()
		if err := r.Claim("127.0.0.1:443", p); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted owner %q/%q: %v", tc.tunnel, tc.instance, err)
		}
		if err := r.Release("127.0.0.1:443", p); !errors.Is(err, ErrInvalid) {
			t.Fatalf("released invalid owner %q/%q: %v", tc.tunnel, tc.instance, err)
		}
	}
}
