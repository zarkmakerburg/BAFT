// Package agentjob defines the jobs BCC hands to node agents and how an
// agent decides to run one (Launch-1 P1-D).
//
// BCC signs every job with its job-signing Ed25519 key; an agent pins that
// public key at enrollment. An agent runs a job only if the signature
// verifies against the pinned key, the job names this node, the action is on
// the fixed allowlist with valid parameters, the job is inside its short
// validity window, and its ID has not been seen before. There is no shell or
// free-form command action: a compromised BCC can only ask for allowlisted
// operations, and nobody else can ask for anything.
package agentjob

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

const (
	PayloadType   = "application/vnd.baft.agent-job+json"
	SchemaVersion = 1
	// MaxLifetime bounds how long a signed job stays runnable.
	MaxLifetime = 24 * time.Hour
	// ClockSkew tolerates an agent clock slightly behind BCC's.
	ClockSkew = 5 * time.Minute
	// maxParamLen bounds any one parameter (pairing codes are the largest).
	maxParamLen = 8192
)

// Actions an agent can run. Anything else is refused.
const (
	ActionHealth     = "health"
	ActionRestart    = "restart"
	ActionReload     = "reload"
	ActionUpdateBAFT = "update_baft"

	ActionSSHMigrationStage    = "ssh_migrate_stage"
	ActionSSHMigrationCommit   = "ssh_migrate_commit"
	ActionSSHMigrationRollback = "ssh_migrate_rollback"

	ActionPathProbeInventory = "path_probe_inventory"
	ActionPathProbeListen    = "path_probe_listen"
	ActionPathProbeRun       = "path_probe_run"
	ActionPathProbeStop      = "path_probe_stop"

	// Tunnel changes (P1-E). prepare stages, commit installs, health checks,
	// finalize makes permanent, rollback restores the previous state.
	ActionTunnelPrepareEX = "tunnel_prepare_ex"
	ActionTunnelPrepareIR = "tunnel_prepare_ir"
	ActionTunnelCommitEX  = "tunnel_commit_ex"
	ActionTunnelCommitIR  = "tunnel_commit_ir"
	ActionTunnelHealth    = "tunnel_health"
	ActionTunnelObserve   = "tunnel_observe"
	ActionTunnelInspect   = "tunnel_inspect"
	// ActionTunnelDiscover is a read-only inventory of the BAFT units on a node.
	ActionTunnelDiscover = "tunnel_discover"
	ActionTunnelFinalize = "tunnel_finalize"
	ActionTunnelRollback = "tunnel_rollback"
	// ActionTunnelRetire removes one finalized BAFT-owned instance only when
	// BCC-provided generation and live file digests still match.
	ActionTunnelRetire = "tunnel_retire"
	ActionTunnelTenantAdd    = "tunnel_tenant_add"
	ActionTunnelTenantRemove = "tunnel_tenant_remove"

	// P0 host observation is read-only. Prepare writes only private manifest
	// bytes; listener/firewall application needs a separate owned-resource path.
	ActionPublicIngressPreflight = "public_ingress_preflight"
	// Read-only bounded host snapshot; this does not establish resource ownership.
	ActionPublicIngressInspect = "public_ingress_inspect"
	// Writes only a private prepared manifest; it does not bind or open firewall.
	ActionPublicIngressPrepare = "public_ingress_prepare"
	// Fences generation 1 in the private manifest store; no host listener or firewall changes.
	ActionPublicIngressCancel = "public_ingress_cancel"
	ActionPublicIngressApply  = "public_ingress_apply"
	ActionPublicIngressRetire = "public_ingress_retire"

	// Certificate rotation of a built tunnel (A4 Stage F). Only public
	// certificates and digests travel in parameters; new private keys never
	// leave the EX.
	ActionCertPrepareEX  = "tunnel_cert_prepare_ex"
	ActionCertTrustIR    = "tunnel_cert_trust_ir"
	ActionCertVerifyEX   = "tunnel_cert_verify_ex"
	ActionCertVerifyIR   = "tunnel_cert_verify_ir"
	ActionCertActivateEX = "tunnel_cert_activate_ex"
	ActionCertConfirmIR  = "tunnel_cert_confirm_ir"
	ActionCertRetireIR   = "tunnel_cert_retire_ir"
	ActionCertRetireEX   = "tunnel_cert_retire_ex"
	ActionCertRollback   = "tunnel_cert_rollback"
)

