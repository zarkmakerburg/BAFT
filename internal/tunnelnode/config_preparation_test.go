package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreparedIRConfigDriftRefusedBeforeLiveMutation(t *testing.T) {
	for _, mode := range []string{"valid-config-edit", "invalid-config", "noise-key-edit", "peer-ca-edit", "config-symlink", "config-mode", "key-mode"} {
		t.Run(mode, func(t *testing.T) {
			p := finalizedPair(t)
			code, err := p.ex.PrepareEX(context.Background(), "candidate", p.exp)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.ir.PrepareIR(context.Background(), "candidate", code, p.irp); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(p.ir.stage("candidate"), "baft.new.yaml")
			switch mode {
			case "valid-config-edit":
				b, _ := os.ReadFile(path)
				err = os.WriteFile(path, []byte(strings.ReplaceAll(string(b), "service-main", "changed-route")), 0600)
			case "invalid-config":
				err = os.WriteFile(path, []byte("not: a valid BAFT configuration\n"), 0600)
			case "noise-key-edit":
				path = p.ir.noiseKey()
				b, _ := os.ReadFile(path)
				err = os.WriteFile(path, append(b, '\n'), 0600)
			case "peer-ca-edit":
				path = filepath.Join(p.ir.stage("candidate"), "peer-ca.pem")
				b, _ := os.ReadFile(path)
				err = os.WriteFile(path, append(b, '\n'), 0600)
			case "config-symlink":
				b, _ := os.ReadFile(path)
				target := filepath.Join(t.TempDir(), "operator.yaml")
				if err := os.WriteFile(target, b, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(target, path)
			case "config-mode":
				err = os.Chmod(path, 0644)
			case "key-mode":
				err = os.Chmod(p.ir.noiseKey(), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			assertPreparedRefusal(t, p.ir, func() error { _, err := p.ir.CommitIR(context.Background(), "candidate"); return err })
		})
	}
}

func assertPreparedRefusal(t *testing.T, n *node, action func() error) {
	t.Helper()
	artifacts := map[string]string{}
	pki, err := snapshotPKI(n.pkiDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{n.liveConfig(), n.unitPath(), n.markerPath(), n.generationPath()} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		artifacts[path] = string(b)
	}
	calls := len(n.host.calls)
	if err := action(); err == nil {
		t.Error("changed candidate was accepted")
	}
	for path, want := range artifacts {
		if b, err := os.ReadFile(path); err != nil || string(b) != want {
			t.Errorf("refusal changed live artifact: %s", filepath.Base(path))
		}
	}
	for _, call := range n.host.calls[calls:] {
		if !strings.HasPrefix(call, "is-active ") && !strings.HasPrefix(call, "is-enabled ") && !strings.HasPrefix(call, "show ") {
			t.Errorf("refusal touched service: %s", call)
		}
	}
	if n.activeID() != "candidate" {
		t.Error("refusal lost prepared ownership")
	}
	if after, err := snapshotPKI(n.pkiDir()); err != nil || !reflect.DeepEqual(pki, after) {
		t.Error("refusal changed live certificates")
	}
}

func TestPreparedEXParamsDriftRefusedBeforeReplyConsumption(t *testing.T) {
	p := finalizedPair(t)
	code, err := p.ex.PrepareEX(context.Background(), "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(context.Background(), "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	var changed ExParams
	if err := readJSON(filepath.Join(p.ex.stage("candidate"), "ex-params.json"), &changed); err != nil {
		t.Fatal(err)
	}
	changed.Target = "127.0.0.1:2444"
	if err := writeJSON(filepath.Join(p.ex.stage("candidate"), "ex-params.json"), changed, 0600); err != nil {
		t.Fatal(err)
	}
	assertPreparedRefusal(t, p.ex, func() error { _, err := p.ex.CommitEX(context.Background(), "candidate", reply); return err })
	for _, name := range []string{"pending.json", "psk"} {
		if _, err := os.Stat(filepath.Join(p.ex.stage("candidate"), name)); err != nil {
			t.Errorf("refusal consumed pairing material: %s", name)
		}
	}
}
