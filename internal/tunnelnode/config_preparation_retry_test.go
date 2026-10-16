package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedIRRestoredCandidateCanBeExplicitlyRetried(t *testing.T) {
	p := finalizedPair(t)
	code, err := p.ex.PrepareEX(context.Background(), "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.PrepareIR(context.Background(), "candidate", code, p.irp); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.ir.stage("candidate"), "baft.new.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	assertPreparedRefusal(t, p.ir, func() error { _, err := p.ir.CommitIR(context.Background(), "candidate"); return err })
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.CommitIR(context.Background(), "candidate"); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedIRRetainsOlderReceiptContract(t *testing.T) {
	p := finalizedPair(t)
	code, err := p.ex.PrepareEX(context.Background(), "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.PrepareIR(context.Background(), "candidate", code, p.irp); err != nil {
		t.Fatal(err)
	}
	tx, err := p.ir.readTxn("candidate")
	if err != nil {
		t.Fatal(err)
	}
	tx.PreparedConfigSHA, tx.PreparedConfigIdentity = "", ""
	tx.PreparedConfigInputs = nil
	if err := p.ir.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.CommitIR(context.Background(), "candidate"); err != nil {
		t.Fatal(err)
	}
}
