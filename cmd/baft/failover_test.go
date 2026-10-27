package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestInstallServiceWritesUnitAndStarts(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	var out, errb bytes.Buffer
	args := []string{"--listen", "127.0.0.1:1080", "--target", "ex1=127.0.0.1:1543,probe=127.0.0.1:1543", "--target", "ex2=127.0.0.1:1443"}
	code := installService("baft-failover", "d", "failover", args, func(a []string) error { return validateFailoverArgs(a, &errb) },
		dir, func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }, &out, &errb)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	unit, _ := os.ReadFile(filepath.Join(dir, "baft-failover.service"))
	if !strings.Contains(string(unit), " failover --listen 127.0.0.1:1080 --target ex1=127.0.0.1:1543,probe=127.0.0.1:1543 --target ex2=127.0.0.1:1443") || !strings.Contains(string(unit), "Restart=always") {
		t.Fatalf("unit:\n%s", unit)
	}
	if len(calls) != 3 {
		t.Fatalf("systemctl calls %v", calls)
	}
}

func TestInstallServiceRefusesBadArgsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	never := func(...string) error { t.Fatal("systemctl called"); return nil }
	if code := installService("baft-failover", "d", "failover", []string{"--listen", "127.0.0.1:1080", "--target", "a=127.0.0.1:1"},
		func(a []string) error { return validateFailoverArgs(a, &errb) }, dir, never, &out, &errb); code != 2 {
		t.Fatalf("one target accepted: %d", code)
	}
	if code := installService("baft-failover", "d", "failover", []string{"--listen", "127.0.0.1:1080", "--target", "a=127.0.0.1:1", "--target", "b=127.0.0.1:2;rm"},
		func([]string) error { return nil }, dir, never, &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
	if ents, _ := os.ReadDir(dir); len(ents) > 1 {
		t.Fatalf("unexpected files %v", ents)
	}
}

func TestEchoInstallRequiresLoopback(t *testing.T) {
	var errb bytes.Buffer
	if code := runEcho(t.Context(), []string{"install", "--listen", "0.0.0.0:2444"}, &errb); code != 2 || !strings.Contains(errb.String(), "loopback") {
		t.Fatalf("code %d: %s", code, errb.String())
	}
}
