package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const SchemaVersion = 1

type Config struct {
	SchemaVersion int        `json:"schema_version"`
	Noise         *Noise     `json:"noise,omitempty"`
	Node          Node       `json:"node"`
	Peer          *Peer      `json:"peer,omitempty"`
	Server        *Server    `json:"server,omitempty"`
	TLS           TLS        `json:"tls"`
	Transport     Transport  `json:"transport"`
	Limits        Limits     `json:"limits"`
	Recovery      Recovery   `json:"recovery"`
	Routes        []Route    `json:"routes"`
	Management    Management `json:"management"`
	Telemetry     Telemetry  `json:"telemetry,omitempty"`
	Revocation    *Revocation `json:"revocation,omitempty"`
	Logging       Logging    `json:"logging"`
}

type Node struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}
type Peer struct {
	Address            string `json:"address"`
	ServerName         string `json:"server_name"`
	AllowedIdentity    string `json:"allowed_identity"`
	// Fallback* optionally describe a distinct network path for
	// Transport.Fallback. Empty values inherit the primary peer/TLS values.
	FallbackAddress    string `json:"fallback_address,omitempty"`
	FallbackServerName string `json:"fallback_server_name,omitempty"`
	FallbackCAFile     string `json:"fallback_ca_file,omitempty"`
}
type Server struct {
	Listen                string   `json:"listen"`
	ServerName            string   `json:"server_name"`
	AllowedPeerIdentities []string `json:"allowed_peer_identities"`
}
type TLS struct {
	MinVersion     string `json:"min_version"`
	CAFile         string `json:"ca_file"`
	CertFile       string `json:"cert_file,omitempty"`
	KeyFile        string `json:"key_file,omitempty"`
	SessionTickets bool   `json:"session_tickets"`
}
type Transport struct {
	Primary   string `json:"primary"`
	// Fallback is an optional second physical carrier used only by same-process
	// recovery. It must differ from Primary; switching it never changes logical
	// Session identity or authority.
	Fallback  string `json:"fallback,omitempty"`
	H3Enabled bool   `json:"h3_enabled"`
	Shards    int    `json:"shards"`
	Profile   string `json:"profile"`
	// UTLS makes the carrier dial present a browser-fidelity (Chrome) TLS
	// ClientHello instead of Go's distinctive one, closing the passive
	// fingerprint gap. Supported on the ws transport.
	UTLS bool `json:"utls,omitempty"`
}
type Limits struct {
	MaxFlows          int `json:"max_flows"`
	DataMemoryMiB     int `json:"data_memory_mib"`
	ReceiveInitialKiB int `json:"receive_initial_kib"`
	ReceiveMaxMiB     int `json:"receive_max_mib"`
	ReplayMaxMiB      int `json:"replay_max_mib"`
	// FlowIdleSeconds closes a carried Flow that moved no bytes in either
	// direction for this long. Zero (default) keeps the historical behaviour
	// of no per-Flow idle limit.
	FlowIdleSeconds int `json:"flow_idle_seconds,omitempty"`
	// MaxFlowsPerPeer caps concurrently open Flows for any one authenticated
	// peer (tenant) of a shared listener, on top of max_flows. Zero means no
	// per-peer cap beyond max_flows.
	MaxFlowsPerPeer int `json:"max_flows_per_peer,omitempty"`
}
type Recovery struct {
	Enabled          bool   `json:"enabled"`
	RetentionSeconds int    `json:"retention_seconds"`
	Mode             string `json:"mode,omitempty"`
	Durable          bool   `json:"durable,omitempty"`
}
type Route struct {
	ID           string   `json:"id"`
	Listen       string   `json:"listen,omitempty"`
	RemoteRoute  string   `json:"remote_route,omitempty"`
	Direction    string   `json:"direction"`
	TrafficClass string   `json:"traffic_class,omitempty"`
	Target       string   `json:"target,omitempty"`
	AllowedPeers []string `json:"allowed_peers,omitempty"`
	// Public lets an outbound route listen on a non-loopback address, so
	// clients on other hosts reach the carried service through this node
	// (e.g. x-ui inbounds served from the IR's public IP). BAFT does not
	// authenticate those clients; the carried service must.
	Public bool `json:"public,omitempty"`
}
type Management struct {
	UnixSocket    string `json:"unix_socket"`
	MetricsListen string `json:"metrics_listen"`
}
type Telemetry struct {
	Enabled                   bool   `json:"enabled"`
	BCCURL                    string `json:"bcc_url,omitempty"`
	AgentTokenEnv             string `json:"agent_token_env,omitempty"`
	IntervalSeconds           int    `json:"interval_seconds,omitempty"`
	RouteProbeIntervalSeconds int    `json:"route_probe_interval_seconds,omitempty"`
	SpoolPath                 string `json:"spool_path,omitempty"`
	SpoolMaxPending           int    `json:"spool_max_pending,omitempty"`
}
type Logging struct {
	Level   string `json:"level"`
	Payload bool   `json:"payload"`
}

