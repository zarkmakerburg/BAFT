package bcc

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"time"
)

// PublicIngressReservation is desired BCC bookkeeping, never proof of an OS
// listener, firewall permission or agent ownership. Entries are immutable in
// this phase. Release/edit needs a later generation-aware recovery protocol.
type PublicIngressReservation struct {
	Spec      PublicIngressSpec `json:"spec"`
	CreatedAt time.Time         `json:"created_at"`
}

func (s *Store) publicIngressReservationSpecsLocked() ([]PublicIngressSpec, error) {
	specs := make([]PublicIngressSpec, 0, len(s.st.PublicIngressReservations))
	for id, reservation := range s.st.PublicIngressReservations {
		// A tunnel whose ingress was retired (Agent retire receipt recorded)
		// no longer holds its port in BCC's desired state; the Agent's own
		// monotonic release fence still decides whether the port is reusable.
		if op, ok := s.st.PublicIngressOperations[id]; ok && ingressOperationRetired(op) {
			continue
		}
		spec, err := PlanPublicIngress(reservation.Spec.IRNode, reservation.Spec.TunnelID, reservation.Spec.PublicPort, reservation.Spec.LocalTarget)
		if err != nil {
			return nil, fmt.Errorf("invalid stored public ingress reservation: %w", err)
		}
		if spec != reservation.Spec || id != spec.TunnelID || reservation.CreatedAt.IsZero() {
			return nil, errors.New("invalid stored public ingress reservation")
		}
		specs = append(specs, spec)
	}
	// Validate the entire canonical ownership set once. The validator already
	// checks every stored record and all cross-record conflicts.
	if len(specs) > 0 {
		if err := ValidatePublicIngressReservation(specs[0], specs); err != nil {
			return nil, err
		}
	}
	for _, spec := range specs {
		if err := s.validateIngressReservationResourcesLocked(spec); err != nil {
			return nil, err
		}
	}
	return specs, nil
}

func ingressEndpointPort(endpoint string) (int, error) {
	_, raw, err := net.SplitHostPort(endpoint)
	if err != nil {
		return 0, errors.New("invalid stored listener endpoint")
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("invalid stored listener port")
	}
	return port, nil
}

