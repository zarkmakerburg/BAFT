package bcc

import (
	"fmt"
	"sort"
	"time"
)

// AutoTransportCandidate is immutable evidence assembled by BCC from a fresh
// ordered PathMatrix cell, authenticated node capability reports, and current
// port ownership. It is a planning input only; selection never reserves or
// mutates a host resource.
type AutoTransportCandidate struct {
	SourceNode                 string    `json:"source_node"`
	DestinationNode            string    `json:"destination_node"`
	Family                     string    `json:"family"`
	Port                       int       `json:"port"`
	Class                      string    `json:"class"`
	State                      string    `json:"state"`
	EvidenceAt                 time.Time `json:"evidence_at"`
	ConnectRTTMS               int64     `json:"connect_rtt_ms"`
	TricklePassed              bool      `json:"trickle_passed"`
	CarrierID                  string    `json:"carrier_id"`
	InstalledAtSource          bool      `json:"installed_at_source"`
	InstalledAtDestination     bool      `json:"installed_at_destination"`
	CapabilityEvidenceVerified bool      `json:"capability_evidence_verified"`
	NegotiatedCapabilities     []string  `json:"negotiated_capabilities,omitempty"`
	ListenerAvailable          bool      `json:"listener_available"`
	SharedListener             bool      `json:"shared_listener"`
	MultiplexVerified          bool      `json:"multiplex_verified"`
	UTLSSupported              bool      `json:"utls_supported"`
}

type AutoTransportRequest struct {
	SourceNode           string
	DestinationNode      string
	Family               string
	RequestedCarrier     string // empty means Auto; a non-empty value is a hard constraint
	RequiredCapabilities []string
	RequireUTLS          bool
	Now                  time.Time
	FreshFor             time.Duration
}

type AutoTransportRejection struct {
	CarrierID string `json:"carrier_id"`
	Family    string `json:"family"`
	Port      int    `json:"port"`
	Reason    string `json:"reason"`
}

type AutoTransportSelection struct {
	Selected *AutoTransportCandidate  `json:"selected,omitempty"`
	Rejected []AutoTransportRejection `json:"rejected,omitempty"`
	Reason   string                   `json:"reason"`
}

func hasCapability(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// SelectAutoTransport ranks only fully-probed, fresh, capability-verified
// candidates. It refuses rather than downgrading to a weaker probe class,
// different explicit carrier, unverified shared listener, or missing feature.
func SelectAutoTransport(req AutoTransportRequest, candidates []AutoTransportCandidate) AutoTransportSelection {
	out := AutoTransportSelection{}
	if req.SourceNode == "" || req.DestinationNode == "" || req.SourceNode == req.DestinationNode {
		out.Reason = "source and destination must be distinct enrolled nodes"
		return out
	}
	if req.Family != "4" && req.Family != "6" {
		out.Reason = "address family must be 4 or 6"
		return out
	}
	if req.Now.IsZero() {
		out.Reason = "selection time is required"
		return out
	}
	freshFor := req.FreshFor
	if freshFor <= 0 {
		freshFor = pathDiscoveryFreshFor
	}

	eligible := make([]AutoTransportCandidate, 0, len(candidates))
	reject := func(c AutoTransportCandidate, reason string) {
		out.Rejected = append(out.Rejected, AutoTransportRejection{
			CarrierID: c.CarrierID, Family: c.Family, Port: c.Port, Reason: reason,
		})
	}
	for _, c := range candidates {
		switch {
		case c.SourceNode != req.SourceNode || c.DestinationNode != req.DestinationNode:
			reject(c, "path evidence is for a different ordered node pair")
		case c.Family != req.Family:
			reject(c, "path evidence uses a different address family")
		case c.Port < 1 || c.Port > 65535:
			reject(c, "candidate transport port is invalid")
		case c.ConnectRTTMS < 0:
			reject(c, "measured path RTT cannot be negative")
		case req.RequestedCarrier != "" && c.CarrierID != req.RequestedCarrier:
			reject(c, "candidate does not match the explicitly selected carrier")
		case c.Class != PathFullData || c.State != PathMatrixPass:
			reject(c, "fresh FULL_DATA path evidence is required; TCP connect or handshake alone is insufficient")
		case !c.TricklePassed:
			reject(c, "path trickle test did not pass")
		case c.EvidenceAt.IsZero() || c.EvidenceAt.After(req.Now) || req.Now.Sub(c.EvidenceAt) > freshFor:
			reject(c, "path evidence is missing, future-dated, or stale")
		case c.CarrierID == "":
			reject(c, "carrier identity is missing")
		case !c.InstalledAtSource || !c.InstalledAtDestination:
			reject(c, "carrier is not verified as installed on both nodes")
		case !c.CapabilityEvidenceVerified:
			reject(c, "authenticated negotiated capability evidence is missing")
		case req.RequireUTLS && !c.UTLSSupported:
			reject(c, "uTLS was required but is not supported by the selected carrier")
		case !c.ListenerAvailable && !(c.SharedListener && c.MultiplexVerified):
			reject(c, "listener port is occupied or unavailable and verified multiplex reuse is not enabled")
		default:
			missing := ""
			for _, required := range req.RequiredCapabilities {
				if !hasCapability(c.NegotiatedCapabilities, required) {
					missing = required
					break
				}
			}
			if missing != "" {
				reject(c, fmt.Sprintf("required capability %q is not negotiated on both nodes", missing))
				continue
			}
			eligible = append(eligible, c)
		}
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if a.ConnectRTTMS != b.ConnectRTTMS {
			return a.ConnectRTTMS < b.ConnectRTTMS
		}
		if a.CarrierID != b.CarrierID {
			return a.CarrierID < b.CarrierID
		}
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		return a.Port < b.Port
	})
	sort.SliceStable(out.Rejected, func(i, j int) bool {
		a, b := out.Rejected[i], out.Rejected[j]
		if a.CarrierID != b.CarrierID {
			return a.CarrierID < b.CarrierID
		}
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.Reason < b.Reason
	})
	if len(eligible) == 0 {
		out.Reason = "no secure compatible candidate satisfies all path, capability, and ownership constraints"
		return out
	}
	selected := eligible[0]
	out.Selected = &selected
	return out
}

// ResolveAutoPublicClientPort keeps the public client port separate from the
// EX transport listener. In Auto mode it defaults to the EX service target;
// an explicit mismatch is returned to the UI so it can require acknowledgement
// about external 3X-UI subscription rewriting.
func ResolveAutoPublicClientPort(targetPort, explicitPort int) (port int, differsFromTarget bool, err error) {
	if targetPort < 1 || targetPort > 65535 {
		return 0, false, fmt.Errorf("target service port must be between 1 and 65535")
	}
	if explicitPort == 0 {
		return targetPort, false, nil
	}
	if explicitPort < 1 || explicitPort > 65535 {
		return 0, false, fmt.Errorf("public client port must be between 1 and 65535")
	}
	return explicitPort, explicitPort != targetPort, nil
}
