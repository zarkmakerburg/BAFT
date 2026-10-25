package node

import (
	"errors"
	"log"
	"sync"

	baftmetrics "github.com/zarkmakerburg/baft/internal/metrics"
	"github.com/zarkmakerburg/baft/internal/protocol"
	"github.com/zarkmakerburg/baft/internal/session"
)

// capabilityStats counts refused capability negotiations by bounded reason.
// The zero value is ready to use.
type capabilityStats struct {
	mu       sync.Mutex
	refusals map[string]uint64
}

func (s *capabilityStats) refuse(reason string) {
	s.mu.Lock()
	if s.refusals == nil {
		s.refusals = map[string]uint64{}
	}
	s.refusals[reason]++
	s.mu.Unlock()
}

func (s *capabilityStats) snapshot() map[string]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint64, len(s.refusals))
	for k, v := range s.refusals {
		out[k] = v
	}
	return out
}

// observeCapabilities is the session.Options.CapabilityObserver of every
// Session of this node. A refusal is logged with its typed reason and counted
// as a handshake error; the session then ends and its carrier is closed, so no
// weaker mode is ever entered.
func (r *Runtime) observeCapabilities(_ protocol.NegotiatedCapabilities, err error) {
	if err == nil {
		return
	}
	reason, missing := protocol.RefusalInvalid, []string(nil)
	var ce *protocol.CapabilityError
	if errors.As(err, &ce) {
		reason, missing = ce.Reason, ce.Missing
	}
	r.capStats.refuse(reason)
	r.handshakeErrors.Add(1)
	log.Printf("baft capability negotiation refused reason=%s missing=%v", reason, missing)
}

// fillCapabilityMetrics records, per capability, how many live sessions
// negotiated it, how many run on the v0.3.7 baseline, and refusals by reason.
func (r *Runtime) fillCapabilityMetrics(out *baftmetrics.Snapshot, peers []*session.Peer) {
	out.CapabilitySessions = map[string]uint64{}
	for _, p := range peers {
		n, ok := p.NegotiatedCapabilities()
		if !ok {
			continue
		}
		if n.Baseline {
			out.CapabilityBaselineSessions++
		}
		for _, c := range n.Enabled {
			out.CapabilitySessions[c]++
		}
	}
	out.CapabilityRefusals = r.capStats.snapshot()
}