func DecodeJSON(r io.Reader) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("config must contain exactly one JSON document")
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Validate(c Config) error {
	if err := validateNoise(c); err != nil {
		return err
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d", c.SchemaVersion)
	}
	if c.Node.ID == "" || len(c.Node.ID) > 64 {
		return errors.New("node.id must be 1..64 bytes")
	}
	if c.Node.Role != "dialer" && c.Node.Role != "listener" {
		return errors.New("node.role must be dialer or listener")
	}
	if c.TLS.MinVersion != "1.3" {
		return errors.New("tls.min_version must be 1.3")
	}
	if c.TLS.SessionTickets {
		return errors.New("tls.session_tickets must be false in baseline")
	}
	if c.TLS.CAFile == "" {
		return errors.New("tls.ca_file is required")
	}
	if (c.TLS.CertFile == "") != (c.TLS.KeyFile == "") {
		return errors.New("tls cert_file and key_file must be set together")
	}
	// A Noise dialer authenticates with its pinned static key and presents no
	// outer client certificate, so only it may omit the certificate pair.
	if c.TLS.CertFile == "" && (c.Noise == nil || c.Node.Role != "dialer") {
		return errors.New("tls ca_file/cert_file/key_file are required")
	}
	if c.Transport.Primary != "h2" && c.Transport.Primary != "ws" {
		return errors.New("transport.primary must be h2 or ws")
	}
	if c.Transport.Fallback != "" {
		if c.Transport.Fallback != "h2" && c.Transport.Fallback != "ws" {
			return errors.New("transport.fallback must be empty, h2, or ws")
		}
		if c.Transport.Fallback == c.Transport.Primary {
			return errors.New("transport.fallback must differ from transport.primary")
		}
		if !c.Recovery.Enabled {
			return errors.New("transport.fallback requires recovery.enabled")
		}
	}
	hasWS := c.Transport.Primary == "ws" || c.Transport.Fallback == "ws"
	if c.Transport.UTLS && !hasWS {
		return errors.New("transport.utls requires ws as primary or fallback")
	}
	if hasWS && c.Noise == nil {
		// Behind Cloudflare the outer TLS is terminated at the edge, so every
		// WebSocket carrier relies entirely on the inner Noise handshake for
		// authentication; it must be configured even when ws is only fallback.
		return errors.New("transport ws requires noise")
	}
	if c.Transport.H3Enabled {
		return errors.New("transport.h3_enabled is unsupported before H2 gate passes")
	}
	if c.Transport.Shards < 1 || c.Transport.Shards > 8 {
		return errors.New("transport.shards must be between 1 and 8")
	}
	if c.Transport.Profile == "" {
		return errors.New("transport.profile is required")
	}
	if c.Limits.MaxFlows < 1 || c.Limits.MaxFlows > 65535 {
		return errors.New("limits.max_flows is out of supported range")
	}
	if c.Limits.DataMemoryMiB < 16 || c.Limits.DataMemoryMiB > 4096 {
		return errors.New("limits.data_memory_mib is unreasonable")
	}
	if c.Limits.ReceiveInitialKiB < 1 || c.Limits.ReceiveMaxMiB < 1 || c.Limits.ReplayMaxMiB < 1 {
		return errors.New("receive/replay limits must be positive")
	}
	if c.Limits.MaxFlowsPerPeer < 0 || c.Limits.MaxFlowsPerPeer > c.Limits.MaxFlows {
		return errors.New("limits.max_flows_per_peer must be between 0 and limits.max_flows")
	}
	if c.Limits.FlowIdleSeconds < 0 || c.Limits.FlowIdleSeconds > 86400 {
		return errors.New("limits.flow_idle_seconds is out of range")
	}
	poolMiB := c.Limits.DataMemoryMiB / 2
	if c.Limits.ReceiveMaxMiB > poolMiB || c.Limits.ReplayMaxMiB > c.Limits.DataMemoryMiB-poolMiB {
		return errors.New("per-flow receive/replay cap exceeds its non-borrowing global pool")
	}
	if c.Recovery.RetentionSeconds < 0 || c.Recovery.RetentionSeconds > 300 {
		return errors.New("recovery.retention_seconds is out of range")
	}
	if c.Recovery.Enabled {
		if c.Recovery.Mode!="" && c.Recovery.Mode!="same_process" { return errors.New("recovery.mode supports only same_process in BAFT 0.3") }
		if c.Recovery.Durable { return errors.New("durable/process-restart recovery is unsupported in step 5.7") }
		if c.Recovery.RetentionSeconds==0 { return errors.New("recovery.retention_seconds must be positive when recovery is enabled") }
	} else if c.Recovery.Durable {
		return errors.New("recovery.durable requires an unsupported process-restart mode")
	}
	if c.Logging.Payload {
		return errors.New("logging.payload must be false")
	}
	if c.Management.UnixSocket == "" {
		return errors.New("management.unix_socket is required")
	}
	if err := validateLoopbackListen(c.Management.MetricsListen, "management.metrics_listen"); err != nil {
		return err
	}
	if err := refuseSSHManagementPort(c.Management.MetricsListen, "management.metrics_listen"); err != nil {
		return err
	}
	if err := validateTelemetry(c.Telemetry); err != nil {
		return err
	}
	if err := validateRevocation(c); err != nil {
		return err
	}
	// Route ids are unique, except that inbound routes may repeat an id for
	// different tenants as long as their allowed_peers sets are disjoint
	// (shared multiplexing: each tenant keeps its own "port-443").
	seen := make(map[string]string, len(c.Routes))
	inboundPeers := make(map[string]map[string]struct{}, len(c.Routes))
	for i, r := range c.Routes {
		if r.ID == "" || len(r.ID) > 64 {
			return fmt.Errorf("routes[%d].id must be 1..64 bytes", i)
		}
		if prev, ok := seen[r.ID]; ok {
			if prev != "inbound" || r.Direction != "inbound" {
				return fmt.Errorf("duplicate route id: %s", r.ID)
			}
			for _, p := range r.AllowedPeers {
				if _, dup := inboundPeers[r.ID][p]; dup {
					return fmt.Errorf("duplicate route id: %s", r.ID)
				}
			}
		}
		seen[r.ID] = r.Direction
		if r.Direction == "inbound" {
			if inboundPeers[r.ID] == nil {
				inboundPeers[r.ID] = map[string]struct{}{}
			}
			for _, p := range r.AllowedPeers {
				inboundPeers[r.ID][p] = struct{}{}
			}
		}
		if r.Direction != "outbound" && r.Direction != "inbound" {
			return fmt.Errorf("route %s: invalid direction", r.ID)
		}
		if r.Direction == "outbound" {
			if r.Listen == "" || r.RemoteRoute == "" || r.Target != "" {
				return fmt.Errorf("route %s: outbound route requires listen+remote_route and no target", r.ID)
			}
			check := validateLoopbackListen
			if r.Public {
				check = validatePublicListen
			}
			if err := check(r.Listen, "route.listen"); err != nil {
				return fmt.Errorf("route %s: %w", r.ID, err)
			}
			if err := refuseSSHManagementPort(r.Listen, "route.listen"); err != nil {
				return fmt.Errorf("route %s: %w", r.ID, err)
			}
		} else {
			if r.Public {
				return fmt.Errorf("route %s: public applies to outbound routes only", r.ID)
			}
			if r.Target == "" || r.Listen != "" || r.RemoteRoute != "" {
				return fmt.Errorf("route %s: inbound route requires target and no listen/remote_route", r.ID)
			}
			if err := validateFixedTarget(r.Target); err != nil {
				return fmt.Errorf("route %s: %w", r.ID, err)
			}
			if len(r.AllowedPeers) == 0 {
				return fmt.Errorf("route %s: allowed_peers must not be empty", r.ID)
			}
		}
	}
	switch c.Node.Role {
	case "dialer":
		if c.Peer == nil || c.Server != nil {
			return errors.New("dialer requires peer and must not define server")
		}
		if c.Peer.Address == "" || c.Peer.ServerName == "" || c.Peer.AllowedIdentity == "" {
			return errors.New("peer address/server_name/allowed_identity are required")
		}
		if _, _, err := net.SplitHostPort(c.Peer.Address); err != nil {
			return fmt.Errorf("peer.address: %w", err)
		}
		fallbackPeerSet := c.Peer.FallbackAddress != "" || c.Peer.FallbackServerName != "" || c.Peer.FallbackCAFile != ""
		if fallbackPeerSet && c.Transport.Fallback == "" {
			return errors.New("peer fallback path requires transport.fallback")
		}
		if c.Peer.FallbackAddress != "" {
			if _, _, err := net.SplitHostPort(c.Peer.FallbackAddress); err != nil {
				return fmt.Errorf("peer.fallback_address: %w", err)
			}
		}
	case "listener":
		if c.Server == nil || c.Peer != nil {
			return errors.New("listener requires server and must not define peer")
		}
		if c.Server.Listen == "" || c.Server.ServerName == "" || len(c.Server.AllowedPeerIdentities) == 0 {
			return errors.New("server listen/server_name/allowed_peer_identities are required")
		}
		if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
			return fmt.Errorf("server.listen: %w", err)
		}
		if err := refuseSSHManagementPort(c.Server.Listen, "server.listen"); err != nil {
			return err
		}
	}
	return nil
}

