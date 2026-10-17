package bcc

import (
	"crypto/x509"
	"encoding/base64"
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
	// InBps/OutBps are the node's latest measured rate while it is reporting.
	InBps  *float64 `json:"in_bps"`
	OutBps *float64 `json:"out_bps"`
	// UptimePct is the share of the window, since the node's first report in
	// it, that the node was reporting without a gap.
	UptimePct *float64   `json:"uptime_pct"`
	AgentSeen *time.Time `json:"agent_seen,omitempty"`
}

// OverviewCert is what BCC knows about a tunnel's current certificate: the
// expiry is known only for certificates BCC issued through a rotation.
type OverviewCert struct {
	TunnelID string     `json:"tunnel_id"`
	Epoch    int        `json:"epoch"`
	NotAfter *time.Time `json:"not_after"`
}

type OverviewHealth struct {
	BackupsConfigured bool           `json:"backups_configured"`
	Backups           int            `json:"backups"`
	LastBackup        *time.Time     `json:"last_backup"`
	Certs             []OverviewCert `json:"certs"`
	RotationsActive   int            `json:"rotations_active"`
	RotationsFailed   int            `json:"rotations_failed"`
	// AgentsStale counts enrolled servers whose agent has not called BCC
	// within the telemetry staleness limit.
	AgentsStale int `json:"agents_stale"`
}

type OverviewFinance struct {
	Currency      string `json:"currency"`
	Bytes         uint64 `json:"bytes"`
	CostMicros    int64  `json:"cost_micros"`
	RevenueMicros int64  `json:"revenue_micros"`
	ProfitMicros  int64  `json:"profit_micros"`
	// CostPerGBMicros is null until the month has carried traffic.
	CostPerGBMicros *int64 `json:"cost_per_gb_micros"`
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
	Health OverviewHealth `json:"health"`
	// Finance is the current month (Asia/Tehran), one row per currency.
	Finance struct {
		Month string            `json:"month"`
		Rows  []OverviewFinance `json:"rows"`
	} `json:"finance"`
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

	if window <= 0 {
		window = 24 * time.Hour
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
		} else if !n.AgentSeen.IsZero() {
			t := n.AgentSeen
			on.LastSeen = &t
		}
		if on.Status == "up" && m.HealthCheckStatus == "up" && m.LatencyMS >= 0 {
			l := m.LatencyMS
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
		if rs := rates[n.ID]; len(rs) > 0 && on.Status == "up" && monitoringFresh(rs[len(rs)-1].at, now, stale) {
			in, out := rs[len(rs)-1].in, rs[len(rs)-1].out
			on.InBps, on.OutBps = &in, &out
		}
		on.UptimePct = uptime(hist, now, window, stale)
		if !n.AgentSeen.IsZero() {
			t := n.AgentSeen
			on.AgentSeen = &t
		}
		if !n.Revoked && !monitoringFresh(n.AgentSeen, now, stale) {
			o.Health.AgentsStale++
		}
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
		if monitoringFresh(last.at, now, stale) && monitor[id].Status == "up" {
			curIn += last.in
			curOut += last.out
			current = true
		}
	}
	if current {
		o.Traffic.InBps, o.Traffic.OutBps = &curIn, &curOut
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
	s.overviewHealth(&o, now)
	s.overviewFinance(&o, now)
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

// uptime is the share of the window, from the node's first report in it, that
// consecutive reports were no further apart than the outage gap. The time
// since the last report counts while it is still within the staleness limit.
func uptime(hist []HistoryPoint, now time.Time, window, stale time.Duration) *float64 {
	start := now.Add(-window)
	var first, prev time.Time
	var covered time.Duration
	for _, p := range hist {
		if p.Timestamp.Before(start) || p.Timestamp.After(now) {
			continue
		}
		if first.IsZero() {
			first = p.Timestamp
		} else if gap := p.Timestamp.Sub(prev); gap <= overviewMaxSampleGap {
			covered += gap
		}
		prev = p.Timestamp
	}
	if first.IsZero() {
		return nil
	}
	if tail := now.Sub(prev); tail <= stale {
		covered += tail
	}
	span := now.Sub(first)
	if span < time.Minute {
		return nil
	}
	pct := min(100, float64(covered)/float64(span)*100)
	return &pct
}

func (s *Server) overviewHealth(o *Overview, now time.Time) {
	o.Health.Certs = []OverviewCert{}
	if entries, err := s.listConfiguredBackups(); err == nil {
		o.Health.BackupsConfigured = true
		o.Health.Backups = len(entries)
		for _, e := range entries {
			if o.Health.LastBackup == nil || e.ModifiedAt.After(*o.Health.LastBackup) {
				t := e.ModifiedAt
				o.Health.LastBackup = &t
			}
		}
	}
	rotations := s.store.ListCertRotations()
	for _, r := range rotations {
		switch r.Phase {
		case CertRotComplete, CertRotRolledBack:
		case CertRotRollbackFailed, CertRotRetireFailed:
			o.Health.RotationsFailed++
		default:
			o.Health.RotationsActive++
		}
	}
	for _, t := range s.store.ListTunnels() {
		if t.Phase != TunnelActive {
			continue
		}
		c := OverviewCert{TunnelID: t.ID, Epoch: t.CertEpoch}
		for _, r := range rotations {
			if r.TunnelID == t.ID && r.Epoch == t.CertEpoch && r.Phase == CertRotComplete && t.CertEpoch > 0 {
				if der, err := base64.RawURLEncoding.DecodeString(r.CertDER); err == nil {
					if cert, err := x509.ParseCertificate(der); err == nil {
						na := cert.NotAfter.UTC()
						c.NotAfter = &na
					}
				}
			}
		}
		o.Health.Certs = append(o.Health.Certs, c)
	}
	sort.Slice(o.Health.Certs, func(i, j int) bool { return o.Health.Certs[i].TunnelID < o.Health.Certs[j].TunnelID })
}

func (s *Server) overviewFinance(o *Overview, now time.Time) {
	o.Finance.Rows = []OverviewFinance{}
	loc, err := reportLocation("")
	if err != nil {
		return
	}
	local := now.In(loc)
	o.Finance.Month = local.Format("2006-01")
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	rows, err := s.store.FinanceReport("monthly", first.Format("2006-01-02"), local.Format("2006-01-02"), "")
	if err != nil {
		return
	}
	// Bytes count exit servers only, as the traffic totals do, so a user byte
	// is not billed twice when it crosses an IR and an EX server.
	role := map[string]string{}
	for _, n := range o.Nodes {
		role[n.ID] = n.Role
	}
	exitBytes, anyExit := map[string]uint64{}, map[string]bool{}
	for _, r := range rows {
		if r.Scope == "node" && role[r.NodeID] == "foreign" {
			exitBytes[r.Currency] += r.IngressBytes + r.EgressBytes
			anyExit[r.Currency] = true
		}
	}
	for _, r := range rows {
		if r.Scope != "cluster" {
			continue
		}
		f := OverviewFinance{Currency: r.Currency, Bytes: r.IngressBytes + r.EgressBytes, CostMicros: r.CostMicros, RevenueMicros: r.RevenueMicros, ProfitMicros: r.ProfitMicros}
		if anyExit[r.Currency] {
			f.Bytes = exitBytes[r.Currency]
		}
		if gb := float64(f.Bytes) / 1e9; gb >= 0.001 {
			v := int64(float64(f.CostMicros) / gb)
			f.CostPerGBMicros = &v
		}
		o.Finance.Rows = append(o.Finance.Rows, f)
	}
}
