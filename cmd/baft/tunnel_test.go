package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestTerminalTunnelRequiresScopedInstance(t *testing.T) {
	for _, args := range [][]string{
		{"ex-code"},
		{"ex-code", "--instance", "default", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
		{"ex-code", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new", "--address", "127.0.0.1"},
		{"ir-apply", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
		{"ex-accept", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
	} {
		var out, stderr bytes.Buffer
		if got := runTunnelCLI(context.Background(), args, &out, &stderr); got != 2 {
			t.Errorf("args %v returned %d, want usage failure: %s", args, got, stderr.String())
		}
		if strings.Contains(out.String(), "BAFTPAIR1:") {
			t.Fatal("pairing code returned on rejected call")
		}
	}
}
