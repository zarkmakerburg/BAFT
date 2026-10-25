package benchpath

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/session"
)

// v0.4.0 only: FlowIdleTimeout wraps each app conn in idleConn (mutex + timer
// reset per Read/Write). Zero (the default used by the shared file) leaves the
// conn unwrapped; these measure the enabled cost.
func withIdle(o *session.Options) { o.FlowIdleTimeout = 300 * time.Second }

func BenchmarkSessionIdleOpenFlowEcho1B(b *testing.B)    { benchOpen(b, withIdle) }
func BenchmarkSessionIdlePingPong64B(b *testing.B)       { benchPingPong(b, 64, withIdle) }
func BenchmarkSessionIdlePingPong32KiB(b *testing.B)     { benchPingPong(b, 32<<10, withIdle) }
func BenchmarkSessionIdleStreamEcho256KiB(b *testing.B)  { benchStream(b, 256<<10, withIdle) }
