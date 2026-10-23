package bcc

import "testing"

func TestTunnelWizardEngineCapabilities(t *testing.T) {
 engines:=[]TunnelEngineCapability{{ID:"native",Installed:true,Direct:true,Reverse:false},{ID:"future",Installed:false,Direct:true,Reverse:true}}
 cases:=[]struct{top TunnelTopology;engine string;ok bool}{
  {TunnelDirect,"native",true},
  {TunnelReverse,"native",false},
  {TunnelDirect,"future",false},
  {TunnelReverse,"future",false},
  {TunnelDirect,"missing",false},
  {TunnelTopology("sideways"),"native",false},
  {TunnelDirect,"",false},
 }
 for _,tc:=range cases {
  err:=ValidateTunnelWizardEngine(tc.top,tc.engine,engines)
  if (err==nil)!=tc.ok {t.Errorf("%s/%s: error=%v expected valid=%v",tc.top,tc.engine,err,tc.ok)}
 }
}
