package bcc

import (
 "errors"
 "fmt"
)

// TunnelTopology is separate from the engine and the connection initiator.
type TunnelTopology string
const (
 TunnelDirect TunnelTopology = "direct"
 TunnelReverse TunnelTopology = "reverse"
)

type TunnelEngineCapability struct {
 ID string
 Installed bool
 Direct bool
 Reverse bool
}

// ValidateTunnelWizardEngine refuses unknown, unavailable or unsupported
// combinations. It never silently changes the requested topology.
func ValidateTunnelWizardEngine(topology TunnelTopology, engineID string, engines []TunnelEngineCapability) error {
 if topology!=TunnelDirect && topology!=TunnelReverse {return errors.New("unknown tunnel topology")}
 if engineID=="" {return errors.New("tunnel engine is required")}
 for _,e:=range engines {
  if e.ID!=engineID {continue}
  if !e.Installed {return fmt.Errorf("tunnel engine %q is not installed",engineID)}
  if topology==TunnelDirect && !e.Direct {return fmt.Errorf("tunnel engine %q does not support direct topology",engineID)}
  if topology==TunnelReverse && !e.Reverse {return fmt.Errorf("tunnel engine %q does not support reverse topology",engineID)}
  return nil
 }
 return fmt.Errorf("unknown tunnel engine %q",engineID)
}
