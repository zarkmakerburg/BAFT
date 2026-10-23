package release_test

import (
 "context"
 "crypto/tls"
 "crypto/x509"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/http/httptest"
 "net/url"
 "reflect"
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// This test-only client models a BCC verifier of the enrolled Agent credential
// over certificate-validated node HTTPS. No Agent Apply is simulated here.
func gammaReadEnrolledReceiptV1(ctx context.Context, endpoint string, roots *x509.CertPool, serverName, enrolledCredential string, expected ingressauthority.ReceiptV1) error {
 if roots==nil || serverName=="" || enrolledCredential=="" {return errors.New("missing TLS identity or enrolled agent credential")}
 u,err:=url.Parse(endpoint)
 if err!=nil || u.Scheme!="https" || u.Host=="" || u.User!=nil{return errors.New("receipt requires HTTPS")}
 tr:=&http.Transport{TLSClientConfig:&tls.Config{RootCAs:roots,ServerName:serverName,MinVersion:tls.VersionTLS12}}
 defer tr.CloseIdleConnections()
 client:=&http.Client{Transport:tr,Timeout:3*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,endpoint,nil);if err!=nil{return err}
 req.Header.Set("Authorization","Bearer "+enrolledCredential)
 resp,err:=client.Do(req);if err!=nil{return err};defer resp.Body.Close()
 if resp.TLS==nil || len(resp.TLS.VerifiedChains)==0{return errors.New("unverified TLS")}
 if resp.StatusCode!=http.StatusOK{return errors.New("unverified enrolled Agent or receipt")}
 if !strings.HasPrefix(resp.Header.Get("Content-Type"),"application/json"){return errors.New("non-JSON receipt")}
 dec:=json.NewDecoder(io.LimitReader(resp.Body,4097));dec.DisallowUnknownFields()
 var got ingressauthority.ReceiptV1
 if err:=dec.Decode(&got);err!=nil{return err}
 var extra any;if err:=dec.Decode(&extra);err!=io.EOF{return errors.New("receipt trailing or oversized data")}
 if got.JobID==""||got.Binding.NodeID==""||got.Binding.PlanSHA256==""||got.Binding.ConfigPort==0||got.Phase!="applied"||!got.OwnershipVerified{return errors.New("receipt lacks actual apply evidence")}
 if !reflect.DeepEqual(got,expected){return errors.New("receipt does not match reviewed command and exact canonical port")}
 return nil
}

func TestGammaEnrolledNodeHTTPSSharedReceiptV1Fixture(t *testing.T) {
 const credential="enrolled-agent-lab-credential"
 expected:=ingressauthority.ReceiptV1{SchemaVersion:1,JobID:"job-gamma",Action:"apply",Phase:"applied",OwnershipVerified:true,Binding:ingressauthority.BindingV1{NodeID:"ir-gamma",OperationID:"operation-gamma",LeaseID:"lease-gamma",Port:24443,ConfigPort:24443,PlanSHA256:strings.Repeat("a",64)}}
 payload:=expected
 handler:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="Bearer "+credential{http.Error(w,"unauthorized",http.StatusUnauthorized);return}
  w.Header().Set("Content-Type","application/json")
  _=json.NewEncoder(w).Encode(payload)
 })
 srv:=httptest.NewTLSServer(handler);defer srv.Close()
 roots:=x509.NewCertPool();roots.AddCert(srv.Certificate())
 identity:="example.com"
 if len(srv.Certificate().DNSNames)>0 {identity=srv.Certificate().DNSNames[0]}
 verify:=func(label,token string,want ingressauthority.ReceiptV1,pass bool){
  t.Helper();err:=gammaReadEnrolledReceiptV1(context.Background(),srv.URL,roots,identity,token,want)
  if pass&&err!=nil{t.Errorf("%s rejected: %v",label,err)}
  if !pass&&err==nil{t.Errorf("%s accepted unexpectedly",label)}
 }
 verify("enrolled TLS shared receipt v1 fixture",credential,expected,true)
 verify("missing enrolled credential","",expected,false)
 verify("wrong enrolled credential","wrong",expected,false)
 wrong:=expected;wrong.Binding.ConfigPort=443
 verify("canonical config port mismatch",credential,wrong,false)
 wrong=expected;wrong.Binding.OperationID="other-operation"
 verify("operation mismatch",credential,wrong,false)
 wrong=expected;wrong.Binding.PlanSHA256=strings.Repeat("b",64)
 verify("reviewed SHA mismatch",credential,wrong,false)
 wrong=expected;wrong.Binding.NodeID="other-node"
 verify("node mismatch",credential,wrong,false)
 payload.Phase="preflight"
 verify("preflight cannot claim apply",credential,expected,false)
 payload=expected;payload.OwnershipVerified=false
 verify("unverified ownership",credential,expected,false)
 plain:=httptest.NewServer(handler);defer plain.Close()
 if err:=gammaReadEnrolledReceiptV1(context.Background(),plain.URL,roots,identity,credential,expected);err==nil{t.Error("HTTP accepted")}
}