// SSHManagementPort is never a BAFT transport, route or metrics endpoint.
const SSHManagementPort = 22

func refuseSSHManagementPort(addr, field string) error {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		if n, convErr := strconv.Atoi(port); convErr != nil || n != SSHManagementPort {
			return nil
		}
		return fmt.Errorf("%s: port %d is reserved for SSH management", field, SSHManagementPort)
	}
	return nil
}

func validateLoopbackListen(addr, field string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if port == "" || port == "0" {
		return fmt.Errorf("%s: explicit non-zero port required", field)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s must use an explicit loopback IP", field)
	}
	return nil
}
// validatePublicListen accepts any literal IP, including 0.0.0.0 and ::, with
// an explicit port; names are refused so the bind address is never resolved.
func validatePublicListen(addr, field string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if port == "" || port == "0" {
		return fmt.Errorf("%s: explicit non-zero port required", field)
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("%s must use an IP literal", field)
	}
	return nil
}
func validateFixedTarget(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	if port == "" || port == "0" {
		return errors.New("target requires explicit non-zero port")
	}
	if strings.ContainsAny(host, "*?") {
		return errors.New("target wildcard is forbidden")
	}
	if net.ParseIP(host) == nil {
		return errors.New("baseline target must be a fixed IP address")
	}
	return nil
}


