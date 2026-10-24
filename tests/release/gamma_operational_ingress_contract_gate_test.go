package release_test

import (
 "context"
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/ingressauthority"
 "github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// The isolated operational LAB must reuse these *real shared types*, never
// treat valid fixture JSON as a successful mutation or mint a fake lease.
func TestGammaRealIngressContractPreflightRefusalMatrix(t *testing.T) {
 spec:=ingressmanifest.Spec{TunnelID:"gamma-ingress-lab",Generation:1,Config:ingressmanifest.Config{Listen:"0.0.0.0:24443",Target:"127.0.0.1:14444",MaxClients:32,DialTimeoutMS:3000}}
 proof,err:=ingressmanifest.Preview(spec);if err!=nil{t.Fatal(err)}
 binding:=ingressauthority.BindingV1{NodeID:"ir-gamma",OperationID:"op-gamma",LeaseID:"lease-gamma",Port:24443,ConfigPort:24443,ListenEndpoint:spec.Config.Listen,LocalTarget:spec.Config.Target,Transport:"dedicated",PlanSHA256:strings.Repeat("a",64),ListenerID:"listener-gamma",FirewallID:"firewall-gamma",Spec:spec,Proof:proof}
 if err:=binding.Validate();err!=nil{t.Fatalf("valid real shared BindingV1 rejected: %v",err)}
 now:=time.Now()
 apply:=ingressauthority.CommandV1{IngressProtocol:ingressauthority.ProtocolV1,JobID:"gamma-signed-job",Action:"public_ingress_apply",IssuedAt:now.Add(-time.Second),ExpiresAt:now.Add(time.Minute),Binding:binding}
 if err:=apply.Validate(now);err!=nil{t.Fatalf("valid real shared CommandV1 rejected: %v",err)}
 // A zero/nil lease can never provide the missing durable OS authority.
 if err:=((*ingressauthority.Lease)(nil)).CheckLockedIngress(context.Background(),apply);err==nil{t.Fatal("nil authority accepted")}
 var zero ingressauthority.Lease
 if err:=zero.CheckLockedIngress(context.Background(),apply);err==nil{t.Fatal("zero authority accepted")}
 mutations:=map[string]func(*ingressauthority.BindingV1){
  "config-port-drift":func(b *ingressauthority.BindingV1){b.ConfigPort=24444},
  "reviewed-hash-drift":func(b *ingressauthority.BindingV1){b.PlanSHA256=strings.Repeat("b",64)},
  "node-change":func(b *ingressauthority.BindingV1){b.NodeID="other-ir"},
  "lease-change":func(b *ingressauthority.BindingV1){b.LeaseID="other-lease"},
  "target-drift":func(b *ingressauthority.BindingV1){b.LocalTarget="127.0.0.1:14445"},
  "proof-drift":func(b *ingressauthority.BindingV1){b.Proof.ConfigSHA256=strings.Repeat("c",64)},
 }
 for name,mutate:=range mutations {t.Run(name,func(t *testing.T){changed:=binding;mutate(&changed);if err:=changed.MatchReviewed(binding);err==nil{t.Fatal("changed binding matched immutable reviewed plan")}})}
 wrongAction:=apply;wrongAction.Action="apply"
 if err:=wrongAction.Validate(now);err==nil{t.Fatal("noncanonical apply admitted")}
 retired:=apply;retired.Action="public_ingress_retire";retired.JobID="gamma-retire-job"
 if err:=retired.Validate(now);err!=nil{t.Fatalf("valid separately signed retire rejected: %v",err)}
 if retired.Binding!=apply.Binding{t.Fatal("retire resource binding drifted")}
}
