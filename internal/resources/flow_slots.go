package resources

import (
	"errors"
	"sync"
)

// FlowSlots bounds the number of concurrently open Flows across every Session
// of one node (limits.max_flows). A nil *FlowSlots admits every Flow.
type FlowSlots struct {
	mu     sync.Mutex
	max    int
	used   int
	parent *FlowSlots
}

// NewChild returns a pool capped at max whose every slot is also taken from
// s, so a tenant ceiling can never exceed (or starve others beyond) the node
// limit. Release returns the slot to both levels.
func (s *FlowSlots) NewChild(max int) (*FlowSlots, error) {
	if max <= 0 {
		return nil, errors.New("flow slot limit must be positive")
	}
	return &FlowSlots{max: max, parent: s}, nil
}

func NewFlowSlots(max int) (*FlowSlots, error) {
	if max <= 0 { return nil, errors.New("flow slot limit must be positive") }
	return &FlowSlots{max:max}, nil
}

func (s *FlowSlots) TryAcquire() bool {
	if s == nil { return true }
	s.mu.Lock()
	if s.used >= s.max {
		s.mu.Unlock()
		return false
	}
	s.used++
	s.mu.Unlock()
	if s.parent != nil && !s.parent.TryAcquire() {
		s.mu.Lock()
		s.used--
		s.mu.Unlock()
		return false
	}
	return true
}

func (s *FlowSlots) Release() {
	if s == nil { return }
	s.mu.Lock()
	released := s.used > 0
	if released { s.used-- }
	s.mu.Unlock()
	if released && s.parent != nil { s.parent.Release() }
}

func (s *FlowSlots) Used() int {
	if s == nil { return 0 }
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}
