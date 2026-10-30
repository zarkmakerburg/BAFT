package integration_test

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/node"
)

func TestListenerShutdownWaitsForSessionTeardown(t *testing.T) {
	p := startRecoveryRuntimePair(t, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() { unblock(); p.closeAllowErrors() }()
	p.exRuntime.SetLogicalSessionLifecycleHookForTest(func(ev node.LogicalSessionLifecycleEvent) {
		if ev.Event == "PEER_RUN_EXIT" {
			enteredOnce.Do(func() { close(entered) })
			<-release // Hold real Peer.Run before it releases flow resources.
		}
	})
	c := openRecoveryFlow(t, p)
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("shutdown")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("shutdown"))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	p.cancel()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("session did not enter teardown")
	}
	select {
	case err := <-p.exDone:
		// Preserve the completion event for the fixture cleanup.
		p.exDone <- err
		t.Fatal("listener Runtime.Run returned while session teardown was held")
	case <-time.After(150 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-p.exDone:
		p.exDone <- err
		if err != nil {
			t.Fatalf("listener shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not finish released teardown")
	}
	if snap := p.exRuntime.Resources.Snapshot(); snap.TotalUsed != 0 || snap.Flows != 0 {
		t.Fatalf("listener returned with resources: %+v", snap)
	}
}