var (
	idRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	versionRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

	tunnelIDRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	instanceIDRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)
	addressRe       = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.:-]{0,251}[A-Za-z0-9])?$`)
	portRe          = regexp.MustCompile(`^[0-9]{1,5}$`)
	fixedTargetR    = regexp.MustCompile(`^(([0-9]{1,3}\.){3}[0-9]{1,3}|\[[0-9A-Fa-f:]+\]):[0-9]{1,5}$`)
	loopbackListenR = regexp.MustCompile(`^(127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|\[::1\]):[0-9]{1,5}$`)
	boolRe          = regexp.MustCompile(`^(true|false)$`)
	pairCodeRe      = regexp.MustCompile(`^BAFTPAIR1:[A-Za-z0-9_-]{16,}$`)
	replyCodeRe     = regexp.MustCompile(`^BAFTREPLY1:[A-Za-z0-9_-]{16,}$`)
	epochRe         = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	sha256Re        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	noisePublicKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	derB64Re        = regexp.MustCompile(`^[A-Za-z0-9_-]{64,}$`)
	familyRe        = regexp.MustCompile(`^[46]$`)
	probeSecondsRe  = regexp.MustCompile(`^(1[0-9]|[2-9][0-9]|1[01][0-9]|120)$`)
	probeAttemptsRe = regexp.MustCompile(`^[1-5]$`)
	probeBytesRe    = regexp.MustCompile(`^[1-9][0-9]{3,6}$`)
	probeDelayRe    = regexp.MustCompile(`^[1-9][0-9]{1,3}$`)
	endpointRe      = regexp.MustCompile(`^[0-9A-Fa-f:.\[\]]{3,255}$`)
	listenEndpointRe = regexp.MustCompile(`^(([0-9]{1,3}\.){3}[0-9]{1,3}|\[[0-9A-Fa-f:]+\]):[1-9][0-9]{0,4}$`)
)

// paramRules lists, per action, the parameters it takes and how each is
// checked. A job with a missing, extra or malformed parameter is refused.
var paramRules = map[string]map[string]*regexp.Regexp{
	ActionHealth:               {},
	ActionRestart:              {},
	ActionReload:               {},
	ActionUpdateBAFT:           {"version": versionRe},
	ActionSSHMigrationStage:    {"old_port": portRe, "new_port": portRe},
	ActionSSHMigrationCommit:   {"new_port": portRe},
	ActionSSHMigrationRollback: {},
	ActionPathProbeInventory:   {"discovery_id": tunnelIDRe},
	ActionPathProbeListen:      {"probe_id": tunnelIDRe, "family": familyRe, "port": portRe, "ttl_seconds": probeSecondsRe},
	ActionPathProbeRun:         {"probe_id": tunnelIDRe, "family": familyRe, "target": endpointRe, "payload_bytes": probeBytesRe, "attempts": probeAttemptsRe, "trickle_bytes": probeBytesRe, "trickle_ms": probeDelayRe},
	ActionPathProbeStop:        {"probe_id": tunnelIDRe},

	ActionTunnelPrepareEX:        {"tunnel_id": tunnelIDRe, "public_address": addressRe, "port": portRe, "target": fixedTargetR, "route_id": tunnelIDRe, "record_shaping": boolRe},
	ActionTunnelPrepareIR:        {"tunnel_id": tunnelIDRe, "code": pairCodeRe, "route_listen": loopbackListenR, "route_id": tunnelIDRe},
	ActionTunnelCommitEX:         {"tunnel_id": tunnelIDRe, "reply": replyCodeRe},
	ActionTunnelCommitIR:         {"tunnel_id": tunnelIDRe},
	ActionTunnelHealth:           {"tunnel_id": tunnelIDRe},
	ActionTunnelObserve:          {"tunnel_id": tunnelIDRe},
	ActionTunnelInspect:          {},
	ActionTunnelDiscover:         {},
	ActionTunnelFinalize:         {"tunnel_id": tunnelIDRe},
	ActionTunnelRollback:         {"tunnel_id": tunnelIDRe},
	ActionTunnelRetire:           {"tunnel_id": tunnelIDRe, "generation": epochRe, "config_sha256": sha256Re, "unit_sha256": sha256Re, "marker_sha256": sha256Re},
	ActionTunnelTenantAdd:        {"tunnel_id": tunnelIDRe, "owner_tunnel_id": tunnelIDRe, "mutation_id": tunnelIDRe, "node_id": idRe, "listen": listenEndpointRe, "config_sha256": sha256Re, "unit_sha256": sha256Re, "marker_sha256": sha256Re, "peer_id": idRe, "peer_public_key": noisePublicKeyRe, "route_id": tunnelIDRe, "target": fixedTargetR},
	ActionTunnelTenantRemove:     {"tunnel_id": tunnelIDRe, "owner_tunnel_id": tunnelIDRe, "mutation_id": tunnelIDRe, "node_id": idRe, "listen": listenEndpointRe, "config_sha256": sha256Re, "unit_sha256": sha256Re, "marker_sha256": sha256Re, "peer_id": idRe, "peer_public_key": noisePublicKeyRe, "route_id": tunnelIDRe},
	ActionPublicIngressPreflight: {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR},
	ActionPublicIngressInspect:   {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR, "generation": epochRe, "manifest_digest": sha256Re, "listener_id": idRe, "firewall_id": idRe},
	ActionPublicIngressPrepare:   {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR, "generation": epochRe, "manifest_digest": sha256Re, "listener_id": idRe, "firewall_id": idRe},
	ActionPublicIngressCancel:    {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR, "generation": epochRe, "manifest_digest": sha256Re, "listener_id": idRe, "firewall_id": idRe},
	ActionPublicIngressApply:     {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR, "generation": epochRe, "manifest_digest": sha256Re, "listener_id": idRe, "firewall_id": idRe},
	ActionPublicIngressRetire:    {"tunnel_id": tunnelIDRe, "public_port": portRe, "local_target": loopbackListenR, "generation": epochRe, "manifest_digest": sha256Re, "listener_id": idRe, "firewall_id": idRe},

	ActionCertPrepareEX:  {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe, "epoch": epochRe},
	ActionCertTrustIR:    {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe, "epoch": epochRe, "ca_der": derB64Re, "ca_sha256": sha256Re},
	ActionCertVerifyEX:   {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe},
	ActionCertVerifyIR:   {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe, "cert_der": derB64Re, "cert_sha256": sha256Re},
	ActionCertActivateEX: {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe},
	ActionCertConfirmIR:  {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe, "cert_sha256": sha256Re},
	ActionCertRetireIR:   {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe},
	ActionCertRetireEX:   {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe},
	ActionCertRollback:   {"tunnel_id": tunnelIDRe, "rotation_id": tunnelIDRe, "ex_never_activated": boolRe},
}

// optionalParamRules extends an action without weakening its required shape.
// Legacy BCCs may omit these fields; new BCCs use them for multi-instance
// tunnel isolation.
var optionalParamRules = map[string]map[string]*regexp.Regexp{
	ActionTunnelPrepareEX:      {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR, "join_owner_id": tunnelIDRe},
	ActionTunnelPrepareIR:      {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelCommitEX:       {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR, "join_owner_id": tunnelIDRe},
	ActionTunnelCommitIR:       {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelHealth:         {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelObserve:        {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelInspect:        {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR, "tunnel_id": tunnelIDRe},
	ActionTunnelFinalize:       {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelRollback:       {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionTunnelRetire:         {"instance_id": instanceIDRe, "metrics_listen": loopbackListenR},
	ActionPublicIngressPrepare: {"instance_id": instanceIDRe},
	ActionPublicIngressCancel:  {"instance_id": instanceIDRe},
	ActionPublicIngressApply:   {"instance_id": instanceIDRe, "ingress_protocol": regexp.MustCompile("^1$"), "operation_id": tunnelIDRe, "lease_id": tunnelIDRe, "plan_sha256": sha256Re, "config_sha256": sha256Re, "listen_endpoint": endpointRe, "transport": regexp.MustCompile("^dedicated$")},
	ActionPublicIngressRetire:  {"instance_id": instanceIDRe, "ingress_protocol": regexp.MustCompile("^1$"), "operation_id": tunnelIDRe, "lease_id": tunnelIDRe, "plan_sha256": sha256Re, "config_sha256": sha256Re, "listen_endpoint": endpointRe, "transport": regexp.MustCompile("^dedicated$")},
	ActionCertPrepareEX:        {"instance_id": instanceIDRe},
	ActionCertTrustIR:          {"instance_id": instanceIDRe},
	ActionCertVerifyEX:         {"instance_id": instanceIDRe},
	ActionCertVerifyIR:         {"instance_id": instanceIDRe},
	ActionCertActivateEX:       {"instance_id": instanceIDRe},
	ActionCertConfirmIR:        {"instance_id": instanceIDRe},
	ActionCertRetireIR:         {"instance_id": instanceIDRe},
	ActionCertRetireEX:         {"instance_id": instanceIDRe},
	ActionCertRollback:         {"instance_id": instanceIDRe},
}

// Actions returns the allowlist, sorted.
func Actions() []string {
	out := make([]string, 0, len(paramRules))
	for a := range paramRules {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

type Job struct {
	SchemaVersion int               `json:"schema_version"`
	JobID         string            `json:"job_id"`
	NodeID        string            `json:"node_id"`
	Action        string            `json:"action"`
	Params        map[string]string `json:"params,omitempty"`
	IssuedAt      time.Time         `json:"issued_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
}

