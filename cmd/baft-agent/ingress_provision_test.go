package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

type fakeRunner struct {
	raws [][]byte
	n    int
}

func (f *fakeRunner) ReadRuleset(context.Context) ([]byte, error) {
	r := f.raws[f.n%len(f.raws)]
	f.n++
	return r, nil
}
func (f *fakeRunner) Run(context.Context, ...string) error { return nil }

func chainJSON(handle int, hook string) []byte {
	h := strconv.Itoa(handle)
	return []byte(`{"nftables":[{"chain":{"family":"inet","table":"baft","name":"ingress","handle":` + h +
		`,"type":"filter","hook":"` + hook + `","prio":-100,"policy":"accept"}}]}`)
}

func goodDeps(t *testing.T) ingressDeps {
	t.Helper()
	return ingressDeps{
		LoadPolicy: func(context.Context, string) error { return nil },
		OpenStore: func(string) (*ingressauthority.ReservationStore, error) {
			dir := filepath.Join(t.TempDir(), "authority")
			if err := os.Mkdir(dir, 0o700); err != nil {
				return nil, err
			}
			return ingressauthority.OpenReservationStore(dir)
		},
		OpenJournal:    func(string) (ingressownership.MutationJournal, error) { return &ingressownership.LabJournal{}, nil },
		RecoverJournal: func(context.Context, string) (int, error) { return 0, nil },
		ChainHandle: func(context.Context) (uint64, ingressownership.NFTMutationRunner, error) {
			return 7, &fakeRunner{raws: [][]byte{chainJSON(7, "input")}}, nil
		},
	}
}

func TestBuildIngressOperationalSuccess(t *testing.T) {
	op, err := buildIngressOperational(context.Background(), "n1", "/x", goodDeps(t))
	if err != nil || op == nil || op.Store == nil || op.Executor == nil {
		t.Fatalf("all prerequisites hold but got op=%v err=%v", op, err)
	}
}

func TestBuildIngressOperationalFailClosed(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(*ingressDeps){
		"policy": func(d *ingressDeps) { d.LoadPolicy = func(context.Context, string) error { return boom } },
		"store err": func(d *ingressDeps) {
			d.OpenStore = func(string) (*ingressauthority.ReservationStore, error) { return nil, boom }
		},
		"store nil": func(d *ingressDeps) {
			d.OpenStore = func(string) (*ingressauthority.ReservationStore, error) { return nil, nil }
		},
		"journal err": func(d *ingressDeps) {
			d.OpenJournal = func(string) (ingressownership.MutationJournal, error) { return nil, boom }
		},
		"journal nil": func(d *ingressDeps) {
			d.OpenJournal = func(string) (ingressownership.MutationJournal, error) { return nil, nil }
		},
		"journal recover": func(d *ingressDeps) { d.RecoverJournal = func(context.Context, string) (int, error) { return 0, boom } },
		"chain err": func(d *ingressDeps) {
			d.ChainHandle = func(context.Context) (uint64, ingressownership.NFTMutationRunner, error) { return 0, nil, boom }
		},
		"chain zero handle": func(d *ingressDeps) {
			d.ChainHandle = func(context.Context) (uint64, ingressownership.NFTMutationRunner, error) {
				return 0, &fakeRunner{raws: [][]byte{chainJSON(7, "input")}}, nil
			}
		},
		"chain nil runner": func(d *ingressDeps) {
			d.ChainHandle = func(context.Context) (uint64, ingressownership.NFTMutationRunner, error) { return 7, nil, nil }
		},
		"unwired": func(d *ingressDeps) { d.ChainHandle = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDeps(t)
			mutate(&d)
			op, err := buildIngressOperational(context.Background(), "n1", "/x", d)
			if err == nil || op != nil {
				t.Fatalf("expected nil executor, got op=%v err=%v", op, err)
			}
		})
	}
	if op, err := buildIngressOperational(context.Background(), "", "/x", goodDeps(t)); err == nil || op != nil {
		t.Fatal("empty node id accepted")
	}
}

func TestBuildIngressOperationalLeftoversStayFenced(t *testing.T) {
	d := goodDeps(t)
	var logs []string
	d.Logf = func(f string, a ...any) { logs = append(logs, f) }
	d.RecoverJournal = func(context.Context, string) (int, error) { return 2, nil }
	op, err := buildIngressOperational(context.Background(), "n1", "/x", d)
	if err != nil || op == nil {
		t.Fatalf("leftovers must not be cleaned or fatal: %v", err)
	}
	if len(logs) == 0 || !strings.Contains(strings.Join(logs, "|"), "remain fenced") {
		t.Fatalf("leftovers not reported: %v", logs)
	}
}

func TestStableChainHandle(t *testing.T) {
	ctx := context.Background()
	if h, err := stableChainHandle(ctx, &fakeRunner{raws: [][]byte{chainJSON(9, "input")}}); err != nil || h != 9 {
		t.Fatalf("stable chain: %d %v", h, err)
	}
	if _, err := stableChainHandle(ctx, &fakeRunner{raws: [][]byte{chainJSON(9, "input"), chainJSON(10, "input")}}); err == nil {
		t.Fatal("unstable handle accepted")
	}
	if _, err := stableChainHandle(ctx, &fakeRunner{raws: [][]byte{chainJSON(9, "forward")}}); err == nil {
		t.Fatal("wrong hook accepted")
	}
	if _, err := stableChainHandle(ctx, &fakeRunner{raws: [][]byte{[]byte(`{"nftables":[]}`)}}); err == nil {
		t.Fatal("absent chain accepted")
	}
}

func TestReadNFTLabChainHandleDuplicateRefused(t *testing.T) {
	c := `{"chain":{"family":"inet","table":"baft","name":"ingress","handle":3,"type":"filter","hook":"input","prio":-100,"policy":"accept"}}`
	if _, err := ingressownership.ReadNFTLabChainHandle([]byte(`{"nftables":[` + c + `,` + c + `]}`)); err == nil {
		t.Fatal("duplicate chains accepted")
	}
}

func TestPrivateSubdirRefusesLoosePerms(t *testing.T) {
	root := t.TempDir()
	if _, err := privateSubdir(root, "ok"); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(root, "loose")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := privateSubdir(root, "loose"); err == nil {
		t.Fatal("0755 dir accepted")
	}
	if _, err := privateSubdir("relative", "x"); err == nil {
		t.Fatal("relative state dir accepted")
	}
}

// Without an installed policy (always true in the sandbox) the host wiring must
// yield no executor.
func TestHostWiringFailsClosedWithoutPolicy(t *testing.T) {
	op, err := buildIngressOperational(context.Background(), "ir-none", t.TempDir(), hostIngressDeps(nil))
	if err == nil || op != nil {
		t.Fatalf("host wiring produced executor without policy: %v %v", op, err)
	}
}
