package agent

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

const maxIngressSocketTable = 8 << 20

// publicIngressPreflightEvidence records a snapshot of the agent's network
// namespace. Absence from /proc is not a bind reservation or reachability proof.
// Ingress ownership cannot be inferred from a BAFT transport's ownership marker.
type publicIngressPreflightEvidence struct {
	SchemaVersion           int      `json:"schema_version"`
	TunnelID                string   `json:"tunnel_id"`
	PublicPort              int      `json:"public_port"`
	LocalTarget             string   `json:"local_target"`
	Scope                   string   `json:"scope"`
	SocketInventoryComplete bool     `json:"socket_inventory_complete"`
	PublicPortState         string   `json:"public_port_state"`
	PublicListenerCount     int      `json:"public_listener_count"`
	TargetListenerObserved  bool     `json:"target_listener_observed"`
	OwnershipVerified       bool     `json:"ownership_verified"`
	FirewallTools           []string `json:"firewall_tools"`
	FirewallPolicyVerified  bool     `json:"firewall_policy_verified"`
	ReadyToApply            bool     `json:"ready_to_apply"`
	Limitations             []string `json:"limitations"`
}

func readIngressSocketTable(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxIngressSocketTable+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxIngressSocketTable {
		return nil, fmt.Errorf("socket table exceeds inspection limit")
	}
	return b, nil
}

type ingressSocket struct {
	ip   net.IP
	port int
}

func parseIngressSocketTable(raw []byte, ipv6 bool) ([]ingressSocket, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("truncated socket table")
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "local_address") {
		return nil, fmt.Errorf("invalid socket table header")
	}
	if len(lines) > 65538 {
		return nil, fmt.Errorf("socket table row limit exceeded")
	}
	var sockets []ingressSocket
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("invalid socket table row")
		}
		state, err := strconv.ParseUint(fields[3], 16, 8)
		if err != nil || len(fields[3]) != 2 || state < 1 || state > 12 {
			return nil, fmt.Errorf("invalid socket state")
		}
		if state != 10 {
			continue
		}
		endpoint := strings.Split(fields[1], ":")
		if len(endpoint) != 2 {
			return nil, fmt.Errorf("invalid listener endpoint")
		}
		data, err := hex.DecodeString(endpoint[0])
		size := 4
		if ipv6 {
			size = 16
		}
		if err != nil || len(data) != size {
			return nil, fmt.Errorf("invalid listener address")
		}
		// Linux procfs emits each address word in native byte order.
		for i := 0; i < len(data); i += 4 {
			word := binary.NativeEndian.Uint32(data[i : i+4])
			binary.BigEndian.PutUint32(data[i:i+4], word)
		}
		port, err := strconv.ParseUint(endpoint[1], 16, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("invalid listener port")
		}
		sockets = append(sockets, ingressSocket{ip: net.IP(data), port: int(port)})
	}
	return sockets, nil
}

func collectPublicIngressPreflight(ctx context.Context, j agentjob.Job, readTable func(string) ([]byte, error), lookPath func(string) (string, error)) (publicIngressPreflightEvidence, error) {
	if j.Action != agentjob.ActionPublicIngressPreflight {
		return publicIngressPreflightEvidence{}, fmt.Errorf("not a public ingress preflight action")
	}
	// Defense in depth for direct callers; production dispatch follows signed
	// verification. This enforces the existing exact parameter contract too.
	if err := j.Validate(); err != nil {
		return publicIngressPreflightEvidence{}, err
	}
	host, portText, _ := net.SplitHostPort(j.Params["local_target"])
	targetIP := net.ParseIP(host)
	targetPort, _ := strconv.Atoi(portText)
	publicPort, _ := strconv.Atoi(j.Params["public_port"])
	ev := publicIngressPreflightEvidence{SchemaVersion: 1, TunnelID: j.Params["tunnel_id"], PublicPort: publicPort, LocalTarget: j.Params["local_target"], Scope: "agent_network_namespace_snapshot", SocketInventoryComplete: true, PublicPortState: "unknown", FirewallTools: []string{}, Limitations: []string{
		"Socket observations do not reserve a port or prove end-to-end delivery.",
		"Public ingress ownership and firewall policy are not verified.",
		"Public ingress prepare, commit, reconcile and rollback are not implemented; application is blocked.",
	}}
	for _, table := range []struct {
		path string
		ipv6 bool
	}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
		if err := ctx.Err(); err != nil {
			return publicIngressPreflightEvidence{}, err
		}
		raw, err := readTable(table.path)
		var sockets []ingressSocket
		if err == nil {
			sockets, err = parseIngressSocketTable(raw, table.ipv6)
		}
		if err != nil {
			ev.SocketInventoryComplete = false
			// Do not include arbitrary file contents, host paths or error output.
			ev.Limitations = append(ev.Limitations, "Socket inventory unavailable or invalid: "+table.path)
			continue
		}
		for _, socket := range sockets {
			if socket.port == publicPort {
				ev.PublicListenerCount++
			}
			sameFamily := (socket.ip.To4() != nil) == (targetIP.To4() != nil)
			if socket.port == targetPort && sameFamily && (socket.ip.Equal(targetIP) || socket.ip.IsUnspecified()) {
				ev.TargetListenerObserved = true
			}
		}
	}
	if ev.PublicListenerCount > 0 {
		ev.PublicPortState = "occupied"
	} else if ev.SocketInventoryComplete {
		ev.PublicPortState = "not_observed"
	}
	for _, tool := range []string{"nft", "iptables", "ip6tables", "ufw", "firewall-cmd"} {
		if err := ctx.Err(); err != nil {
			return publicIngressPreflightEvidence{}, err
		}
		if _, err := lookPath(tool); err == nil {
			ev.FirewallTools = append(ev.FirewallTools, tool)
		}
	}
	return ev, nil
}

func (a *Agent) publicIngressPreflight(ctx context.Context, j agentjob.Job) (string, error) {
	ev, err := collectPublicIngressPreflight(ctx, j, readIngressSocketTable, exec.LookPath)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(ev)
	return string(b), err
}
