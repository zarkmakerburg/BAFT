package publicingress

import (
	"sync/atomic"
	"testing"
	"time"
)

type blockingCloser struct {
	release chan struct{}
	calls   atomic.Int32
	done    atomic.Bool
}

func (b *blockingCloser) Close() error {
	b.calls.Add(1)
	<-b.release
	b.done.Store(true)
	return nil
}

// A second closer must not return while the first Close is still in
// progress: Serve's return (and Runtime.Retire) promises a released socket.
func TestOnceCloserWaitsForInFlightClose(t *testing.T) {
	b := &blockingCloser{release: make(chan struct{})}
	closeOnce := onceCloser(b)
	go closeOnce()
	for b.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	second := make(chan struct{})
	go func() { closeOnce(); close(second) }()
	select {
	case <-second:
		t.Fatal("second close returned before the in-flight close finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(b.release)
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("second close never returned")
	}
	if !b.done.Load() || b.calls.Load() != 1 {
		t.Fatalf("close ran %d times, done=%v", b.calls.Load(), b.done.Load())
	}
}
