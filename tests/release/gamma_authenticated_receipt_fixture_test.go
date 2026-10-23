package release_test

import (
 "context"
 "io"
 "net/url"
 "crypto/tls"
 "crypto/x509"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

// gammaReceiptFixture represents the *expected* node-authenticated result.
// It is not an Agent Apply, a cryptographic signature, or proof of a live listener.
// Gamma will replace this local HTTP fixture with the agreed BCC/Agent integration seam.
type gammaReceiptFixture struct {
 OperationID string `json:"operation_id"`
 NodeID string `json:"node_id"`
 PlanHash string `json:"plan_hash"`
 Generation uint64 `json:"generation"`
 ClientPort int `json:"client_port"`
 Status string `json:"status"`
}

func gammaReadAuthenticatedReceipt(ctx context.Context, endpoint string, roots *x509.CertPool, serverName string, expected gammaReceiptFixture) error {
 if roots == nil || serverName == "" { return errors.New("missing node TLS trust or identity") }
 u,err:=url.Parse(endpoint);if err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil {return errors.New("receipt requires explicit HTTPS endpoint")}
 tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: serverName, MinVersion: tls.VersionTLS12}}
 defer tr.CloseIdleConnections()
 client := &http.Client{Transport: tr, Timeout: 3*time.Second, CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
 if err != nil { return err }
 resp, err := client.Do(req)
 if err != nil { return fmt.Errorf("certificate-validated node HTTPS: %w", err) }
 defer resp.Body.Close()
 if resp.TLS==nil||len(resp.TLS.VerifiedChains)==0{return errors.New("receipt requires verified TLS transport")}
 if resp.StatusCode != http.StatusOK { return fmt.Errorf("node receipt HTTP %d", resp.StatusCode) }
 if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") { return errors.New("node receipt has unexpected content type") }
 dec := json.NewDecoder(http.MaxBytesReader(nil,resp.Body,4096))
 dec.DisallowUnknownFields()
 var got gammaReceiptFixture
 if err := dec.Decode(&got); err != nil { return fmt.Errorf("invalid node receipt: %w",err) }
 var extra any; if err:=dec.Decode(&extra);err!=io.EOF{return errors.New("receipt contains trailing data or exceeds body limit")}
 if got.OperationID == "" || got.NodeID == "" || got.PlanHash == "" || got.Generation == 0 || got.ClientPort < 1 || got.ClientPort > 65535 || got.Status != "applied" {return errors.New("incomplete or non-applied node receipt")}
 if got != expected {return fmt.Errorf("authenticated node receipt does not bind operation, identity, reviewed hash, generation and canonical client port")}
 return nil
}

func TestGammaNodeHTTPSReceiptNegativeContractFixture(t *testing.T) {
 expected := gammaReceiptFixture{OperationID:"op-gamma-1",NodeID:"ir-gamma-1",PlanHash:"digest-of-reviewed-plan",Generation:7,ClientPort:24443,Status:"applied"}
 payload := expected
 handler:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(payload)})
 srv:=httptest.NewTLSServer(handler)
 defer srv.Close()
 roots:=x509.NewCertPool()
 roots.AddCert(srv.Certificate())
 ctx:=context.Background()
 check:=func(label string,root *x509.CertPool,identity string,want gammaReceiptFixture,shouldPass bool){
  t.Helper()
  err:=gammaReadAuthenticatedReceipt(ctx,srv.URL,root,identity,want)
  if shouldPass && err != nil {t.Errorf("%s: correctly trusted test-only receipt rejected: %v",label,err)}
  if !shouldPass && err == nil {t.Errorf("%s: untrusted/mismatched receipt accepted",label)}
 }
 // This positive control proves only HTTPS fixture parsing, never Apply.
 // httptest certificate's DNS identity is example.com on supported Go versions;
 // use certificate DNS SAN explicitly below instead of assuming a hostname.
 serverIdentity:=""
 for _,dns:=range srv.Certificate().DNSNames {if dns!="" {serverIdentity=dns;break}}
 if serverIdentity=="" {serverIdentity="example.com"}
 check("authenticated-fixture",roots,serverIdentity,expected,true)
 unknown:=x509.NewCertPool() // no fixture signing cert trusted
 check("untrusted-ca",unknown,serverIdentity,expected,false)
 check("wrong-node-identity",roots,"wrong-node.invalid",expected,false)
 check("missing-trust",nil,serverIdentity,expected,false)
 check("missing-identity",roots,"",expected,false)
 wrongPort:=expected;wrongPort.ClientPort=443
 check("canonical-port-mismatch",roots,serverIdentity,wrongPort,false)
 wrongPlan:=expected;wrongPlan.PlanHash="other-plan"
 check("reviewed-plan-hash-mismatch",roots,serverIdentity,wrongPlan,false)
 wrongNode:=expected;wrongNode.NodeID="different-node"
 check("node-id-mismatch",roots,serverIdentity,wrongNode,false)
 wrongGeneration:=expected;wrongGeneration.Generation++
 check("generation-mismatch",roots,serverIdentity,wrongGeneration,false)
 wrongOperation:=expected;wrongOperation.OperationID="other-operation"
 check("operation-id-mismatch",roots,serverIdentity,wrongOperation,false)
 payload.Status="preflight_only"
 check("preflight-never-apply",roots,serverIdentity,expected,false)
 payload=expected;payload.ClientPort=0
 check("missing-actual-client-port",roots,serverIdentity,expected,false)
}
