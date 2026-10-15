package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type changeDuringConfigValidation struct {
	System
	path    string
	changed bool
}

func (s *changeDuringConfigValidation) Run(ctx context.Context, bin string, args ...string) (string, error) {
	out, err := s.System.Run(ctx, bin, args...)
	if err == nil && len(args) == 4 && args[0] == "config" && args[1] == "validate" && args[3] == s.path {
		b, err := os.ReadFile(s.path)
		if err != nil {
			return out, err
		}
		err = os.WriteFile(s.path, []byte(strings.ReplaceAll(string(b), "service-main", "changed-route")), 0600)
		s.changed = err == nil
		return out, err
	}
	return out, err
}

func TestPreparedValidationDriftRefusedBeforePKIActivation(t *testing.T) {
	p := finalizedPair(t)
	p.exp.PublicAddress = "127.0.0.2"
	code, err := p.ex.PrepareEX(context.Background(), "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(context.Background(), "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	s := &changeDuringConfigValidation{System: p.ex.System, path: filepath.Join(p.ex.stage("candidate"), "baft.new.yaml")}
	p.ex.System = s
	assertPreparedRefusal(t, p.ex, func() error { _, err := p.ex.CommitEX(context.Background(), "candidate", reply); return err })
	if !s.changed {
		t.Fatal("test did not inject the configuration change")
	}
}