// tunnelPortsOnNode includes loopback listeners because a public wildcard
// bind cannot safely share their port. No multiplexing capability is assumed.
func tunnelPortsOnNode(t Tunnel, node string) ([]int, error) {
	ports := []int{}
	endpoints := []string{}
	if t.EXNode == node {
		if t.Port < 1 || t.Port > 65535 {
			return nil, errors.New("invalid stored transport port")
		}
		ports = append(ports, t.Port)
		metrics := t.EXMetricsListen
		if metrics == "" {
			metrics = "127.0.0.1:9191"
		}
		endpoints = append(endpoints, metrics)
	}
	if t.IRNode == node {
		metrics := t.IRMetricsListen
		if metrics == "" {
			metrics = "127.0.0.1:9191"
		}
		endpoints = append(endpoints, t.RouteListen, metrics)
	}
	for _, endpoint := range endpoints {
		port, err := ingressEndpointPort(endpoint)
		if err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func tunnelKeepsPorts(t Tunnel) bool {
	return t.Phase == TunnelActive || !terminalTunnel(t.Phase) || t.Phase == TunnelRollbackFailed || t.Phase == TunnelDecommissionFailed
}

// Called by the shared tunnel planner/creator, before any job is enqueued.
func (s *Store) validateTunnelPublicIngressReservationsLocked(t Tunnel) error {
	specs, err := s.publicIngressReservationSpecsLocked()
	if err != nil {
		return err
	}
	for _, spec := range specs {
		ports, err := tunnelPortsOnNode(t, spec.IRNode)
		if err != nil {
			return err
		}
		for _, port := range ports {
			if port == spec.PublicPort {
				return fmt.Errorf("port %d on node %s is reserved for public ingress of tunnel %s", port, spec.IRNode, spec.TunnelID)
			}
		}
	}
	return nil
}

func (s *Store) ReservePublicIngress(spec PublicIngressSpec, now time.Time, audit AuditEntry) (PublicIngressReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservePublicIngressLocked(spec, now, audit)
}

// ReserveReviewedPublicIngress is the BCC-side durable desired-only seam.
// It requires the actual persisted active tunnel, unchanged reviewed hash and
// the same canonical client/config port. The check and reservation commit
// occur under one Store lock. This is NOT an Agent apply or lease proof.
func (s *Store) ReserveReviewedPublicIngress(spec PublicIngressSpec, planHash string, now time.Time, audit AuditEntry) (PublicIngressReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.Tunnels[spec.TunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash == "" || t.PlanHash != planHash || t.ClientPort == 0 || t.ClientPort != spec.PublicPort || t.IRNode != spec.IRNode || t.RouteListen != spec.LocalTarget {
		return PublicIngressReservation{}, errors.New("reviewed tunnel identity, active status, hash or client port mismatch")
	}
	return s.reservePublicIngressLocked(spec, now, audit)
}

func (s *Store) reservePublicIngressLocked(spec PublicIngressSpec, now time.Time, audit AuditEntry) (PublicIngressReservation, error) {
	spec, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, spec.LocalTarget)
	if err != nil {
		return PublicIngressReservation{}, err
	}
	if now.IsZero() {
		return PublicIngressReservation{}, errors.New("reservation timestamp is required")
	}
	// Protect SSH management before consulting stored resource ownership.
	// Other ports still require live host checks before any future agent apply.
	if spec.PublicPort == 22 {
		return PublicIngressReservation{}, errors.New("port 22 is reserved for SSH management")
	}
	n, ok := s.st.Nodes[spec.IRNode]
	if !ok || n.Revoked || n.AgentTokenHash == "" || (n.Role != "worker" && n.Role != "master") {
		return PublicIngressReservation{}, errors.New("reservation requires an active enrolled IR node")
	}
	t, ok := s.st.Tunnels[spec.TunnelID]
	if !ok || t.IRNode != spec.IRNode {
		return PublicIngressReservation{}, errors.New("tunnel does not belong to the requested IR node")
	}
	expected, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, t.RouteListen)
	if err != nil || expected.LocalTarget != spec.LocalTarget {
		return PublicIngressReservation{}, errors.New("reservation target must match the tunnel IR route")
	}
	specs, err := s.publicIngressReservationSpecsLocked()
	if err != nil {
		return PublicIngressReservation{}, err
	}
	if err := ValidatePublicIngressReservation(spec, specs); err != nil {
		return PublicIngressReservation{}, err
	}
	if err := s.validateIngressReservationResourcesLocked(spec); err != nil {
		return PublicIngressReservation{}, err
	}
	if old, ok := s.st.PublicIngressReservations[spec.TunnelID]; ok {
		return old, nil
	}
	before, err := cloneState(s.st)
	if err != nil {
		return PublicIngressReservation{}, err
	}
	if s.st.PublicIngressReservations == nil {
		s.st.PublicIngressReservations = map[string]PublicIngressReservation{}
	}
	reservation := PublicIngressReservation{Spec: spec, CreatedAt: now.UTC()}
	s.st.PublicIngressReservations[spec.TunnelID] = reservation
	audit.Action, audit.Target, audit.Outcome = "public_ingress.reserve", spec.TunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"ir_node": spec.IRNode, "public_port": spec.PublicPort, "local_target": spec.LocalTarget, "desired_only": true})
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return PublicIngressReservation{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return PublicIngressReservation{}, err
	}
	return reservation, nil
}

func (s *Store) ListPublicIngressReservations() ([]PublicIngressReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.publicIngressReservationSpecsLocked(); err != nil {
		return nil, err
	}
	out := make([]PublicIngressReservation, 0, len(s.st.PublicIngressReservations))
	for _, reservation := range s.st.PublicIngressReservations {
		out = append(out, reservation)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.TunnelID < out[j].Spec.TunnelID })
	return out, nil
}

