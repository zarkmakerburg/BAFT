// Package ingressmanifest stores prepared ingress bytes, not live resources.
// A Proof demonstrates correspondence with a caller-held digest and generation;
// it does not authenticate a caller, establish listener ownership or authorize
// deployment. There is no agent action, service management or network operation.
package ingressmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/publicingress"
)

var (
	ErrBusy                = errors.New("ingress manifest store is in use")
	ErrNotPrepared         = errors.New("ingress manifest is not prepared")
	ErrDurabilityUncertain = errors.New("ingress manifest rename completed but durability could not be confirmed")
	idPattern              = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	instancePattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)
	digestPattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const maxManifestBytes = 16 << 10
const (
	PhasePrepared  = "PREPARED"
	PhaseCancelled = "CANCELLED"
)

type Config struct {
	Listen        string `json:"listen"`
	Target        string `json:"target"`
	MaxClients    int    `json:"max_clients"`
	DialTimeoutMS int    `json:"dial_timeout_ms"`
}

type Spec struct {
	TunnelID   string `json:"tunnel_id"`
	InstanceID string `json:"instance_id"`
	Generation uint64 `json:"generation"`
	Config     Config `json:"config"`
}

type Manifest struct {
	SchemaVersion       int    `json:"schema_version"`
	Phase               string `json:"phase"`
	Spec                Spec   `json:"spec"`
	ConfigSHA256        string `json:"config_sha256"`
	CancelledFromSHA256 string `json:"cancelled_from_sha256,omitempty"`
}

type Proof struct {
	TunnelID       string `json:"tunnel_id"`
	InstanceID     string `json:"instance_id"`
	Generation     uint64 `json:"generation"`
	ConfigSHA256   string `json:"config_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type Inspection struct {
	Manifest              Manifest `json:"manifest"`
	Proof                 Proof    `json:"proof"`
	ManifestBytesMatch    bool     `json:"manifest_bytes_match"`
	PreparedBytesMatch    bool     `json:"prepared_bytes_match"`
	LiveOwnershipVerified bool     `json:"live_ownership_verified"`
	ReadyToApply          bool     `json:"ready_to_apply"`
}

func normalize(spec Spec) (Spec, error) {
	// Native tunnel managers alias "default" to the empty legacy scope. Reject
	// the alias here so one intended instance cannot acquire two manifest keys.
	if spec.InstanceID == "default" || !idPattern.MatchString(spec.TunnelID) || (spec.InstanceID != "" && !instancePattern.MatchString(spec.InstanceID)) || spec.Generation == 0 {
		return Spec{}, errors.New("invalid ingress owner tuple or generation")
	}
	if spec.Config.MaxClients == 0 {
		spec.Config.MaxClients = 128
	}
	if spec.Config.DialTimeoutMS == 0 {
		spec.Config.DialTimeoutMS = 3000
	}
	if spec.Config.DialTimeoutMS < 10 || spec.Config.DialTimeoutMS > 10000 {
		return Spec{}, errors.New("invalid ingress dial timeout")
	}
	_, err := publicingress.New(publicingress.Config{Listen: spec.Config.Listen, Target: spec.Config.Target, MaxClients: spec.Config.MaxClients, DialTimeout: time.Duration(spec.Config.DialTimeoutMS) * time.Millisecond})
	if err != nil {
		return Spec{}, err
	}
	canonical := func(endpoint string) string {
		host, port, _ := net.SplitHostPort(endpoint)
		n, _ := strconv.Atoi(port)
		return net.JoinHostPort(net.ParseIP(host).String(), strconv.Itoa(n))
	}
	spec.Config.Listen = canonical(spec.Config.Listen)
	spec.Config.Target = canonical(spec.Config.Target)
	return spec, nil
}

func hash(raw []byte) string                  { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func filename(tunnel, instance string) string { return hash([]byte(tunnel+"\x00"+instance)) + ".json" }

func encode(spec Spec) ([]byte, Proof, error) {
	return encodeRecord(Manifest{SchemaVersion: 2, Phase: PhasePrepared, Spec: spec})
}

// Preview computes the exact canonical prepared-file proof without writing a
// file. A caller can retain this digest before authorizing a signed prepare.
func Preview(spec Spec) (Proof, error) {
	_, proof, err := encode(spec)
	return proof, err
}

func encodeRecord(manifest Manifest) ([]byte, Proof, error) {
	if manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2 {
		return nil, Proof{}, errors.New("unsupported ingress manifest schema")
	}
	switch manifest.Phase {
	case PhasePrepared:
		if manifest.CancelledFromSHA256 != "" {
			return nil, Proof{}, errors.New("prepared manifest cannot carry cancellation lineage")
		}
	case PhaseCancelled:
		if manifest.SchemaVersion != 2 || !digestPattern.MatchString(manifest.CancelledFromSHA256) {
			return nil, Proof{}, errors.New("invalid cancelled ingress manifest lineage")
		}
	default:
		return nil, Proof{}, errors.New("unsupported ingress manifest phase")
	}
	normalized, err := normalize(manifest.Spec)
	if err != nil {
		return nil, Proof{}, err
	}
	config, err := json.Marshal(normalized.Config)
	if err != nil {
		return nil, Proof{}, err
	}
	manifest.Spec = normalized
	manifest.ConfigSHA256 = hash(config)
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, Proof{}, err
	}
	proof := Proof{TunnelID: normalized.TunnelID, InstanceID: normalized.InstanceID, Generation: normalized.Generation, ConfigSHA256: manifest.ConfigSHA256, ManifestSHA256: hash(raw)}
	return raw, proof, nil
}

func decode(raw []byte) (Manifest, Proof, error) {
	var manifest Manifest
	if len(raw) == 0 || len(raw) > maxManifestBytes {
		return manifest, Proof{}, errors.New("invalid ingress manifest size")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, Proof{}, errors.New("invalid ingress manifest")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return Manifest{}, Proof{}, errors.New("trailing ingress manifest data")
	}
	canonical, proof, err := encodeRecord(manifest)
	if err != nil {
		return Manifest{}, Proof{}, err
	}
	// A single deterministic encoding rejects missing/duplicate/unknown fields,
	// stale config hashes and noncanonical endpoints instead of adopting them.
	if !bytes.Equal(raw, canonical) {
		return Manifest{}, Proof{}, errors.New("ingress manifest is not canonical or its config digest differs")
	}
	return manifest, proof, nil
}

func validateProof(proof Proof) error {
	if proof.InstanceID == "default" || !idPattern.MatchString(proof.TunnelID) || (proof.InstanceID != "" && !instancePattern.MatchString(proof.InstanceID)) || proof.Generation == 0 || !digestPattern.MatchString(proof.ConfigSHA256) || !digestPattern.MatchString(proof.ManifestSHA256) {
		return fmt.Errorf("invalid expected ingress manifest proof")
	}
	return nil
}
