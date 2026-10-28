package bcc

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
)

// autoSelectCarriers are the carriers the native engine can run, by the
// carrier capability that proves an installed baft binary implements them.
var autoSelectCarriers = map[string]string{"h2": "carrier.h2", "ws": "carrier.ws"}

// AutoSelectRequest asks BCC to rank EX transport ports for one IR -> EX
// tunnel. It is read-only: no probe, reservation, job or host change.
type AutoSelectRequest struct {
	EXNode  string `json:"ex_node"`
	IRNode  string `json:"ir_node"`
	Family  string `json:"family"`            // "4" or "6"
	Carrier string `json:"carrier,omitempty"` // empty = Auto; "h2" or "ws" is a hard constraint
}

// nodeTransportCapabilities returns the dotted protocol capabilities a node's
// agent declared on an authenticated poll no older than ingressCapabilityMaxAge,
// and whether that evidence is usable at all.
func nodeTransportCapabilities(n Node, ok bool, now time.Time) ([]string, bool) {
	if !ok || n.Revoked || n.AgentTokenHash == "" || n.AgentSeen.IsZero() ||
		n.AgentSeen.After(now.Add(time.Minute)) || now.Sub(n.AgentSeen) > ingressCapabilityMaxAge {
		return nil, false
	}
	var out []string
	for _, c := range n.Capabilities {
		if dotted := strings.Replace(c, "-", ".", 1); dotted != c && slices.Contains([]string{"carrier.h2", "carrier.ws", "noise.v1", "shape.v1"}, dotted) {
			out = append(out, dotted)
		}
	}
	return out, true
}

// AutoSelectTransport builds candidates only from BCC's own evidence (the
// IR -> EX path-matrix cell, fresh agent capability reports, and the tunnel
// records that occupy EX ports) and lets SelectAutoTransport refuse anything
// short of fresh FULL_DATA on a carrier both installed binaries implement.
func (s *Store) AutoSelectTransport(req AutoSelectRequest, now time.Time) (AutoTransportSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.autoSelectTransportLocked(req, now)
}

// autoSelectTransportLocked assembles the complete candidate snapshot under
// Store.mu. Review/deploy validation can call this while holding the same lock
// used to build the plan, avoiding mixed path/node/port snapshots.
func (s *Store) autoSelectTransportLocked(req AutoSelectRequest, now time.Time) (AutoTransportSelection, error) {
	now = now.UTC()
	if req.Carrier != "" {
		if _, ok := autoSelectCarriers[req.Carrier]; !ok {
			return AutoTransportSelection{}, errors.New(`carrier must be empty (Auto), "h2" or "ws"`)
		}
	}

	ex, exOK := s.st.Nodes[req.EXNode]
	ir, irOK := s.st.Nodes[req.IRNode]
	exCaps, exFresh := nodeTransportCapabilities(ex, exOK, now)
	irCaps, irFresh := nodeTransportCapabilities(ir, irOK, now)
	used := map[int]bool{}
	for _, t := range s.st.Tunnels {
		if t.EXNode == req.EXNode && (t.Phase == TunnelActive || !terminalTunnel(t.Phase) || t.Phase == TunnelRollbackFailed || t.Phase == TunnelDecommissionFailed) {
			used[t.Port] = true
		}
	}

	var negotiated []string
	for _, c := range irCaps {
		if slices.Contains(exCaps, c) {
			negotiated = append(negotiated, c)
		}
	}

	// Mirror the path-matrix eligibility and ordering directly from the same
	// locked Store snapshot. Only the ordered IR -> EX pair is a candidate.
	var paths []PathMatrixCandidate
	if exOK && irOK && !ex.Revoked && !ir.Revoked && req.EXNode != req.IRNode {
		for _, p := range s.st.PathProbes {
			if p.Phase != PathProbeComplete || p.SourceNode != req.IRNode || p.DestinationNode != req.EXNode || p.SourceNode == p.DestinationNode {
				continue
			}
			for _, candidate := range p.Candidates {
				if candidate.Class == "" {
					continue
				}
				age := now.Sub(p.UpdatedAt)
				if age < 0 {
					age = 0
				}
				state := matrixState(candidate.Class)
				if age > pathDiscoveryFreshFor {
					state = PathMatrixStale
				}
				paths = append(paths, PathMatrixCandidate{
					ProbeID: p.ID, Family: p.Family, Port: candidate.Port,
					Class: candidate.Class, State: state, ConnectRTTMS: candidate.ConnectRTTMS,
					ByteCeiling: candidate.ByteCeiling, TricklePassed: candidate.TricklePassed,
					Detail: candidate.Detail, EvidenceAt: p.UpdatedAt, AgeSeconds: int64(age / time.Second),
				})
			}
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := paths[i], paths[j]
		if matrixStateRank(a.State) != matrixStateRank(b.State) {
			return matrixStateRank(a.State) > matrixStateRank(b.State)
		}
		if !a.EvidenceAt.Equal(b.EvidenceAt) {
			return a.EvidenceAt.After(b.EvidenceAt)
		}
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.ProbeID < b.ProbeID
	})

	carriers := []string{"h2", "ws"}
	if req.Carrier != "" {
		carriers = []string{req.Carrier}
	}
	var candidates []AutoTransportCandidate
	for _, pc := range paths {
		for _, carrier := range carriers {
			capName := autoSelectCarriers[carrier]
			candidates = append(candidates, AutoTransportCandidate{
				SourceNode: req.IRNode, DestinationNode: req.EXNode,
				ProbeID: pc.ProbeID, CapabilityEvidenceAtSource: ir.AgentSeen, CapabilityEvidenceAtDestination: ex.AgentSeen,
				Family: pc.Family, Port: pc.Port, Class: pc.Class, State: pc.State,
				EvidenceAt: pc.EvidenceAt, ConnectRTTMS: pc.ConnectRTTMS, TricklePassed: pc.TricklePassed,
				CarrierID:                  carrier,
				InstalledAtSource:          slices.Contains(irCaps, capName),
				InstalledAtDestination:     slices.Contains(exCaps, capName),
				CapabilityEvidenceVerified: exFresh && irFresh,
				NegotiatedCapabilities:     negotiated,
				// The probe proved the port was bindable on EX when it ran; a port
				// another tunnel record holds is not currently available.
				ListenerAvailable: !used[pc.Port],
			})
		}
	}
	required := []string{}
	if req.Carrier == "ws" {
		// WebSocket is only offered with Noise; never as a weaker carrier.
		required = append(required, "noise.v1")
	}
	sel := SelectAutoTransport(AutoTransportRequest{
		SourceNode: req.IRNode, DestinationNode: req.EXNode, Family: req.Family,
		RequestedCarrier: req.Carrier, RequiredCapabilities: required, Now: now,
	}, candidates)
	// ws without Noise must never be selected in Auto mode either.
	if sel.Selected != nil && sel.Selected.CarrierID == "ws" && !slices.Contains(sel.Selected.NegotiatedCapabilities, "noise.v1") {
		sel.Rejected = append(sel.Rejected, AutoTransportRejection{CarrierID: "ws", Family: sel.Selected.Family, Port: sel.Selected.Port, Reason: "WebSocket requires noise.v1 on both nodes"})
		sel.Selected, sel.Reason = nil, "no secure compatible candidate satisfies all path, capability, and ownership constraints"
	}
	if sel.Selected == nil && sel.Reason == "" {
		sel.Reason = "no secure compatible candidate satisfies all path, capability, and ownership constraints"
	}
	if len(paths) == 0 && sel.Selected == nil {
		sel.Reason = "no path evidence for this IR -> EX pair; run path discovery first"
	}
	return sel, nil
}

