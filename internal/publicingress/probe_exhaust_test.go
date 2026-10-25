package publicingress

import (
	"net/netip"
	"testing"
	"time"
)

// SECREVIEW: an attacker holding one IPv6 /64 (2^64 addresses) must not be
// able to fill the per-source limiter and thereby deny BCC's readiness probe.
func TestProbeLimiterIPv6PrefixCannotExhaustTable(t *testing.T) {
	var l probeLimiter
	now := time.Now()
	for i := 0; i < probeMaxSources*2; i++ {
		var b [16]byte
		copy(b[:], []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 1, 0, 0, 0, 0, byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		l.allow(netip.AddrFrom16(b), now)
	}
	bcc := netip.MustParseAddr("198.51.100.7")
	if !l.allow(bcc, now) {
		t.Fatal("legitimate prober denied after one /64 flooded the limiter table")
	}
}

// Sources idle long enough to have fully refilled carry no state worth
// keeping; they must be reclaimable so a one-minute flood cannot lock out a
// new source for a full minute.
func TestProbeLimiterReclaimsRefilledSourcesPromptly(t *testing.T) {
	var l probeLimiter
	now := time.Now()
	for i := 0; i < probeMaxSources; i++ {
		l.allow(netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}), now)
	}
	later := now.Add(probeBurst*probeRefill + time.Second)
	if !l.allow(netip.MustParseAddr("198.51.100.7"), later) {
		t.Fatal("new source denied although every stored bucket had already refilled")
	}
}
