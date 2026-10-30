package ingressownership

import (
	"bytes"
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

func TestNFTReaderBoundsOutputBeforeParsing(t *testing.T) {
	var output bytes.Buffer
	writer := &limitedNFTWriter{dst: &output, remaining: 5}
	if n, err := writer.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("first write: %d %v", n, err)
	}
	if n, err := writer.Write([]byte("def")); n != 0 || !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("overflow: %d %v", n, err)
	}
	if output.String() != "abc" {
		t.Fatalf("overflow changed buffer: %q", output.String())
	}
}

func TestNFTReadOnlyInspectionRejectsNilContextAndOversizeSnapshot(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("read-limit", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectNFTIngress(nil, nftFakeReader{}, plan); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("nil context: %v", err)
	}
	if _, err := (NFTCommandReader{Binary: "/usr/sbin/nft"}).ReadRuleset(nil); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("nil command context: %v", err)
	}
	if _, err := InspectNFTIngress(context.Background(), nftFakeReader{raw: make([]byte, (4<<20)+1)}, plan); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("oversized snapshot: %v", err)
	}
}

func TestNFTReaderRejectsCancelledContextWithoutExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (NFTCommandReader{Binary: "/usr/sbin/nft"}).ReadRuleset(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled nft read: %v", err)
	}
}