// sameAutoTransportCandidate compares every field that can affect whether a
// recommendation is valid. It intentionally includes evidence time, measured
// RTT, negotiated capabilities, and current listener ownership.
func sameAutoTransportCandidate(a, b AutoTransportCandidate) bool {
	return a.SourceNode == b.SourceNode &&
		a.DestinationNode == b.DestinationNode &&
		a.ProbeID == b.ProbeID &&
		a.CapabilityEvidenceAtSource.Equal(b.CapabilityEvidenceAtSource) &&
		a.CapabilityEvidenceAtDestination.Equal(b.CapabilityEvidenceAtDestination) &&
		a.Family == b.Family &&
		a.Port == b.Port &&
		a.Class == b.Class &&
		a.State == b.State &&
		a.EvidenceAt.Equal(b.EvidenceAt) &&
		a.ConnectRTTMS == b.ConnectRTTMS &&
		a.TricklePassed == b.TricklePassed &&
		a.CarrierID == b.CarrierID &&
		a.InstalledAtSource == b.InstalledAtSource &&
		a.InstalledAtDestination == b.InstalledAtDestination &&
		a.CapabilityEvidenceVerified == b.CapabilityEvidenceVerified &&
		slices.Equal(a.NegotiatedCapabilities, b.NegotiatedCapabilities) &&
		a.ListenerAvailable == b.ListenerAvailable &&
		a.SharedListener == b.SharedListener &&
		a.MultiplexVerified == b.MultiplexVerified &&
		a.UTLSSupported == b.UTLSSupported
}

// validateAutoSelectEvidence re-reads live path evidence, node capabilities,
// and port ownership. Client-supplied evidence is a binding only, never
// authority by itself. BuildPlan calls this for review and again at deploy.
func (s *Store) validateAutoSelectEvidence(req TunnelRequest, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validateAutoSelectEvidenceLocked(req, now)
}

func (s *Store) validateAutoSelectEvidenceLocked(req TunnelRequest, now time.Time) error {
	evidence := req.AutoSelectEvidence
	if evidence == nil {
		return nil
	}
	if evidence.SourceNode != req.IRNode || evidence.DestinationNode != req.EXNode ||
		evidence.Port != req.Port || evidence.CarrierID == "" {
		return errors.New("auto-select evidence does not match the requested IR -> EX path, carrier, and port")
	}
	selection, err := s.autoSelectTransportLocked(AutoSelectRequest{
		EXNode: req.EXNode, IRNode: req.IRNode,
		Family: evidence.Family, Carrier: evidence.CarrierID,
	}, now)
	if err != nil {
		return err
	}
	if selection.Selected == nil || !sameAutoTransportCandidate(*evidence, *selection.Selected) {
		return errors.New("auto-select evidence changed or expired; select a fresh transport and review the plan again")
	}
	return nil
}

func (s *Server) tunnelAutoSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	var in AutoSelectRequest
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sel, err := s.store.AutoSelectTransport(in, s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, sel)
}
