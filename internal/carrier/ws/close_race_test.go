package ws

import (
	"net"
	"sync"
	"testing"
	"time"
)

type closeWriteSignal struct {
	net.Conn
	entered chan struct{}
	once    sync.Once
}

func (c *closeWriteSignal) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.entered) })
	return c.Conn.Write(p)
}

func TestForcedCloseInterruptsBlockedGracefulClose(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	signaled := &closeWriteSignal{Conn: local, entered: make(chan struct{})}
	conn := newConn(signaled, nil, false)
	graceful := make(chan struct{})
	go func() { _ = conn.Close(); close(graceful) }()
	// The real pipe's peer does not read. Entry to Write proves graceful
	// Close owns closeOnce, without scheduler sleeps or timing assumptions.
	select {
	case <-signaled.entered:
	case <-time.After(time.Second):
		t.Fatal("graceful close did not start its frame")
	}
	forced := make(chan struct{})
	go func() { conn.closeUnderlying(); close(forced) }()
	select {
	case <-forced:
	case <-time.After(time.Second):
		t.Fatal("forced close waited behind a blocked graceful close")
	}
	select {
	case <-graceful:
	case <-time.After(time.Second):
		t.Fatal("raw socket closure did not unblock graceful close")
	}
}
