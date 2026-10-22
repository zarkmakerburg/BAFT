package ingressownership

import (
	"context"
	"errors"
	"testing"
)

type nftFakeReader struct {
	raw []byte
	err error
}

func (f nftFakeReader) ReadRuleset(context.Context) ([]byte, error) { return f.raw, f.err }
func TestNFTReadOnlyInspection(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("reader", 6))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"nftables":[{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":88,"comment":"` + plan.Tag + `","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}},{"accept":null}]}}]}`)
	observation, err := InspectNFTIngress(context.Background(), nftFakeReader{raw: raw}, plan)
	if err != nil || observation.Handle != 88 {
		t.Fatalf("inspection failed: %+v %v", observation, err)
	}
	for _, runner := range []NFTRunner{nil, nftFakeReader{err: errors.New("nft unavailable")}, nftFakeReader{raw: []byte(`{"nftables":[{"metainfo":{}}]}`)}} {
		if _, err := InspectNFTIngress(context.Background(), runner, plan); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("unverified inspection accepted: %v", err)
		}
	}
	if _, err := (NFTCommandReader{Binary: "/bin/sh"}).ReadRuleset(context.Background()); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("arbitrary command accepted: %v", err)
	}
}
