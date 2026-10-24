//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestKernelProcessStartStrict(t *testing.T) {
	raw, err := readBoundedKernelFile(context.Background(), "/proc/self/stat")
	if err != nil { t.Fatal(err) }
	if ticks, err := parseKernelProcessStart(raw, os.Getpid()); err != nil || ticks == 0 {
		t.Fatalf("actual generation unavailable: %v", err)
	}
	tests := []struct{ raw []byte; pid int }{
		{raw, os.Getpid()+1},
		{nil, os.Getpid()},
		{[]byte("33 S 0"), 33},
		{[]byte("33 (comm) S 0 0"), 33},
	}
	for _, tc := range tests {
		if _, err := parseKernelProcessStart(tc.raw, tc.pid); !errors.Is(err, ErrCoverageIncomplete) {
			t.Fatalf("malformed kernel generation accepted: %v", err)
		}
	}
	ctx,cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readBoundedKernelFile(ctx,"/proc/self/cgroup"); !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("cancelled metadata read accepted: %v",err)
	}
	if _,_,_,err := digestRunningExecutable(ctx); !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("cancelled executable read accepted: %v",err)
	}
}
