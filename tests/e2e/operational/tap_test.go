//go:build linux

package operational

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

// tap is a TLS reverse proxy in front of the real BCC. It is transparent
// unless a hook rewrites the signed-job poll response, which lets a test
// deliver a replayed, expired, mis-addressed or tampered envelope to the real
// Agent exactly as a network attacker (or compromised signer) could. Everything
// else (receipts, acks, node auth) reaches the real BCC.
type tap struct {
	srv *httptest.Server
	b   *bccRig

	mu       sync.Mutex
	hook     func(orig []byte) []byte
	captured [][]byte // every job-poll body that carried jobs
	auth     []string // Authorization headers the front saw
}

func newTap(t *testing.T, b *bccRig) *tap {
	t.Helper()
	target, err := url.Parse(b.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	p := httputil.NewSingleHostReverseProxy(target)
	p.Transport = b.srv.Client().Transport
	tp := &tap{b: b}
	p.ModifyResponse = func(resp *http.Response) error {
		if resp.Request.URL.Path != "/api/agent/jobs" || resp.StatusCode != http.StatusOK {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		tp.mu.Lock()
		if bytes.Contains(body, []byte(`"signed"`)) {
			tp.captured = append(tp.captured, append([]byte(nil), body...))
		}
		hook := tp.hook
		tp.mu.Unlock()
		if hook != nil {
			body = hook(body)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	tp.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tp.mu.Lock()
		tp.auth = append(tp.auth, r.Header.Get("Authorization"))
		tp.mu.Unlock()
		p.ServeHTTP(w, r)
	}))
	t.Cleanup(tp.srv.Close)
	return tp
}

func (tp *tap) setHook(h func([]byte) []byte) { tp.mu.Lock(); tp.hook = h; tp.mu.Unlock() }

func (tp *tap) lastCaptured() []byte {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if len(tp.captured) == 0 {
		return nil
	}
	return tp.captured[len(tp.captured)-1]
}

// serve makes the poll return exactly the given signed envelopes.
func serve(envs ...release.Envelope) func([]byte) []byte {
	return func([]byte) []byte {
		var list []bcc.AgentJob
		for i := range envs {
			env := envs[i]
			list = append(list, bcc.AgentJob{Job: bcc.Job{ID: jobIDOf(env)}, Signed: &env})
		}
		out, _ := json.Marshal(list)
		return out
	}
}

func jobIDOf(env release.Envelope) string {
	var j struct {
		JobID string `json:"job_id"`
	}
	// The envelope payload is the base64 signed JSON of the job; the Agent
	// only accepts a wrapper whose ID equals it.
	raw, _ := base64.StdEncoding.DecodeString(env.Payload)
	_ = json.Unmarshal(raw, &j)
	return j.JobID
}

// jobParams is the exact parameter set BCC signs for an operational job.
func jobParams(cmd ingressauthority.CommandV1) map[string]string {
	b := cmd.Binding
	p := map[string]string{
		"tunnel_id": b.Spec.TunnelID, "public_port": strconv.Itoa(int(b.Port)), "local_target": b.LocalTarget,
		"generation": strconv.FormatUint(b.Spec.Generation, 10), "manifest_digest": b.Proof.ManifestSHA256,
		"listener_id": b.ListenerID, "firewall_id": b.FirewallID, "ingress_protocol": ingressauthority.ProtocolV1,
		"operation_id": b.OperationID, "lease_id": b.LeaseID, "plan_sha256": b.PlanSHA256,
		"config_sha256": b.Proof.ConfigSHA256, "listen_endpoint": b.ListenEndpoint, "transport": b.Transport,
	}
	if b.Spec.InstanceID != "" {
		p["instance_id"] = b.Spec.InstanceID
	}
	return p
}

// forgedJob is the command's signed job with the given mutation applied.
func forgedJob(cmd ingressauthority.CommandV1, mutate func(j *agentjob.Job)) agentjob.Job {
	j := agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: cmd.JobID, NodeID: cmd.Binding.NodeID, Action: cmd.Action,
		Params: jobParams(cmd), IssuedAt: cmd.IssuedAt, ExpiresAt: cmd.ExpiresAt}
	if mutate != nil {
		mutate(&j)
	}
	return j
}

// reDigest recomputes the canonical manifest/config digests for params whose
// generation or listen endpoint was changed, so a forged job is internally
// consistent and only the reviewed/reserved identity can refuse it.
func reDigest(j *agentjob.Job, instance string) {
	gen, _ := strconv.ParseUint(j.Params["generation"], 10, 64)
	proof, err := ingressmanifest.Preview(ingressmanifest.Spec{TunnelID: j.Params["tunnel_id"], InstanceID: instance, Generation: gen,
		Config: ingressmanifest.Config{Listen: j.Params["listen_endpoint"], Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000}})
	if err != nil {
		panic(err)
	}
	j.Params["manifest_digest"], j.Params["config_sha256"] = proof.ManifestSHA256, proof.ConfigSHA256
}

var _ = strings.TrimSpace
var _ = time.Second
