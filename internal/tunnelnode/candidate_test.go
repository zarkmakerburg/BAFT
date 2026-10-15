package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func stageCandidatePair(t *testing.T) (*pair, string, CandidateReport, CandidateReport) {
	t.Helper()
	p := finalizedPair(t)
	p.exp.StageConfig, p.irp.StageConfig = true, true
	p.exp.PublicAddress = "localhost"
	ctx := context.Background()
	code, err := p.ex.PrepareEX(ctx, "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(ctx, "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := p.ex.StageEX(ctx, "candidate", reply)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := p.ir.ValidateCandidate(ctx, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	return p, reply, ex, ir
}

func liveArtifacts(t *testing.T, n *node) map[string]string {
	t.Helper()
	out, err := snapshotPKI(n.pkiDir())
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		out = map[string]string{}
	}
	for _, path := range []string{n.liveConfig(), n.unitPath(), n.markerPath(), n.generationPath()} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out[path] = string(b)
	}
	return out
}

func TestCandidateStagingPreservesActiveStateAndCommitsSealedBytes(t *testing.T) {
	p := finalizedPair(t)
	p.exp.StageConfig, p.irp.StageConfig = true, true
	p.exp.PublicAddress = "localhost"
	exBefore, irBefore := liveArtifacts(t, p.ex), liveArtifacts(t, p.ir)
	exCalls, irCalls := len(p.ex.host.calls), len(p.ir.host.calls)
	ctx := context.Background()
	code, err := p.ex.PrepareEX(ctx, "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(ctx, "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := p.ex.StageEX(ctx, "candidate", reply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.StageEX(ctx, "candidate", reply); err != nil {
		t.Fatal("completed staging retry:", err)
	}
	ir, err := p.ir.ValidateCandidate(ctx, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []struct {
		n      *node
		before map[string]string
		calls  int
	}{{p.ex, exBefore, exCalls}, {p.ir, irBefore, irCalls}} {
		if !reflect.DeepEqual(side.before, liveArtifacts(t, side.n)) {
			t.Fatal("preparation changed active artifacts")
		}
		for _, call := range side.n.host.calls[side.calls:] {
			if !strings.HasPrefix(call, "is-active ") && !strings.HasPrefix(call, "is-enabled ") && !strings.HasPrefix(call, "show ") {
				t.Fatalf("preparation operated service: %s", call)
			}
		}
	}
	for _, name := range []string{"pending.json", "psk"} {
		if _, err := os.Stat(filepath.Join(p.ex.stage("candidate"), name)); !os.IsNotExist(err) {
			t.Fatal("completed staging did not consume one-time pairing material")
		}
	}
	assertPreparedRefusal(t, p.ex, func() error { _, err := p.ex.CommitEX(ctx, "candidate", reply); return err })
	assertPreparedRefusal(t, p.ir, func() error { _, err := p.ir.CommitIR(ctx, "candidate"); return err })
	assertPreparedRefusal(t, p.ex, func() error {
		_, err := p.ex.CommitCandidate(ctx, "candidate", strings.Repeat("0", 64), ex.Generation)
		return err
	})
	assertPreparedRefusal(t, p.ir, func() error {
		_, err := p.ir.CommitCandidate(ctx, "candidate", ir.ConfigSHA256, ir.Generation+1)
		return err
	})
	if _, err := p.ex.CommitCandidate(ctx, "candidate", ex.ConfigSHA256, ex.Generation); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.CommitCandidate(ctx, "candidate", ir.ConfigSHA256, ir.Generation); err != nil {
		t.Fatal(err)
	}
	for _, side := range []struct {
		n *node
		r CandidateReport
	}{{p.ex, ex}, {p.ir, ir}} {
		b, err := os.ReadFile(side.n.liveConfig())
		if err != nil || shaHex(b) != side.r.ConfigSHA256 || side.n.readGeneration() != side.r.Generation+1 {
			t.Fatal("installed config differs from sealed candidate")
		}
	}
}

func TestCandidateCommitRefusesPostReportDrift(t *testing.T) {
	for _, mode := range []string{"ex-config", "ex-params", "ex-key", "ex-pki", "ir-ca", "wrong-reply"} {
		t.Run(mode, func(t *testing.T) {
			p, reply, ex, ir := stageCandidatePair(t)
			n, report := p.ex, ex
			path := filepath.Join(n.stage("candidate"), "baft.new.yaml")
			switch mode {
			case "ex-params":
				path = filepath.Join(n.stage("candidate"), "ex-params.json")
			case "ex-key":
				path = n.noiseKey()
			case "ex-pki":
				tx, _ := n.readTxn("candidate")
				path = filepath.Join(n.preparedPKIDir(tx), "server.pem")
			case "ir-ca":
				n, report = p.ir, ir
				path = filepath.Join(n.stage("candidate"), "peer-ca.pem")
			}
			if mode == "wrong-reply" {
				assertPreparedRefusal(t, n, func() error { _, err := n.StageEX(context.Background(), "candidate", reply+"A"); return err })
				return
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(b, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			assertPreparedRefusal(t, n, func() error {
				_, err := n.CommitCandidate(context.Background(), "candidate", report.ConfigSHA256, report.Generation)
				return err
			})
		})
	}
}
