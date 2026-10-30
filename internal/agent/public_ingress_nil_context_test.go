package agent

import "testing"

func TestIngressSignedHandlersRejectNilContext(t *testing.T) {
	j, _ := prepareJob(t)
	a := &Agent{}
	if _, err := a.publicIngressPrepare(nil, j); err == nil {
		t.Fatal("prepare accepted nil context")
	}
	if _, err := a.publicIngressCancel(nil, j); err == nil {
		t.Fatal("cancel accepted nil context")
	}
}
