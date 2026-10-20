package bcc

import (
 "fmt"
 "net"
 "strconv"
 "strings"
 "unicode"
)

// PublicIngressSpec is a validated desired-state contract, NOT an applied
// firewall rule or running listener. Agent reconciliation is a separate gate.
type PublicIngressSpec struct {
 IRNode string `json:"ir_node"`
 PublicPort int `json:"public_port"`
 LocalTarget string `json:"local_target"`
 TunnelID string `json:"tunnel_id"`
}

// PlanPublicIngress creates a fail-closed desired-state record. The public
// port is intentionally separate from the EX inter-node transport port.
func PlanPublicIngress(irNode, tunnelID string, publicPort int, localTarget string) (PublicIngressSpec, error) {
 if strings.TrimSpace(irNode)=="" || strings.TrimSpace(tunnelID)=="" {return PublicIngressSpec{}, fmt.Errorf("public ingress requires IR node and tunnel ID")}
 // Reject control characters in identifiers before persisting or auditing them.
 // Whitespace around an identifier is canonicalized, but embedded newlines,
 // tabs, and other control bytes must never reach ownership or audit keys.
 for _, identifier := range []string{irNode, tunnelID} {
  for _, r := range identifier {
   if unicode.IsControl(r) {return PublicIngressSpec{}, fmt.Errorf("public ingress identifier contains a control character")}
  }
 }
 if publicPort<1 || publicPort>65535 {return PublicIngressSpec{}, fmt.Errorf("public ingress port must be 1..65535")}
 host, port, err := net.SplitHostPort(localTarget)
 if err!=nil {return PublicIngressSpec{}, fmt.Errorf("invalid local ingress target: %w",err)}
 ip:=net.ParseIP(host)
 if ip==nil || !ip.IsLoopback() {return PublicIngressSpec{}, fmt.Errorf("local ingress target must use a loopback IP")}
 n,err:=strconv.Atoi(port)
 if err!=nil || n<1 || n>65535 {return PublicIngressSpec{}, fmt.Errorf("invalid local ingress target port")}
 // The ingress listener must never forward back into its own port, even
 // through a different loopback address or an IPv6 wildcard listener.
 if n == publicPort {return PublicIngressSpec{}, fmt.Errorf("public ingress target port must differ from public port")}
 return PublicIngressSpec{IRNode:strings.TrimSpace(irNode),TunnelID:strings.TrimSpace(tunnelID),PublicPort:publicPort,LocalTarget:net.JoinHostPort(ip.String(),strconv.Itoa(n))},nil
}
