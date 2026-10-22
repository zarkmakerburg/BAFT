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
