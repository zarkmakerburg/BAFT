package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/resources"
	"github.com/zarkmakerburg/baft/internal/routes"
)

// Shared transport multiplexing (docs/design/shared-multiplexing.md): one EX
// listener serves several tenants (authenticated IR identities), each with its
// own routes, and tenants can be added or removed without a restart.

var (
	// ErrNoListener means ReloadListener ran on a node with no running listener.
	ErrNoListener = errors.New("no running listener to reload")
	// ErrReloadUnsupported means the listener cannot change tenants live.
	ErrReloadUnsupported = errors.New("live tenant reload requires a Noise listener")
)

// TenantReload reports what ReloadListener changed.
type TenantReload struct {
	Added   []string
	Removed []string
	Routes  int
}

type handlerBox struct{ h http.Handler }

type listenerLive struct {
	mu      sync.Mutex
	cfg     config.Config
	table   *routes.Table
	allowed map[string]struct{}
	mk      func(config.Config) (http.Handler, error)
	cur     atomic.Pointer[handlerBox]
	cancels map[string]map[uint64]context.CancelFunc
	seq     uint64
	slots   map[string]*resources.FlowSlots
	nodeCap *resources.FlowSlots
	perPeer int
}

func (r *Runtime) newListenerLive(cfg config.Config, table *routes.Table) (*listenerLive, error) {
	l := &listenerLive{
		cfg: cfg, table: table, allowed: identitySet(cfg.Server.AllowedPeerIdentities),
		cancels: map[string]map[uint64]context.CancelFunc{}, slots: map[string]*resources.FlowSlots{},
		nodeCap: r.FlowSlots, perPeer: cfg.Limits.MaxFlowsPerPeer,
	}
	r.liveMu.Lock()
	r.live = l
	r.liveMu.Unlock()
	return l, nil
}

func (r *Runtime) clearListenerLive(l *listenerLive) {
	r.liveMu.Lock()
	if r.live == l {
		r.live = nil
	}
	r.liveMu.Unlock()
}

func identitySet(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func (l *listenerLive) installNoise(mk func(config.Config) (http.Handler, error), h http.Handler) {
	l.mu.Lock()
	l.mk = mk
	l.mu.Unlock()
	l.cur.Store(&handlerBox{h})
}

// handler is a stable http.Handler that always dispatches to the current one.
func (l *listenerLive) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		l.cur.Load().h.ServeHTTP(w, req)
	})
}

func (l *listenerLive) authorized(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.allowed[id]
	return ok
}

// slotsFor returns the flow slots a tenant's sessions share: the node-wide
// pool, or a per-tenant child of it when limits.max_flows_per_peer is set.
func (l *listenerLive) slotsFor(id string) *resources.FlowSlots {
	if l.perPeer <= 0 || l.nodeCap == nil {
		return l.nodeCap
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if s := l.slots[id]; s != nil {
		return s
	}
	s, err := l.nodeCap.NewChild(l.perPeer)
	if err != nil {
		return l.nodeCap
	}
	l.slots[id] = s
	return s
}

// track derives a context that ReloadListener cancels when the tenant is
// removed. release must be called when the session ends.
func (l *listenerLive) track(id string, parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	l.mu.Lock()
	if _, ok := l.allowed[id]; !ok {
		l.mu.Unlock()
		cancel()
		return ctx, func() {}
	}
	l.seq++
	n := l.seq
	if l.cancels[id] == nil {
		l.cancels[id] = map[uint64]context.CancelFunc{}
	}
	l.cancels[id][n] = cancel
	l.mu.Unlock()
	return ctx, func() {
		l.mu.Lock()
		delete(l.cancels[id], n)
		if len(l.cancels[id]) == 0 {
			delete(l.cancels, id)
		}
		l.mu.Unlock()
		cancel()
	}
}

// staticShape is the part of a listener config a live reload must not change.
func staticShape(c config.Config) ([]byte, error) {
	c.Routes = nil
	if c.Server != nil {
		s := *c.Server
		s.AllowedPeerIdentities = nil
		c.Server = &s
	}
	if c.Noise != nil {
		n := *c.Noise
		n.AllowedPeerPublicKeys = nil
		n.PeerPublicKey = ""
		c.Noise = &n
	}
	return json.Marshal(c)
}

// ReloadListener applies a changed tenant set (routes, allowed peer
// identities, allowed Noise keys) to the running listener without restarting
// it. Sessions of tenants that stay allowed are not interrupted; sessions of a
// removed tenant are cancelled. Any other difference, or an invalid config,
// is refused and leaves the running state untouched.
func (r *Runtime) ReloadListener(newCfg config.Config) (TenantReload, error) {
	r.liveMu.Lock()
	l := r.live
	r.liveMu.Unlock()
	if l == nil {
		return TenantReload{}, ErrNoListener
	}
	if err := config.Validate(newCfg); err != nil {
		return TenantReload{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.mk == nil {
		return TenantReload{}, ErrReloadUnsupported
	}
	if newCfg.Node.Role != "listener" || newCfg.Server == nil || newCfg.Noise == nil {
		return TenantReload{}, ErrReloadUnsupported
	}
	oldShape, err := staticShape(l.cfg)
	if err != nil {
		return TenantReload{}, err
	}
	newShape, err := staticShape(newCfg)
	if err != nil {
		return TenantReload{}, err
	}
	if string(oldShape) != string(newShape) {
		return TenantReload{}, errors.New("reload refused: settings other than routes and allowed peers changed; restart required")
	}
	var defs []routes.Route
	for _, cr := range newCfg.Routes {
		if cr.Direction != "inbound" {
			continue
		}
		defs = append(defs, routes.Route{ID: cr.ID, Target: cr.Target, AllowedPeers: identitySet(cr.AllowedPeers)})
	}
	if len(defs) == 0 {
		return TenantReload{}, errors.New("listener requires at least one inbound route")
	}
	if _, err := routes.New(defs); err != nil {
		return TenantReload{}, err
	}
	h, err := l.mk(newCfg)
	if err != nil {
		return TenantReload{}, fmt.Errorf("reload refused: %w", err)
	}
	newAllowed := identitySet(newCfg.Server.AllowedPeerIdentities)
	var res TenantReload
	res.Routes = len(defs)
	for id := range newAllowed {
		if _, ok := l.allowed[id]; !ok {
			res.Added = append(res.Added, id)
		}
	}
	for id := range l.allowed {
		if _, ok := newAllowed[id]; !ok {
			res.Removed = append(res.Removed, id)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Removed)
	// Routes first so a newly admitted tenant always finds them; a removed
	// tenant's sessions are cancelled last, after it can no longer authenticate.
	if err := l.table.Replace(defs); err != nil {
		return TenantReload{}, err
	}
	l.allowed = newAllowed
	l.cur.Store(&handlerBox{h})
	l.cfg = newCfg
	for _, id := range res.Removed {
		for _, cancel := range l.cancels[id] {
			cancel()
		}
	}
	return res, nil
}
