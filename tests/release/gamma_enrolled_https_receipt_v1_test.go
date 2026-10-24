package release_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// Gamma models the correct direction: enrolled Agent POSTs ReceiptV1 to BCC.
// This is a fixture receiver, not the operational BCC endpoint or Agent Apply.
func gammaPostEnrolledReceipt(ctx context.Context, endpoint string, client *http.Client, credential string, receipt ingressauthority.ReceiptV1) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("Agent receipt requires HTTPS")
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := safeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 {
		return errors.New("unverified BCC TLS")
	}
	if resp.StatusCode != http.StatusNoContent {
		return errors.New("BCC refused receipt")
	}
	return nil
}

func gammaReceiptReceiver(credential string, reviewed ingressauthority.ReceiptV1) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.TLS == nil {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		// Enrolled Agent credential authenticates the sender separately from BCC TLS.
		if r.Header.Get("Authorization") != "Bearer "+credential {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "content type", http.StatusUnsupportedMediaType)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil || len(raw) > 4096 {
			http.Error(w, "oversized", http.StatusRequestEntityTooLarge)
			return
		}
		for _, key := range []string{`"job_id"`, `"operation_id"`, `"node_id"`, `"config_port"`, `"public_port"`, `"plan_sha256"`, `"phase"`, `"ownership_verified"`} {
			if bytes.Count(raw, []byte(key)) != 1 {
				http.Error(w, "duplicate or missing receipt identity field", http.StatusBadRequest)
				return
			}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var got ingressauthority.ReceiptV1
		if dec.Decode(&got) != nil {
			http.Error(w, "bad receipt", http.StatusBadRequest)
			return
		}
		var trailing any
		if dec.Decode(&trailing) != io.EOF {
			http.Error(w, "trailing JSON", http.StatusBadRequest)
			return
		}
		if got.Phase != "applied" || !got.OwnershipVerified || !reflect.DeepEqual(got, reviewed) {
			http.Error(w, "not exact reviewed apply", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestGammaAgentToBCCEnrolledHTTPSSharedReceiptV1(t *testing.T) {
	expected := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: "job-gamma", Action: "apply", Phase: "applied", OwnershipVerified: true, Binding: ingressauthority.BindingV1{NodeID: "ir-gamma", OperationID: "op-gamma", LeaseID: "lease-gamma", Port: 24443, ConfigPort: 24443, PlanSHA256: strings.Repeat("a", 64)}}
	const token = "enrolled-agent-gamma"
	srv := httptest.NewTLSServer(gammaReceiptReceiver(token, expected))
	defer srv.Close()
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	check := func(label string, cred string, rec ingressauthority.ReceiptV1, ok bool) {
		t.Helper()
		err := gammaPostEnrolledReceipt(context.Background(), srv.URL, client, cred, rec)
		if ok && err != nil {
			t.Errorf("%s rejected: %v", label, err)
		}
		if !ok && err == nil {
			t.Errorf("%s incorrectly accepted", label)
		}
	}
	check("enrolled agent POST via validated HTTPS", token, expected, true)
	check("wrong enrolled credential", "other", expected, false)
	check("missing enrolled credential", "", expected, false)
	changed := expected
	changed.Binding.ConfigPort = 443
	check("different config port", token, changed, false)
	changed = expected
	changed.Binding.PlanSHA256 = strings.Repeat("b", 64)
	check("different plan", token, changed, false)
	changed = expected
	changed.Binding.NodeID = "wrong-node"
	check("different node", token, changed, false)
	changed = expected
	changed.Binding.OperationID = "wrong-operation"
	check("different operation", token, changed, false)
	changed = expected
	changed.Phase = "preflight"
	check("preflight not apply", token, changed, false)
	changed = expected
	changed.OwnershipVerified = false
	check("not verified", token, changed, false)
	plain := httptest.NewServer(gammaReceiptReceiver(token, expected))
	defer plain.Close()
	checkHTTP := gammaPostEnrolledReceipt(context.Background(), plain.URL, client, token, expected)
	if checkHTTP == nil {
		t.Error("accepted plaintext HTTP")
	}
	// The enrolled bearer token alone must not override invalid BCC TLS trust.
	untrusted := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := gammaPostEnrolledReceipt(context.Background(), srv.URL, untrusted, token, expected); err == nil {
		t.Error("accepted receipt over untrusted BCC TLS")
	}
	untrusted.Transport.(*http.Transport).CloseIdleConnections()
	// Redirects are prohibited, including a TLS-to-plaintext downgrade.
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	if err := gammaPostEnrolledReceipt(context.Background(), redirect.URL, redirect.Client(), token, expected); err == nil {
		t.Error("followed HTTPS-to-HTTP redirect")
	}
	// Cross-origin TLS redirects are also forbidden.
	crossOrigin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusTemporaryRedirect)
	}))
	defer crossOrigin.Close()
	if err := gammaPostEnrolledReceipt(context.Background(), crossOrigin.URL, crossOrigin.Client(), token, expected); err == nil {
		t.Error("followed cross-origin redirect")
	}
}

func TestGammaBCCReceiverRejectsUntrustedReceiptJSON(t *testing.T) {
	reviewed := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: "j", Action: "apply", Phase: "applied", OwnershipVerified: true, Binding: ingressauthority.BindingV1{NodeID: "ir", OperationID: "op", Port: 24443, ConfigPort: 24443, PlanSHA256: strings.Repeat("b", 64)}}
	srv := httptest.NewTLSServer(gammaReceiptReceiver("enrolled", reviewed))
	defer srv.Close()
	send := func(label, stringBody string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(stringBody))
		req.Header.Set("Authorization", "Bearer enrolled")
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			t.Errorf("%s accepted", label)
		}
	}
	b, _ := json.Marshal(reviewed)
	send("trailing object", string(b)+" {}")
	send("oversized whitespace", string(b)+strings.Repeat(" ", 5000))
	send("unknown field", strings.TrimSuffix(string(b), "}")+",\"unknown\":1}")
	send("duplicate job_id", strings.TrimSuffix(string(b), "}")+",\"job_id\":\"j\"}")
	send("duplicate binding config_port", strings.Replace(string(b), `"config_port":24443`, `"config_port":24443,"config_port":24443`, 1))
}