// Validate checks a job's shape: IDs, allowlisted action, exact parameters
// and validity window. It does not check time against a clock.
func (j Job) Validate() error {
	if j.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported job schema %d", j.SchemaVersion)
	}
	if !idRe.MatchString(j.JobID) || !idRe.MatchString(j.NodeID) {
		return errors.New("malformed job or node id")
	}
	rules, ok := paramRules[j.Action]
	if !ok {
		return fmt.Errorf("action %q is not allowed", j.Action)
	}
	for k, v := range j.Params {
		re, ok := rules[k]
		if !ok {
			re, ok = optionalParamRules[j.Action][k]
			if !ok {
				return fmt.Errorf("action %s does not take parameter %q", j.Action, k)
			}
		}
		if len(v) > maxParamLen || !re.MatchString(v) {
			return fmt.Errorf("parameter %s is malformed", k)
		}
	}
	for k := range rules {
		if _, ok := j.Params[k]; !ok {
			return fmt.Errorf("action %s needs parameter %q", j.Action, k)
		}
	}
	if err := validateIngressV1(j); err != nil {
		return err
	}
	// Regular expressions constrain syntax, but can still match invalid IPs
	// and ports such as 0 or 65536. Apply the same semantic checks before
	// planning, signing and accepting native tunnel preparation jobs.
	if j.Action == ActionTunnelPrepareEX {
		if !validTCPPort(j.Params["port"]) {
			return errors.New("parameter port must be between 1 and 65535")
		}
		if err := validateTunnelEndpoint(j.Params["target"], false); err != nil {
			return fmt.Errorf("parameter target: %w", err)
		}
	}
	if j.Action == ActionTunnelPrepareIR {
		if err := validateTunnelEndpoint(j.Params["route_listen"], true); err != nil {
			return fmt.Errorf("parameter route_listen: %w", err)
		}
	}
	if j.Action == ActionPublicIngressPreflight || j.Action == ActionPublicIngressInspect || j.Action == ActionPublicIngressPrepare || j.Action == ActionPublicIngressCancel || j.Action == ActionPublicIngressApply || j.Action == ActionPublicIngressRetire {
		if !validTCPPort(j.Params["public_port"]) {
			return errors.New("parameter public_port must be between 1 and 65535")
		}
		if err := validateTunnelEndpoint(j.Params["local_target"], true); err != nil {
			return fmt.Errorf("parameter local_target: %w", err)
		}
		// A wildcard public listener would also capture loopback traffic on
		// the same port. Refuse self-forwarding before signing a job.
		_, targetPort, _ := net.SplitHostPort(j.Params["local_target"])
		publicPort, _ := strconv.Atoi(j.Params["public_port"])
		targetNumber, _ := strconv.Atoi(targetPort)
		if publicPort < 1024 && !(j.Params["ingress_protocol"] == "1" && publicPort != 22) {
			return errors.New("public ingress listen port must be at least 1024 for legacy jobs, and never SSH 22")
		}
		if targetNumber == 22 {
			return errors.New("public ingress target port 22 is reserved for SSH management")
		}
		if publicPort == targetNumber {
			return errors.New("public ingress target port must differ from public port")
		}
	}
	if j.Action == ActionTunnelPrepareEX || j.Action == ActionTunnelPrepareIR {
		if endpoint, ok := j.Params["metrics_listen"]; ok {
			if err := validateTunnelEndpoint(endpoint, true); err != nil {
				return fmt.Errorf("parameter metrics_listen: %w", err)
			}
		}
	}
	if !j.ExpiresAt.After(j.IssuedAt) || j.ExpiresAt.Sub(j.IssuedAt) > MaxLifetime {
		return errors.New("invalid job validity window")
	}
	return nil
}

