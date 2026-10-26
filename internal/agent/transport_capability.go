package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zarkmakerburg/baft/internal/protocol"
)

// transportCapabilityToken is the header spelling of a protocol capability:
// the header allows only [a-z0-9-], so "carrier.h2" travels as "carrier-h2".
func transportCapabilityToken(c string) string { return strings.ReplaceAll(c, ".", "-") }

// transportCapabilities returns the header tokens of the transport capabilities
// the INSTALLED baft binary implements, as `baft version --json` reports them.
// It is evidence about that binary, not about the agent's own build, because
// update_baft replaces baft without replacing the agent. Only tokens this
// agent also knows are passed on. The answer is cached per binary identity
// (size, modification time), so a binary swap is re-read on the next poll.
// Any failure (missing binary, old binary without the field, bad output)
// reports nothing, which BCC treats as "no capability".
func (a *Agent) transportCapabilities(ctx context.Context) []string {
	if a.cfg.BinDir == "" || a.cfg.System == nil {
		return nil
	}
	bin := filepath.Join(a.cfg.BinDir, "baft")
	st, err := os.Stat(bin)
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	key := fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
	a.capsMu.Lock()
	defer a.capsMu.Unlock()
	if key == a.transportCapsKey {
		return a.transportCaps
	}
	out, err := a.cfg.System.Run(ctx, bin, "version", "--json")
	var v struct {
		Capabilities []string `json:"capabilities"`
	}
	var caps []string
	if err == nil && json.NewDecoder(strings.NewReader(out)).Decode(&v) == nil {
		for _, c := range v.Capabilities {
			if protocol.IsKnownCapability(c) {
				caps = append(caps, transportCapabilityToken(c))
			}
		}
	}
	a.transportCapsKey, a.transportCaps = key, caps
	return caps
}
