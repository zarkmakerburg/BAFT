package bcc

import (
	"net/http"
	"sort"
	"time"
)

// The overview is what the Command Center dashboard shows at a glance. Every
// number is derived from what nodes reported; a value BCC cannot back with a
// measurement is null, never zero or an estimate.

type OverviewNode struct {
	ID        string     `json:"id"`
	Alias     string     `json:"alias"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	LatencyMS *int64     `json:"latency_ms"`
	Country   string     `json:"country,omitempty"`
	Lat       *float64   `json:"lat,omitempty"`
	Lon       *float64   `json:"lon,omitempty"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	Revoked   bool       `json:"revoked,omitempty"`
	// Spark is the node's recent health-check latency, oldest first.
	Spark []int64 `json:"spark"`
}

type OverviewLink struct {
	TunnelID string `json:"tunnel_id"`
	IRNode   string `json:"ir_node"`
	EXNode   string `json:"ex_node"`
	Phase    string `json:"phase"`
	Port     int    `json:"port"`
}

type TrafficPoint struct {
	Time   time.Time `json:"t"`
	InBps  *float64  `json:"in_bps"`
	OutBps *float64  `json:"out_bps"`
}

type Overview struct {
	GeneratedAt time.Time `json:"generated_at"`
	Version     string    `json:"version"`
	Servers     struct {
		Online int `json:"online"`
		Total  int `json:"total"`
	} `json:"servers"`
	Tunnels struct {
		Active   int `json:"active"`
		Building int `json:"building"`
		Failed   int `json:"failed"`
	} `json:"tunnels"`
	LatencyMS *int64 `json:"latency_ms"`
	Traffic   struct {
		InBps  *float64 `json:"in_bps"`
		OutBps *float64 `json:"out_bps"`
		// Source names the nodes the totals add up: "foreign" when any exit
		// node reports traffic (each user byte crosses exactly one exit), else
		// "all".
		Source string `json:"source"`
	} `json:"traffic"`
	Alerts struct {
		Active   int `json:"active"`
		Critical int `json:"critical"`
	} `json:"alerts"`
	Nodes  []OverviewNode `json:"nodes"`
	Links  []OverviewLink `json:"links"`
	Series struct {
		Window        string         `json:"window"`
		BucketSeconds int            `json:"bucket_seconds"`
		Points        []TrafficPoint `json:"points"`
	} `json:"series"`
}

const (
	overviewSparkPoints = 24
	// A gap longer than this between two reports is an outage, not a rate.
	overviewMaxSampleGap = 15 * time.Minute
)

type rateSample struct {
	at      time.Time
	in, out float64
}

// nodeRates turns cumulative counters into per-interval rates. A counter that
// went backwards (agent restart) or a long gap starts a new run.
func nodeRates(points []HistoryPoint) []rateSample {
	out := make([]rateSample, 0, len(points))
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		dt := b.Timestamp.Sub(a.Timestamp)
		if dt <= 0 || dt > overviewMaxSampleGap || b.IngressBytes < a.IngressBytes || b.EgressBytes < a.EgressBytes {
			continue
		}
		sec := dt.Seconds()
		out = append(out, rateSample{at: b.Timestamp,
			in:  float64(b.IngressBytes-a.IngressBytes) * 8 / sec,
			out: float64(b.EgressBytes-a.EgressBytes) * 8 / sec})
	}
	return out
}

func tunnelFinal(phase string) bool {
	switch phase {
	case TunnelActive, TunnelSuperseded, TunnelRolledBack, TunnelRollbackFailed, TunnelDecommissioned, TunnelDecommissionFailed:
		return true
	}
	return false
}

