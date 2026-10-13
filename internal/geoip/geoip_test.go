package geoip

import (
	"net/netip"
	"testing"
)

func TestCountry(t *testing.T) {
	for _, c := range []struct{ ip, want string }{
		{"1.1.1.1", ""}, // anycast: whatever the table says, it must not panic
		{"8.8.8.8", "US"},
		{"2001:4860:4860::8888", "US"},
	} {
		got, ok := Country(netip.MustParseAddr(c.ip))
		if c.want != "" && (!ok || got != c.want) {
			t.Errorf("%s: got %q %v, want %s", c.ip, got, ok, c.want)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.1", "::1", "fd00::1", "0.0.0.0"} {
		if got, ok := Country(netip.MustParseAddr(ip)); ok {
			t.Errorf("%s is not public, got %s", ip, got)
		}
	}
	if len(countries) < 200 || len(v4.starts) < 100000 || len(v6.starts) < 100000 {
		t.Fatalf("table too small: %d countries, %d v4, %d v6", len(countries), len(v4.starts), len(v6.starts))
	}
}
