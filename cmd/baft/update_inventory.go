package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zarkmakerburg/baft/internal/selfupdate"
	"github.com/zarkmakerburg/baft/internal/uninstall"
)

// What is actually installed on this host, component by component.
//
// The release state is one file per install prefix and Core, the agent and
// BCC normally share it, so after Core is updated it already says "current"
// while a BCC binary can still be the old one. Each component is therefore
// judged by its own binaries: identical, byte for byte, to the verified
// release's signed artifact, or not. No binary has to report its own version
// (the agent, BCC and baft-pair have no version command), and nothing is
// executed.

const (
	compTransport = "transport"
	compAgent     = "agent"
	compBCC       = "bcc"

	statCurrent  = "current"
	statOutdated = "outdated"
	statUnknown  = "unknown"
)

type binaryCheck struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"` // identical | differs | unreadable | unverifiable
}

type componentInfo struct {
	Name     string        `json:"name"`
	Unit     string        `json:"unit"`
	Status   string        `json:"status"`
	Binaries []binaryCheck `json:"binaries"`
	Reason   string        `json:"reason,omitempty"`
}

// inventoryHost is the file access the inventory uses, so tests can fake it.
type inventoryHost struct {
	readFile func(path string) ([]byte, error)
	hashFile func(path string) (string, error)
}

var hostInventory = inventoryHost{readFile: os.ReadFile, hashFile: sha256File}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// inspectComponents looks for the transport service, the agent and BCC by
// their unit files and compares each one's binaries with digests, the signed
// sha256 of the verified release's artifacts (nil when no release verified).
func inspectComponents(l menuCfg, arch string, digests map[string]string, h inventoryHost) []componentInfo {
	var out []componentInfo
	unitPath := func(name string) string { return filepath.Join(l.unitDir, name+".service") }

	if raw, err := h.readFile(unitPath(l.service)); err == nil {
		c := componentInfo{Name: compTransport, Unit: l.service}
		if p, ok := uninstall.ParseInstallerServiceUnit(string(raw)); ok {
			c.Binaries = []binaryCheck{
				{Name: "baft", Path: p.Bin},
				{Name: "baft-pair", Path: filepath.Join(filepath.Dir(p.Bin), "baft-pair")},
			}
		} else {
			c.Reason = "the unit is not the one install.sh writes"
		}
		out = append(out, judge(c, arch, digests, h))
	}
	if raw, err := h.readFile(unitPath(l.agentUnit)); err == nil {
		c := componentInfo{Name: compAgent, Unit: l.agentUnit}
		if p, ok := uninstall.ParseInstallerAgentUnit(string(raw)); ok {
			c.Binaries = []binaryCheck{
				{Name: "baft-agent", Path: p.AgentBin},
				{Name: "baft", Path: filepath.Join(p.BinDir, "baft")},
				{Name: "baft-pair", Path: filepath.Join(p.BinDir, "baft-pair")},
			}
		} else {
			c.Reason = "the unit is not the one install.sh writes"
		}
		out = append(out, judge(c, arch, digests, h))
	}
	if raw, err := h.readFile(unitPath(l.bccService)); err == nil {
		c := componentInfo{Name: compBCC, Unit: l.bccService}
		if bin, ok := bccBinary(string(raw)); ok {
			c.Binaries = []binaryCheck{{Name: "baft-bcc", Path: bin}}
		} else {
			c.Reason = "the unit is not the BCC unit install.sh writes"
		}
		out = append(out, judge(c, arch, digests, h))
	}
	return out
}

// bccBinary returns the BCC binary an installer-labelled BCC unit runs.
func bccBinary(unit string) (string, bool) {
	var managed, bcc bool
	bin := ""
	for _, l := range strings.Split(unit, "\n") {
		switch {
		case l == "# baft-managed: true":
			managed = true
		case l == "# baft-component: bcc":
			bcc = true
		case bin == "" && strings.HasPrefix(l, "ExecStart="):
			if f := strings.Fields(strings.TrimPrefix(l, "ExecStart=")); len(f) > 0 && filepath.Base(f[0]) == "baft-bcc" {
				bin = f[0]
			}
		}
	}
	return bin, managed && bcc && bin != ""
}

func judge(c componentInfo, arch string, digests map[string]string, h inventoryHost) componentInfo {
	c.Status = statUnknown
	if len(c.Binaries) == 0 {
		return c
	}
	outdated, unreadable, unverifiable := 0, 0, 0
	for i := range c.Binaries {
		b := &c.Binaries[i]
		want := digests[b.Name+"-linux-"+arch]
		have, err := h.hashFile(b.Path)
		switch {
		case err != nil:
			b.State = "unreadable"
			unreadable++
		case want == "":
			b.State = "unverifiable"
			unverifiable++
		case have == want:
			b.State = "identical"
		default:
			b.State = "differs"
			outdated++
		}
	}
	switch {
	case outdated > 0:
		c.Status = statOutdated
		c.Reason = "a binary differs from the signed release"
	case unreadable > 0:
		c.Reason = "a binary cannot be read"
	case unverifiable > 0:
		c.Reason = "no verified release to compare with"
	default:
		c.Status = statCurrent
	}
	return c
}

// foldInventory lets the components correct the release-level verdict. A
// release state that already says "current" does not make a component whose
// binaries differ from the signed release current.
func foldInventory(rep *selfupdate.Report, comps []componentInfo) {
	if rep.Status != selfupdate.Current {
		return
	}
	var behind []string
	for _, c := range comps {
		if c.Status == statOutdated {
			behind = append(behind, c.Name)
		}
	}
	if len(behind) > 0 {
		rep.Status = selfupdate.UpdateAvailable
		rep.Reason = fmt.Sprintf("the release state already records %s, but these components run binaries that differ from the signed release: %s", rep.Latest, strings.Join(behind, ", "))
	}
}