// Older backups must not free desired ports that may correspond to unknown
// live resources. Preserve current intents and refuse conflicting backup tuples.
func mergePublicIngressReservations(restored *state, current state) error {
	merged := make(map[string]PublicIngressReservation, len(restored.PublicIngressReservations)+len(current.PublicIngressReservations))
	for id, reservation := range restored.PublicIngressReservations {
		merged[id] = reservation
	}
	for id, reservation := range current.PublicIngressReservations {
		if old, ok := merged[id]; ok && old.Spec != reservation.Spec {
			return errors.New("backup changes a current public ingress reservation")
		}
		merged[id] = reservation
	}
	check := Store{st: *restored}
	check.st.PublicIngressReservations = merged
	if _, err := check.publicIngressReservationSpecsLocked(); err != nil {
		return err
	}
	restored.PublicIngressReservations = merged
	return nil
}

// Missing historical owners retain their allocation; present resources must
// still be conflict-free after node updates, migrations or older restores.
func (s *Store) validateIngressReservationResourcesLocked(spec PublicIngressSpec) error {
	if spec.PublicPort == 22 {
		return errors.New("protected SSH management port")
	}
	if node, ok := s.st.Nodes[spec.IRNode]; ok {
		if node.Role != "worker" && node.Role != "master" {
			return errors.New("reserved ingress node must retain IR role")
		}
		port, err := ingressEndpointPort(node.Address)
		if err != nil {
			return errors.New("cannot establish reserved node management port")
		}
		if port == spec.PublicPort {
			return errors.New("reservation conflicts with node management port")
		}
	}
	if owner, ok := s.st.Tunnels[spec.TunnelID]; ok {
		expected, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, owner.RouteListen)
		if err != nil || owner.IRNode != spec.IRNode || expected.LocalTarget != spec.LocalTarget {
			return errors.New("present tunnel owner does not match reserved ingress target")
		}
	}
	for _, other := range s.st.Tunnels {
		if !tunnelKeepsPorts(other) {
			continue
		}
		ports, err := tunnelPortsOnNode(other, spec.IRNode)
		if err != nil {
			return err
		}
		for _, port := range ports {
			if port == spec.PublicPort {
				return errors.New("reservation conflicts with managed tunnel listener")
			}
		}
	}
	for _, migration := range s.st.SSHMigrations {
		if migration.NodeID == spec.IRNode && (migration.OldPort == spec.PublicPort || migration.NewPort == spec.PublicPort) {
			return errors.New("reservation conflicts with recorded SSH migration port")
		}
	}
	for _, probe := range s.st.PathProbes {
		if probe.DestinationNode != spec.IRNode || probe.Phase == PathProbeComplete || probe.Phase == PathProbeFailed || probe.Phase == PathProbeCancelled {
			continue
		}
		for _, candidate := range probe.Candidates {
			if candidate.Port == spec.PublicPort {
				return errors.New("reservation conflicts with pending path probe port")
			}
		}
	}
	return nil
}

func (s *Store) validatePublicIngressPortSetLocked(node string, ports ...int) error {
	specs, err := s.publicIngressReservationSpecsLocked()
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if spec.IRNode == node {
			for _, port := range ports {
				if spec.PublicPort == port {
					return errors.New("port is reserved for public ingress")
				}
			}
		}
	}
	return nil
}

func (s *Store) validateNodePublicIngressReservationsLocked(node Node) error {
	for _, reservation := range s.st.PublicIngressReservations {
		if reservation.Spec.IRNode != node.ID {
			continue
		}
		port, err := ingressEndpointPort(node.Address)
		if err != nil {
			return errors.New("node with reservations requires an explicit management port")
		}
		if node.Role != "worker" && node.Role != "master" {
			return errors.New("node with ingress reservations must retain its IR role")
		}
		if port == reservation.Spec.PublicPort {
			return errors.New("node management port is reserved for public ingress")
		}
	}
	return nil
}
