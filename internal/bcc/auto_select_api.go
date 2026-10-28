package bcc

import (
	"errors"
	"net/http"
	"slices"
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
	now = now.UTC()
	if req.Carrier != "" {
		if _, ok := autoSelectCarriers[req.Carrier]; !ok {
			return AutoTransportSelection{}, errors.New(`carrier must be empty (Auto), "h2" or "ws"`)
		}
	}
	matrix := s.PathMatrix(now)

	s.mu.Lock()
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
	s.mu.Unlock()

	var negotiated []string
	for _, c := range irCaps {
		if slices.Contains(exCaps, c) {
			negotiated = append(negotiated, c)
		}
	}
	var cell *PathMatrixCell
	for i := range matrix.Cells {
		if matrix.Cells[i].SourceNode == req.IRNode && matrix.Cells[i].DestinationNode == req.EXNode {
			cell = &matrix.Cells[i]
			break
		}
	}
	carriers := []string{"h2", "ws"}
	if req.Carrier != "" {
		carriers = []string{req.Carrier}
	}
	var candidates []AutoTransportCandidate
	if cell != nil {
		for _, pc := range cell.Candidates {
			for _, carrier := range carriers {
				capName := autoSelectCarriers[carrier]
				candidates = append(candidates, AutoTransportCandidate{
					SourceNode: req.IRNode, DestinationNode: req.EXNode,
					Family: pc.Family, Port: pc.Port, Class: pc.Class, State: pc.State,
					EvidenceAt: pc.EvidenceAt, ConnectRTTMS: pc.ConnectRTTMS, TricklePassed: pc.TricklePassed,
					CarrierID:                  carrier,
					InstalledAtSource:          slices.Contains(irCaps, capName),
					InstalledAtDestination:     slices.Contains(exCaps, capName),
					CapabilityEvidenceVerified: exFresh && irFresh,
					NegotiatedCapabilities:     negotiated,
					// The probe proved the port was bindable on EX when it ran;
					// a port another tunnel record holds is not available.
					ListenerAvailable: !used[pc.Port],
				})
			}
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
	if (cell == nil || len(cell.Candidates) == 0) && sel.Selected == nil {
		sel.Reason = "no path evidence for this IR -> EX pair; run path discovery first"
	}
	return sel, nil
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
