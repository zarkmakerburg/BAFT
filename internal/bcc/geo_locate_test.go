package bcc

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// BCC places a node from its IP address, follows a changed address, and
// never overrides a location the operator chose or one set before it.
func TestAutoCountryFromAddress(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	add := func(id, addr, country string) {
		t.Helper()
		if _, err := store.UpsertNode(Node{ID: id, Address: addr, Role: "foreign", Country: country}, ""); err != nil {
			t.Fatal(err)
		}
	}
	add("auto", "8.8.8.8:22", "")
	add("legacy", "8.8.8.8:22", "DE")
	add("manual", "8.8.8.8:22", "")
	add("private", "10.0.0.1:22", "")
	if _, err := store.SetNodeCountry("manual", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if cc, ok := nodeCountry(context.Background(), Node{Address: "8.8.8.8:22"}); !ok || cc != "US" {
		t.Fatalf("nodeCountry=%q %v", cc, ok)
	}
	s := &Server{store: store}
	s.LocateNodes(context.Background())
	want := map[string]string{"auto": "US", "legacy": "DE", "manual": "", "private": ""}
	for id, w := range want {
		n, _ := store.GetNode(id)
		if n.Country != w {
			t.Errorf("%s country=%q, want %q", id, n.Country, w)
		}
	}
	// A new address is located again; the manual node stays as chosen.
	add("auto", "5.9.1.1:22", "")
	s.LocateNodes(context.Background())
	if n, _ := store.GetNode("auto"); n.Country != "DE" || !n.CountryAuto {
		t.Errorf("moved node: %+v", n)
	}
	if _, err := store.SetNodeCountry("auto", "NL", time.Now()); err != nil {
		t.Fatal(err)
	}
	add("auto", "8.8.4.4:22", "")
	s.LocateNodes(context.Background())
	if n, _ := store.GetNode("auto"); n.Country != "NL" || !n.CountryManual {
		t.Errorf("manual choice overridden: %+v", n)
	}
}