func (s *Server) Overview(now time.Time, window time.Duration) Overview {
	now = now.UTC()
	var o Overview
	o.GeneratedAt = now
	o.Version = s.version
	if o.Version == "" {
		o.Version = "dev"
	}
	stale := s.alertConfig.TelemetryStaleAfter
	if stale <= 0 {
		stale = 3 * time.Minute
	}
	monitor := map[string]MonitoringNode{}
	for _, m := range s.store.MonitoringSnapshot(now, stale) {
		monitor[m.NodeID] = m
	}

	var latencySum, latencyN int64
	rates := map[string][]rateSample{}
	o.Nodes = []OverviewNode{}
	for _, n := range s.store.ListNodes() {
		m := monitor[n.ID]
		on := OverviewNode{ID: n.ID, Alias: n.Alias, Role: n.Role, Status: m.Status, Country: n.Country, Revoked: n.Revoked, Spark: []int64{}}
		if on.Status == "" || n.Revoked {
			on.Status = "unknown"
			if n.Revoked {
				on.Status = "revoked"
			}
		}
		if p, ok := countryPoints[n.Country]; ok {
			lat, lon := p.Lat, p.Lon
			on.Lat, on.Lon = &lat, &lon
		}
		if !m.LastSeen.IsZero() {
			t := m.LastSeen
			on.LastSeen = &t
		}
		if on.Status == "up" && n.LatencyMS >= 0 && !n.LastChecked.IsZero() {
			l := n.LatencyMS
			on.LatencyMS = &l
			latencySum += l
			latencyN++
		}
		hist := s.store.History(n.ID, now)
		for i := max(0, len(hist)-overviewSparkPoints); i < len(hist); i++ {
			if hist[i].LatencyMS >= 0 {
				on.Spark = append(on.Spark, hist[i].LatencyMS)
			}
		}
		rates[n.ID] = nodeRates(hist)
		if !n.Revoked {
			o.Servers.Total++
			if on.Status == "up" {
				o.Servers.Online++
			}
		}
		o.Nodes = append(o.Nodes, on)
	}
	if latencyN > 0 {
		avg := latencySum / latencyN
		o.LatencyMS = &avg
	}

	o.Links = []OverviewLink{}
	for _, t := range s.store.ListTunnels() {
		switch {
		case t.Phase == TunnelActive:
			o.Tunnels.Active++
			o.Links = append(o.Links, OverviewLink{TunnelID: t.ID, IRNode: t.IRNode, EXNode: t.EXNode, Phase: t.Phase, Port: t.Port})
		case t.Phase == TunnelRollbackFailed || t.Phase == TunnelDecommissionFailed:
			o.Tunnels.Failed++
		case !tunnelFinal(t.Phase):
			o.Tunnels.Building++
			o.Links = append(o.Links, OverviewLink{TunnelID: t.ID, IRNode: t.IRNode, EXNode: t.EXNode, Phase: t.Phase, Port: t.Port})
		}
	}

	for _, a := range s.store.ActiveAlertsSnapshot() {
		if a.Status == "resolved" {
			continue
		}
		o.Alerts.Active++
		if a.Severity == "critical" {
			o.Alerts.Critical++
		}
	}

	// Traffic: add up exits when any exit reports, so a byte that crossed an
	// IR and an EX node is not counted twice.
	o.Traffic.Source = "all"
	counted := map[string]bool{}
	for _, n := range o.Nodes {
		if n.Role == "foreign" && len(rates[n.ID]) > 0 {
			o.Traffic.Source = "foreign"
		}
	}
	for _, n := range o.Nodes {
		counted[n.ID] = !n.Revoked && (o.Traffic.Source == "all" || n.Role == "foreign")
	}
	var curIn, curOut float64
	current := false
	for id, rs := range rates {
		if !counted[id] || len(rs) == 0 {
			continue
		}
		last := rs[len(rs)-1]
		if now.Sub(last.at) < stale && monitor[id].Status == "up" {
			curIn += last.in
			curOut += last.out
			current = true
		}
	}
	if current {
		o.Traffic.InBps, o.Traffic.OutBps = &curIn, &curOut
	}

	if window <= 0 {
		window = 24 * time.Hour
	}
	bucket := window / 96
	o.Series.Window = window.String()
	if window == 24*time.Hour {
		o.Series.Window = "24h"
	} else if window == 7*24*time.Hour {
		o.Series.Window = "7d"
	}
	o.Series.BucketSeconds = int(bucket / time.Second)
	start := now.Add(-window).Truncate(bucket)
	n := int(now.Sub(start)/bucket) + 1
	// Each bucket averages every node's samples in it, then adds the nodes.
	type sum struct {
		in, out float64
		n       int
	}
	buckets := make([]map[string]*sum, n)
	for id, rs := range rates {
		if !counted[id] {
			continue
		}
		for _, r := range rs {
			if r.at.Before(start) || r.at.After(now) {
				continue
			}
			i := int(r.at.Sub(start) / bucket)
			if buckets[i] == nil {
				buckets[i] = map[string]*sum{}
			}
			if buckets[i][id] == nil {
				buckets[i][id] = &sum{}
			}
			c := buckets[i][id]
			c.in, c.out, c.n = c.in+r.in, c.out+r.out, c.n+1
		}
	}
	o.Series.Points = make([]TrafficPoint, n)
	for i, b := range buckets {
		p := TrafficPoint{Time: start.Add(time.Duration(i) * bucket)}
		if len(b) > 0 {
			var in, out float64
			for _, c := range b {
				in += c.in / float64(c.n)
				out += c.out / float64(c.n)
			}
			p.InBps, p.OutBps = &in, &out
		}
		o.Series.Points[i] = p
	}
	sort.Slice(o.Nodes, func(i, j int) bool {
		if (o.Nodes[i].Role == "foreign") != (o.Nodes[j].Role == "foreign") {
			return o.Nodes[i].Role != "foreign"
		}
		return o.Nodes[i].Alias < o.Nodes[j].Alias
	})
	return o
}

func (s *Server) overviewAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.admin(w, r) {
		return
	}
	window := 24 * time.Hour
	switch r.URL.Query().Get("window") {
	case "", "24h":
	case "7d":
		window = 7 * 24 * time.Hour
	default:
		http.Error(w, "window must be 24h or 7d", 400)
		return
	}
	writeJSON(w, http.StatusOK, s.Overview(s.now(), window))
}
