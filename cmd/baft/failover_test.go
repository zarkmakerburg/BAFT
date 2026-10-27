package main

import (
	"bytes"
	"testing"
)

func TestFailoverTargetParsing(t *testing.T) {
	var ts failoverTargets
	if err := ts.Set("ex1=127.0.0.1:1443,probe=127.0.0.1:1444"); err != nil || ts[0].Probe != "127.0.0.1:1444" {
		t.Fatalf("%v %+v", err, ts)
	}
	if err := ts.Set("ex2=127.0.0.1:1543"); err != nil || ts[1].Probe != "" {
		t.Fatalf("%v %+v", err, ts)
	}
	for _, bad := range []string{"ex", "=1.2.3.4:1", "ex=host", "ex=1.2.3.4:1,prob=1.2.3.4:2", "ex=1.2.3.4:1,probe=nope"} {
		if err := ts.Set(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestFailoverNeedsTwoTargets(t *testing.T) {
	var errb bytes.Buffer
	if code := runFailover(t.Context(), []string{"--listen", "127.0.0.1:0", "--target", "a=127.0.0.1:1"}, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
}