func validateTelemetry(t Telemetry) error {
	if !t.Enabled { return nil }
	if t.BCCURL=="" { return errors.New("telemetry.bcc_url is required when enabled") }
	u,err:=url.Parse(t.BCCURL);if err!=nil||u.Host==""{return errors.New("telemetry.bcc_url must be an absolute URL")}
	if u.RawQuery!=""||u.Fragment!=""||u.User!=nil{return errors.New("telemetry.bcc_url must not contain credentials, query, or fragment")}
	if u.Path!=""&&u.Path!="/"{return errors.New("telemetry.bcc_url must not contain an API path")}
	if u.Scheme!="https"{
		host:=u.Hostname()
		ip:=net.ParseIP(host)
		if u.Scheme!="http"||!(host=="localhost"||(ip!=nil&&ip.IsLoopback())) {
			return errors.New("telemetry.bcc_url must use https except on loopback")
		}
	}
	if t.IntervalSeconds<0||t.IntervalSeconds>3600{return errors.New("telemetry.interval_seconds is out of range")}
	if t.RouteProbeIntervalSeconds<0||t.RouteProbeIntervalSeconds>60{return errors.New("telemetry.route_probe_interval_seconds is out of range")}
	if t.RouteProbeIntervalSeconds>0&&t.RouteProbeIntervalSeconds<1{return errors.New("telemetry.route_probe_interval_seconds is too small")}
	if strings.ContainsAny(t.AgentTokenEnv,"=\x00"){return errors.New("telemetry.agent_token_env is invalid")}
	if strings.ContainsRune(t.SpoolPath,'\x00'){return errors.New("telemetry.spool_path is invalid")}
	if t.SpoolMaxPending<0||t.SpoolMaxPending>1_000_000{return errors.New("telemetry.spool_max_pending is out of range")}
	return nil
}