func validTCPPort(raw string) bool {
	port, err := strconv.Atoi(raw)
	return err == nil && port >= 1 && port <= 65535
}

func validateTunnelEndpoint(endpoint string, loopback bool) error {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || !validTCPPort(port) {
		return errors.New("requires an explicit IP and port between 1 and 65535")
	}
	ip := net.ParseIP(host)
	if ip == nil || (loopback && !ip.IsLoopback()) {
		return errors.New("requires a valid IP in the permitted address scope")
	}
	return nil
}

// Sign has BCC sign a job after checking it.
func Sign(key ed25519.PrivateKey, j Job) (release.Envelope, error) {
	j.SchemaVersion = SchemaVersion
	j.IssuedAt = j.IssuedAt.UTC().Truncate(time.Second)
	j.ExpiresAt = j.ExpiresAt.UTC().Truncate(time.Second)
	if err := j.Validate(); err != nil {
		return release.Envelope{}, err
	}
	payload, err := json.Marshal(j)
	if err != nil {
		return release.Envelope{}, err
	}
	return release.Sign(PayloadType, payload, key), nil
}

// Verifier is the agent side.
type Verifier struct {
	BCCKey ed25519.PublicKey // pinned at enrollment
	NodeID string            // this node
	// Seen reports whether a job ID was already accepted; Verify does not
	// record it, so the caller records only jobs it actually runs.
	Seen func(jobID string) bool
}

