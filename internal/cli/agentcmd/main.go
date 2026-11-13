// Command baft-agent runs on each BAFT server. It pulls jobs from BCC and
// runs only jobs signed by the pinned BCC key for allowlisted actions; it
// installs only releases signed under the pinned release root.
//
//	baft-agent --bcc-url https://bcc.example.com --node-id ex-1 \
//	  --token-file /etc/baft-agent/token --bcc-job-key /etc/baft-agent/bcc-job.pub \
//	  --release-root /etc/baft-agent/release-root.pub
package agentcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

type hostSystem struct{}

func (hostSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func (hostSystem) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Version is set by the command that runs the agent (the release version).
var Version = "dev"

// Main runs the agent with args (without the program name) and returns its
// exit code.
func Main(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args, os.Stderr)
}

func readSecret(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s must not be readable or writable by group/other", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return v, nil
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("baft-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bccURL := fs.String("bcc-url", "", "BCC base URL (https)")
	nodeID := fs.String("node-id", "", "this node's ID in BCC")
	tokenFile := fs.String("token-file", "/etc/baft-agent/token", "agent token file (owner-only)")
	jobKeyFile := fs.String("bcc-job-key", "/etc/baft-agent/bcc-job.pub", "pinned BCC job public key (baft-bcc jobkey show)")
	rootFile := fs.String("release-root", "/etc/baft-agent/release-root.pub", "pinned release root public key")
	releaseBase := fs.String("release-base-url", "https://github.com/zarkmakerburg/baft/releases/download", "release assets at <base>/<version>/<file>")
	revocations := fs.String("revocations-url", "https://raw.githubusercontent.com/zarkmakerburg/baft/main/release/keys/revocations.json", "current release revocation list")
	stateDir := fs.String("state-dir", "/var/lib/baft-agent", "agent state (seen jobs)")
	ingressManifestDir := fs.String("ingress-manifest-dir", "", "pre-existing private ingress manifest directory (empty disables prepare)")
	releaseState := fs.String("release-state", "/opt/baft/release-state.json", "installer release trust state")
	binDir := fs.String("bin-dir", "/usr/local/bin", "where baft and baft-pair are installed")
	service := fs.String("service", "baft", "BAFT systemd unit")
	configDir := fs.String("config-dir", "/etc/baft", "BAFT config directory (tunnel jobs write baft.yaml, keys and PKI here)")
	baftState := fs.String("baft-state-dir", "/var/lib/baft", "BAFT service state directory")
	unitDir := fs.String("unit-dir", "/etc/systemd/system", "where the BAFT unit file lives")
	metricsListen := fs.String("metrics-listen", "127.0.0.1:9191", "loopback metrics address given to tunnels this agent builds")
	serviceUser := fs.String("service-user", "baft", "user the BAFT service runs as")
	interval := fs.Duration("interval", 30*time.Second, "poll interval")
	provisionPolicy := fs.Bool("provision-ingress-policy", false, "local root-only managed ingress policy bootstrap; does not enable Apply")
	ingressProfile := fs.String("ingress-profile", "", "with --provision-ingress-policy: empty = isolated LAB namespace policy; \"host-v1\" = safe host mode on an ordinary server (ADR-003; opt-in, see install.sh --enable-host-ingress)")
	reconcileMode := fs.String("ingress-reconcile", "auto", "post-restart reconciliation of interrupted operational ingress: auto (finish only with complete kernel/firewall/authority evidence), report (evaluate, write nothing) or off")
	reconcileOnly := fs.Bool("reconcile-ingress", false, "run the post-restart ingress reconciliation once, print the evidence and exit (1 if anything stays fenced)")
	once := fs.Bool("once", false, "handle pending jobs once and exit")
	allowHTTP := fs.Bool("allow-insecure-http", false, "allow a plain-HTTP BCC URL (testing only)")
	selfUpdateGuard := fs.Duration("self-update-guard", 0, "how long a self-updated agent has to reconnect to BCC before the previous agent is restored (default 3m)")
	selfUpdate := fs.Bool("self-update", true, "let signed update_baft jobs also replace this agent; a systemd timer restores the previous agent if the new one does not reconnect to BCC")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *provisionPolicy {
		if *nodeID == "" {
			fmt.Fprintln(stderr, "baft-agent: --provision-ingress-policy requires --node-id")
			return 2
		}
		if *ingressProfile != "" && *ingressProfile != ingressinventory.ExecutionProfileHostV1 {
			fmt.Fprintf(stderr, "baft-agent: --ingress-profile must be empty or %q\n", ingressinventory.ExecutionProfileHostV1)
			return 2
		}
		p, err := ingressauthority.ProvisionInstalledManagedNodePolicyProfile(ctx, *nodeID, *ingressProfile)
		if err != nil {
			if errors.Is(err, ingressauthority.ErrPolicyProfileConflict) {
				fmt.Fprintln(stderr, "baft-agent: ingress policy: a different managed ingress policy is already installed at /etc/baft/ingress/managed-node.json and may back existing public ingress; it was left unchanged")
				return 1
			}
			fmt.Fprintln(stderr, "baft-agent: ingress policy:", err)
			return 1
		}
		mode := "isolated LAB"
		if p.ExecutionProfile == ingressinventory.ExecutionProfileHostV1 {
			mode = "safe host mode"
		}
		fmt.Fprintf(stderr, "baft-agent: installed managed ingress policy (%s) for %s at %s; each public port still needs complete coverage, held authority and, in host mode, the operator's per-port acknowledgement\n", mode, p.NodeID, p.ProfilePath)
		return 0
	}
	if *bccURL == "" || *nodeID == "" {
		fmt.Fprintln(stderr, "baft-agent: --bcc-url and --node-id are required")
		return 2
	}
	mode, merr := parseReconcileMode(*reconcileMode)
	if merr != nil {
		fmt.Fprintln(stderr, "baft-agent:", merr)
		return 2
	}
	if *reconcileOnly && mode == agent.RestartReconcileOff {
		fmt.Fprintln(stderr, "baft-agent: --reconcile-ingress cannot be combined with --ingress-reconcile=off")
		return 2
	}
	if !strings.HasPrefix(*bccURL, "https://") && !*allowHTTP {
		fmt.Fprintln(stderr, "baft-agent: --bcc-url must be https:// (the agent token travels on it)")
		return 2
	}
	token, err := readSecret(*tokenFile)
	if err != nil {
		fmt.Fprintln(stderr, "baft-agent: token:", err)
		return 1
	}
	jobKey, err := release.ReadPublic(*jobKeyFile)
	if err != nil {
		fmt.Fprintln(stderr, "baft-agent: BCC job key:", err)
		return 1
	}
	cfg := agent.Config{
		BCCURL: *bccURL, NodeID: *nodeID, Token: token, BCCJobKey: jobKey, StateDir: *stateDir,
		IngressManifestDir: *ingressManifestDir,
		Service:            *service, ReleaseBaseURL: *releaseBase, RevocationsURL: *revocations,
		ReleaseState: *releaseState, BinDir: *binDir, System: hostSystem{},
	}
	tn, err := tunnelnode.New(tunnelnode.Env{
		ConfigDir: *configDir, StateDir: *baftState, UnitDir: *unitDir, Service: *service, User: *serviceUser,
		MetricsListen: *metricsListen, BaftBin: filepath.Join(*binDir, "baft"), PairBin: filepath.Join(*binDir, "baft-pair"), System: hostSystem{},
	})
	if err != nil {
		fmt.Fprintln(stderr, "baft-agent:", err)
		return 1
	}
	cfg.Tunnel = tn
	if root, err := release.ReadPublic(*rootFile); err == nil {
		cfg.ReleaseRoot = root
	} else if !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(stderr, "baft-agent: release root:", err)
		return 1
	} else {
		fmt.Fprintln(stderr, "baft-agent: no release root pinned; update_baft jobs will fail")
	}
	// Operational ingress is built only if every prerequisite verifies; any
	// failure leaves it nil (no capability header, v1 jobs BLOCKED). It never
	// falls back to the LAB executor or an in-memory firewall.
	logger := log.New(stderr, "", log.LstdFlags)
	if op, err := buildIngressOperational(ctx, *nodeID, *stateDir, hostIngressDeps(logger.Printf)); err != nil {
		// errors.Join separates causes with newlines; keep the whole cause on
		// one journal line so operators (and `journalctl | grep`) see why.
		logger.Printf("baft-agent: operational ingress disabled (v1 ingress jobs stay BLOCKED): %s", strings.ReplaceAll(err.Error(), "\n", "; "))
	} else {
		cfg.IngressOperational = op
		logger.Printf("baft-agent: operational ingress provisioned for %s", *nodeID)
	}
	if *selfUpdate && !*once {
		if unit := ownSystemdUnit("/proc/self/cgroup"); unit != "" {
			cfg.AgentUnit = unit
			cfg.AgentUpdateGuard = *selfUpdateGuard
		} else {
			logger.Printf("baft-agent: not running as a systemd service; update_baft jobs leave baft-agent unchanged")
		}
	}
	a, err := agent.New(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "baft-agent:", err)
		return 1
	}
	// Interrupted operational ingress is reconciled BEFORE any job can run.
	// It finishes an operation only with complete evidence (see
	// docs/release/managed-ingress-policy-bootstrap.md); otherwise it leaves
	// everything fenced exactly as before and logs why.
	fenced := reconcileIngressAtStartup(ctx, a, mode, logger.Printf)
	if *reconcileOnly {
		if fenced {
			return 1
		}
		return 0
	}
	if *once {
		results, err := a.RunOnce(ctx)
		for _, r := range results {
			logger.Printf("job %s %s: %s %s", r.JobID, r.Action, r.Status, r.Detail)
		}
		if err != nil {
			logger.Printf("baft-agent: %v", err)
			return 1
		}
		return 0
	}
	logger.Printf("baft-agent %s (%s) polling %s every %s", *nodeID, Version, *bccURL, *interval)
	a.Run(ctx, *interval, logger.Printf)
	return 0
}

// ownSystemdUnit returns the systemd service this process runs in (from its
// cgroup, e.g. "baft-agent"), or "" outside a service.
func ownSystemdUnit(cgroupFile string) string {
	b, err := os.ReadFile(cgroupFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		base := filepath.Base(parts[2])
		if strings.HasSuffix(base, ".service") && len(base) > len(".service") {
			return strings.TrimSuffix(base, ".service")
		}
	}
	return ""
}
