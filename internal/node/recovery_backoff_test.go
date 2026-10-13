package node

import (
	"testing"
	"time"
)

func TestRecoveryBackoffBounds(t *testing.T) {
	lo := func(int64) int64 { return 0 }
	hi := func(n int64) int64 { return n - 1 }

	if got := recoveryBackoffWith(0, lo); got != 5*time.Millisecond {
		t.Fatalf("attempt 0 min = %v, want 5ms", got)
	}
	if got := recoveryBackoffWith(0, hi); got != 10*time.Millisecond {
		t.Fatalf("attempt 0 max = %v, want 10ms", got)
	}
	prev := time.Duration(0)
	for a := 0; a < 12; a++ {
		got := recoveryBackoffWith(a, hi)
		if got < prev {
			t.Fatalf("attempt %d: %v < previous %v", a, got, prev)
		}
		prev = got
	}
	if got := recoveryBackoffWith(1, hi); got != 20*time.Millisecond {
		t.Fatalf("attempt 1 max = %v, want 20ms", got)
	}
	for _, a := range []int{9, 10, 50, 1000, 1 << 30} {
		if got := recoveryBackoffWith(a, hi); got != 5*time.Second {
			t.Fatalf("attempt %d max = %v, want cap 5s", a, got)
		}
		if got := recoveryBackoffWith(a, lo); got != 2500*time.Millisecond {
			t.Fatalf("attempt %d min = %v, want 2.5s", a, got)
		}
	}
}

func TestRecoveryBackoffRandomNeverZero(t *testing.T) {
	for a := -1; a < 40; a++ {
		for i := 0; i < 200; i++ {
			d := recoveryBackoff(a)
			if d <= 0 || d > recoveryBackoffMax {
				t.Fatalf("attempt %d: delay %v out of range", a, d)
			}
			if a <= 0 && (d < 5*time.Millisecond || d > 10*time.Millisecond) {
				t.Fatalf("attempt %d: first delay %v not in [5ms,10ms]", a, d)
			}
		}
	}
}
