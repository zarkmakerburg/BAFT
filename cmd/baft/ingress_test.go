package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
)

func TestIngressCLIArgumentAndHelpExitCodes(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"ingress"}, 2},
		{[]string{"ingress", "apply"}, 2},
		{[]string{"ingress", "run"}, 2},
		{[]string{"ingress", "run", "--listen", "127.0.0.1:1023", "--target", "127.0.0.1:1443"}, 2},
		{[]string{"ingress", "run", "--listen", "127.0.0.1:22", "--target", "127.0.0.1:1443"}, 2},
		{[]string{"ingress", "run", "--listen", "127.0.0.1:14443", "--target", "localhost:1443"}, 2},
		{[]string{"ingress", "run", "--listen", "127.0.0.1:14443", "--target", "127.0.0.1:1443", "extra"}, 2},
		{[]string{"ingress", "run", "--help"}, 0},
	} {
		var stdout, stderr bytes.Buffer
		if got := runContext(context.Background(), tc.args, &stdout, &stderr); got != tc.want {
			t.Fatalf("%v: exit=%d want=%d stderr=%s", tc.args, got, tc.want, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("runtime emitted readiness output: %s", stdout.String())
		}
		if tc.want == 0 && !strings.Contains(stderr.String(), "custom SSH/management ports") {
			t.Fatal("help omitted runtime boundary")
		}
	}
}

func TestIngressCLICanceledRunAndBindFailure(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	args := []string{"ingress", "run", "--listen", ln.Addr().String(), "--target", "127.0.0.1:1443"}
	var stdout, stderr bytes.Buffer
	if got := runContext(context.Background(), args, &stdout, &stderr); got != 1 {
		t.Fatalf("bind failure exit=%d stderr=%s", got, stderr.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stderr.Reset()
	if got := runContext(ctx, args, &stdout, &stderr); got != 0 {
		t.Fatalf("cancellation exit=%d stderr=%s", got, stderr.String())
	}
	// Both calls must leave the unrelated occupied listener open.
	if tcp, ok := ln.(*net.TCPListener); !ok {
		t.Fatal("unexpected listener type")
	} else if file, err := tcp.File(); err != nil {
		t.Fatalf("existing listener closed: %v", err)
	} else {
		_ = file.Close()
	}
}

func TestIngressCLIRequiresExplicitPublicListenerOptIn(t *testing.T) {
 for _, listen := range []string{"0.0.0.0:14443", "[::]:14443", "192.0.2.10:14443"} {
  t.Run(listen, func(t *testing.T) {
   var stderr bytes.Buffer
   args := []string{"run", "--listen", listen, "--target", "127.0.0.1:1443"}
   if got := runIngress(context.Background(), args, &stderr); got != 2 {
    t.Fatalf("public listener without consent exit=%d, stderr=%s", got, stderr.String())
   }
   if !strings.Contains(stderr.String(), "--allow-public") {
    t.Fatalf("missing explicit opt-in guidance: %s", stderr.String())
   }
   // With opt-in the request proceeds past the policy check, even if the
   // local host cannot bind the selected address or port.
   ctx, cancel := context.WithCancel(context.Background())
   cancel()
   stderr.Reset()
   args = append(args, "--allow-public")
   if got := runIngress(ctx, args, &stderr); got != 0 {
    t.Fatalf("explicit public opt-in rejected: exit=%d stderr=%s", got, stderr.String())
   }
  })
 }
}
