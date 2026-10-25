package session

import (
	"net"
	"sync"
	"time"
)

// idleConn closes a Flow's application connection after a period with no
// successful read or write in EITHER direction, so a silent or half-dead peer
// cannot hold a Flow slot, buffers and a socket indefinitely. Closing makes
// the existing pumps observe an ordinary connection error and tear the Flow
// down through the normal reset path; nothing here touches Session, recovery
// or ledger state. Activity is real bytes only, never mere liveness.
type idleConn struct {
	net.Conn
	idle  time.Duration
	mu    sync.Mutex
	timer *time.Timer
}

// idleTCPConn additionally forwards the two TCP-only operations the Flow
// teardown paths feature-test for (half-close and abortive close), so wrapping
// never changes how a TCP connection is closed.
type idleTCPConn struct {
	*idleConn
	tcp interface {
		CloseWrite() error
		SetLinger(int) error
	}
}

func (c idleTCPConn) CloseWrite() error     { return c.tcp.CloseWrite() }
func (c idleTCPConn) SetLinger(s int) error { return c.tcp.SetLinger(s) }

// newIdleConn returns c unchanged when idle <= 0 (feature disabled).
func newIdleConn(c net.Conn, idle time.Duration) net.Conn {
	if c == nil || idle <= 0 {
		return c
	}
	ic := &idleConn{Conn: c, idle: idle}
	ic.timer = time.AfterFunc(idle, func() { _ = c.Close() })
	if t, ok := c.(interface {
		CloseWrite() error
		SetLinger(int) error
	}); ok {
		return idleTCPConn{idleConn: ic, tcp: t}
	}
	return ic
}

func (c *idleConn) touch() {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Reset(c.idle)
	}
	c.mu.Unlock()
}

func (c *idleConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *idleConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *idleConn) Close() error {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
	return c.Conn.Close()
}
