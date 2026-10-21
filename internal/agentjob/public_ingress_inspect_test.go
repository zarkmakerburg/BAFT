package agentjob

import (
 "strings"
 "testing"
 "time"
)

func TestPublicIngressInspectRequiresExactProofBoundParameters(t *testing.T) {
 now := time.Now().UTC()
 base := map[string]string{
  "tunnel_id":"tun-a", "public_port":"54619", "local_target":"127.0.0.1:1443",
  "generation":"7", "manifest_digest":strings.Repeat("a",64),
  "listener_id":"baft-listener-7", "firewall_id":"baft-firewall-7",
 }
 makeJob:=func(params map[string]string) Job{return Job{SchemaVersion:SchemaVersion,JobID:"ingress-inspect",NodeID:"ir-node",Action:ActionPublicIngressInspect,Params:params,IssuedAt:now,ExpiresAt:now.Add(time.Minute)}}
 if err:=makeJob(base).Validate();err!=nil {t.Fatalf("valid inspect rejected: %v",err)}
 for _,key:=range []string{"generation","manifest_digest","listener_id","firewall_id","public_port","local_target"} {
  t.Run("missing-"+key,func(t *testing.T){
   params:=map[string]string{};for k,v:=range base {if k!=key {params[k]=v}}
   if err:=makeJob(params).Validate();err==nil {t.Fatal("missing proof accepted")}
  })
 }
 for _,tc:=range []struct{key,value string}{
  {"generation","0"},{"generation","-1"},{"manifest_digest",strings.Repeat("g",64)},
  {"listener_id","../other"},{"firewall_id","other/rule"},
  {"public_port","22"},{"local_target","0.0.0.0:1443"},
 }{
  t.Run(tc.key+"-"+tc.value,func(t *testing.T){
   params:=map[string]string{};for k,v:=range base{params[k]=v};params[tc.key]=tc.value
   if err:=makeJob(params).Validate();err==nil {t.Fatal("invalid inspection parameter accepted")}
  })
 }
 params:=map[string]string{};for k,v:=range base {params[k]=v};params["command"]="id"
 if err:=makeJob(params).Validate();err==nil {t.Fatal("free-form parameter accepted")}
}
