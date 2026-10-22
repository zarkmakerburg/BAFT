package ingressownership

import (
	"errors"
	"testing"
)

func TestNFTSnapshotFailsClosedOnUnverifiedIngress(t *testing.T) {
	cases := []string{"", `{}`, `{"nftables":[]}`, `{"nftables":[{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":12,"comment":"baft-ingress-test","expr":[]}}]}`}
	for i, raw := range cases {
		if _, err := ParseNFTIngressRules([]byte(raw)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	raw := `{"nftables":[{"metainfo":{"version":"1.0"}},{"rule":{"family":"inet","table":"unrelated","chain":"input","handle":10}}]}`
	observations, err := ParseNFTIngressRules([]byte(raw))
	if err != nil || len(observations) != 0 {
		t.Fatalf("unrelated rule: %+v %v", observations, err)
	}
}

func TestNFTSnapshotRejectsTrailingDocuments(t *testing.T) {
	valid := `{"nftables":[{"metainfo":{"version":"1.0"}}]}`
	for _, suffix := range []string{` {"nftables":[]}`, ` true`, ` []`, ` garbage`} {
		if _, err := ParseNFTIngressRules([]byte(valid + suffix)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("trailing document accepted: %q %v", suffix, err)
		}
	}
}
