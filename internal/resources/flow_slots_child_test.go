package resources

import "testing"

func TestChildFlowSlotsCapAndParentAccounting(t *testing.T) {
	parent, _ := NewFlowSlots(4)
	a, err := parent.NewChild(2)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := parent.NewChild(3)
	if !a.TryAcquire() || !a.TryAcquire() {
		t.Fatal("child should admit up to its cap")
	}
	if a.TryAcquire() {
		t.Fatal("child admitted past its cap")
	}
	if parent.Used() != 2 {
		t.Fatalf("parent used=%d want 2", parent.Used())
	}
	if !b.TryAcquire() || !b.TryAcquire() {
		t.Fatal("b should get the remaining parent slots")
	}
	if b.TryAcquire() {
		t.Fatal("parent exhausted: b must be refused")
	}
	if b.Used() != 2 {
		t.Fatalf("b used=%d want 2 (rollback on parent refusal)", b.Used())
	}
	a.Release()
	if parent.Used() != 3 || a.Used() != 1 {
		t.Fatalf("release must release both levels: parent=%d a=%d", parent.Used(), a.Used())
	}
	if !b.TryAcquire() {
		t.Fatal("freed parent slot should be usable")
	}
	if _, err := parent.NewChild(0); err == nil {
		t.Fatal("non-positive child cap must be rejected")
	}
}