// Verify returns the job if this agent may run it now.
func (v Verifier) Verify(env release.Envelope, now time.Time) (Job, error) {
	var j Job
	payload, err := env.Open(PayloadType, v.BCCKey)
	if err != nil {
		return j, fmt.Errorf("job signature: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return j, fmt.Errorf("job payload: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return j, errors.New("job payload: trailing data")
	}
	if err := j.Validate(); err != nil {
		return j, err
	}
	if j.NodeID != v.NodeID {
		return j, fmt.Errorf("job is for node %s, not this node", j.NodeID)
	}
	if j.IssuedAt.After(now.Add(ClockSkew)) {
		return j, errors.New("job is issued in the future")
	}
	if !now.Before(j.ExpiresAt) {
		return j, errors.New("job has expired")
	}
	if v.Seen != nil && v.Seen(j.JobID) {
		return j, fmt.Errorf("job %s was already run", j.JobID)
	}
	return j, nil
}

// VerifiedJob is evidence that a job passed signature, node, window and
// replay checks in Verifier.VerifyAdmitted. Its fields are unexported, so no
// other package can construct a non-zero value or mutate one; the zero value
// is never valid. It carries no authority to mutate a host by itself.
type VerifiedJob struct {
	job      Job
	verified bool
}

// Valid reports whether v was produced by VerifyAdmitted.
func (v VerifiedJob) Valid() bool { return v.verified }

// Job returns a detached copy of the verified job (zero Job if not Valid).
func (v VerifiedJob) Job() Job {
	if !v.verified {
		return Job{}
	}
	j := v.job
	j.Params = make(map[string]string, len(v.job.Params))
	for k, val := range v.job.Params {
		j.Params[k] = val
	}
	return j
}

// VerifyAdmitted is Verify returning sealed evidence for lease minting.
func (v Verifier) VerifyAdmitted(env release.Envelope, now time.Time) (VerifiedJob, error) {
	j, err := v.Verify(env, now)
	if err != nil {
		return VerifiedJob{}, err
	}
	return VerifiedJob{job: j, verified: true}, nil
}
