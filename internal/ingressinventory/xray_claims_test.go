package ingressinventory

import (
	"testing"
)

func TestParseXrayInboundClaims(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		covered []uint16
		free    []uint16
		unknown int
	}{
		{name: "number", raw: `{"inbounds":[{"port":443}]}`, covered: []uint16{443}, free: []uint16{444}},
		{name: "string", raw: `{"inbounds":[{"port":"8443"}]}`, covered: []uint16{8443}},
		{name: "range", raw: `{"inbounds":[{"port":"1000-1002"}]}`, covered: []uint16{1000, 1001, 1002}, free: []uint16{999, 1003}},
		{name: "list", raw: `{"inbounds":[{"port":"80, 2053-2054"}]}`, covered: []uint16{80, 2053, 2054}, free: []uint16{2052, 81}},
		{name: "no inbounds", raw: `{"outbounds":[]}`, free: []uint16{443}},
		{name: "unix socket listener", raw: `{"inbounds":[{"listen":"/dev/shm/x.sock","protocol":"vless"}]}`, free: []uint16{443}},
		{name: "abstract socket listener", raw: `{"inbounds":[{"listen":"@xray","protocol":"vless"}]}`, free: []uint16{443}},
		{name: "env port", raw: `{"inbounds":[{"port":"env:PORT"}]}`, unknown: 1},
		{name: "missing port", raw: `{"inbounds":[{"tag":"x"}]}`, unknown: 1},
		{name: "zero", raw: `{"inbounds":[{"port":0}]}`, unknown: 1},
		{name: "out of range", raw: `{"inbounds":[{"port":70000}]}`, unknown: 1},
		{name: "reversed range", raw: `{"inbounds":[{"port":"9-3"}]}`, unknown: 1},
		{name: "not json", raw: `// comment
{"inbounds":[]}`, unknown: 1},
		{name: "one bad one good", raw: `{"inbounds":[{"port":"env:X"},{"port":443}]}`, covered: []uint16{443}, unknown: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims, unknown := parseXrayInboundClaims("cfg", []byte(tc.raw))
			if len(unknown) != tc.unknown {
				t.Fatalf("unknown=%v want %d", unknown, tc.unknown)
			}
			covers := func(p uint16) bool {
				for _, c := range claims {
					if c.Covers(p) {
						return true
					}
				}
				return false
			}
			for _, p := range tc.covered {
				if !covers(p) {
					t.Fatalf("port %d not claimed: %+v", p, claims)
				}
			}
			for _, p := range tc.free {
				if covers(p) {
					t.Fatalf("port %d wrongly claimed: %+v", p, claims)
				}
			}
		})
	}
}
