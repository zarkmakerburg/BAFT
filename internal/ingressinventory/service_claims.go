package ingressinventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Host-profile latent-claim detectors beyond Xray (ADR-003 §4.2 item 2):
// nginx `listen`, HAProxy `bind` and Docker published ports. Each reads only
// fixed, root-owned locations, never runs or signals the other service, and
// returns either claims or UNKNOWN. Unknown makes coverage incomplete, so an
// Apply is refused rather than guessed. A detector whose service is not
// installed (its location does not exist) contributes nothing.

const (
	maxServiceConfigBytes = 4 << 20
	maxServiceConfigFiles = 512
)

// hostServiceDetector is one detector; root is overridable only in tests.
type hostServiceDetector struct {
	name  string
	root  string
	parse func(root string) ([]ConfigPortClaim, []string)
}

var hostServiceDetectors = []hostServiceDetector{
	{name: "xray", parse: func(string) ([]ConfigPortClaim, []string) { return collectXrayInboundClaims(xrayConfigPaths) }},
	{name: "nginx", root: "/etc/nginx", parse: collectNginxListenClaims},
	{name: "haproxy", root: "/etc/haproxy", parse: collectHAProxyBindClaims},
	{name: "docker", root: "/var/lib/docker/containers", parse: collectDockerPublishedClaims},
}

// collectHostServiceClaims runs every host-profile detector.
func collectHostServiceClaims() (claims []ConfigPortClaim, unknown []string) {
	for _, d := range hostServiceDetectors {
		c, u := d.parse(d.root)
		claims = append(claims, c...)
		unknown = append(unknown, u...)
	}
	return claims, unknown
}

// readBoundedConfig reads one regular file within the size bound.
func readBoundedConfig(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxServiceConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxServiceConfigBytes {
		return nil, errors.New("too large")
	}
	return raw, nil
}

// configFiles lists the regular files (following symlinks to regular files,
// as nginx's sites-enabled uses) under root, skipping directories whose name
// says they are not loaded. A missing root is "not installed".
func configFiles(root string, skipDir func(string) bool) (files []string, missing bool, err error) {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	} else if err != nil {
		return nil, false, err
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxServiceConfigFiles {
			return errors.New("too many configuration files")
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	return files, false, err
}

func readFollowing(path string) ([]byte, error) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	return readBoundedConfig(target)
}

// stripHashComments removes `# ...` comments outside quotes.
func stripHashComments(line string) string {
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return line[:i]
		}
	}
	return line
}

// parsePortRange parses "N" or "A-B".
func parsePortRange(s string) (uint16, uint16, error) {
	lo, hi, isRange := strings.Cut(strings.TrimSpace(s), "-")
	a, err := strconv.ParseUint(lo, 10, 16)
	if err != nil || a == 0 {
		return 0, 0, fmt.Errorf("unsupported port %q", s)
	}
	b := a
	if isRange {
		b, err = strconv.ParseUint(hi, 10, 16)
		if err != nil || b < a {
			return 0, 0, fmt.Errorf("unsupported port range %q", s)
		}
	}
	return uint16(a), uint16(b), nil
}

// hostPortPart returns the port part of an address like "80", "*:80",
// "0.0.0.0:80", "[::]:80", "[::1]:80", "example.com:80".
func hostPortPart(addr string) string {
	if strings.HasPrefix(addr, "[") {
		if i := strings.LastIndex(addr, "]:"); i >= 0 {
			return addr[i+2:]
		}
		return ""
	}
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

// ---- nginx ----

// collectNginxListenClaims reads every loaded-looking file under the nginx
// configuration root ("*-available" directories are not loaded by the usual
// layouts) and records each TCP `listen` port. `listen unix:` takes no port.
// nginx's default port 80 is used when only an address is given.
func collectNginxListenClaims(root string) ([]ConfigPortClaim, []string) {
	files, missing, err := configFiles(root, func(name string) bool { return strings.HasSuffix(name, "-available") })
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, []string{"nginx configuration " + root + " unreadable"}
	}
	var claims []ConfigPortClaim
	var unknown []string
	for _, path := range files {
		raw, err := readFollowing(path)
		if err != nil {
			unknown = append(unknown, "nginx configuration "+path+" unreadable")
			continue
		}
		c, u := parseNginxListen(path, string(raw))
		claims = append(claims, c...)
		unknown = append(unknown, u...)
	}
	return claims, unknown
}

func parseNginxListen(source, text string) (claims []ConfigPortClaim, unknown []string) {
	for n, line := range strings.Split(text, "\n") {
		// Statements may share a line: split on ';' after removing comments.
		for _, stmt := range strings.Split(stripHashComments(line), ";") {
			fields := strings.Fields(stmt)
			// A listen directive may follow a '{' on the same line.
			for i := 0; i < len(fields); i++ {
				if fields[i] != "listen" || (i > 0 && !strings.HasSuffix(fields[i-1], "{")) {
					continue
				}
				if i+1 >= len(fields) {
					unknown = append(unknown, fmt.Sprintf("nginx %s:%d: listen without address", source, n+1))
					break
				}
				addr := strings.Trim(fields[i+1], "\"'")
				if strings.HasPrefix(addr, "unix:") {
					break
				}
				port := addr
				switch {
				case strings.HasPrefix(addr, "[") && !strings.Contains(addr, "]:"):
					port = "80" // [IPv6] only: nginx's default port
				case strings.Contains(addr, ":") || strings.HasPrefix(addr, "["):
					port = hostPortPart(addr)
				default:
					if _, err := strconv.Atoi(addr); err != nil {
						port = "80" // address only: nginx's default port
					}
				}
				lo, hi, err := parsePortRange(port)
				if err != nil {
					unknown = append(unknown, fmt.Sprintf("nginx %s:%d: %v", source, n+1, err))
					break
				}
				claims = append(claims, ConfigPortClaim{Kind: "nginx listen", Source: source, Tag: "line " + strconv.Itoa(n+1), From: lo, To: hi})
				break
			}
		}
	}
	return claims, unknown
}

// ---- HAProxy ----

func collectHAProxyBindClaims(root string) ([]ConfigPortClaim, []string) {
	files, missing, err := configFiles(root, func(string) bool { return false })
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, []string{"haproxy configuration " + root + " unreadable"}
	}
	var claims []ConfigPortClaim
	var unknown []string
	for _, path := range files {
		if !strings.HasSuffix(path, ".cfg") {
			continue
		}
		raw, err := readFollowing(path)
		if err != nil {
			unknown = append(unknown, "haproxy configuration "+path+" unreadable")
			continue
		}
		c, u := parseHAProxyBind(path, string(raw))
		claims = append(claims, c...)
		unknown = append(unknown, u...)
	}
	return claims, unknown
}

// parseHAProxyBind records `bind addr:port[,addr:port...]` in any section.
// Unix sockets, abstract namespaces and fd@ take no TCP port.
func parseHAProxyBind(source, text string) (claims []ConfigPortClaim, unknown []string) {
	for n, line := range strings.Split(text, "\n") {
		fields := strings.Fields(stripHashComments(line))
		if len(fields) < 2 || fields[0] != "bind" {
			continue
		}
		for _, addr := range strings.Split(fields[1], ",") {
			addr = strings.TrimSpace(addr)
			for _, prefix := range []string{"ipv4@", "ipv6@", "tcp@", "tcp4@", "tcp6@"} {
				addr = strings.TrimPrefix(addr, prefix)
			}
			if addr == "" || strings.HasPrefix(addr, "/") || strings.HasPrefix(addr, "unix@") ||
				strings.HasPrefix(addr, "abns@") || strings.HasPrefix(addr, "fd@") || strings.HasPrefix(addr, "sockpair@") ||
				strings.HasPrefix(addr, "udp") || strings.HasPrefix(addr, "quic") {
				continue
			}
			lo, hi, err := parsePortRange(hostPortPart(addr))
			if err != nil {
				unknown = append(unknown, fmt.Sprintf("haproxy %s:%d: %v", source, n+1, err))
				continue
			}
			claims = append(claims, ConfigPortClaim{Kind: "haproxy bind", Source: source, Tag: "line " + strconv.Itoa(n+1), From: lo, To: hi})
		}
	}
	return claims, unknown
}

// ---- Docker ----

// collectDockerPublishedClaims reads each container's hostconfig.json
// (running and stopped: a stopped container with a restart policy claims its
// port again later). An empty HostPort is an ephemeral port chosen at start,
// not a fixed claim. UDP bindings do not claim a TCP port.
func collectDockerPublishedClaims(root string) ([]ConfigPortClaim, []string) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{"docker container store " + root + " unreadable"}
	}
	if len(entries) > maxServiceConfigFiles {
		return nil, []string{"docker container store " + root + " has too many containers to inspect"}
	}
	var claims []ConfigPortClaim
	var unknown []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), "hostconfig.json")
		raw, err := readBoundedConfig(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			unknown = append(unknown, "docker "+path+" unreadable")
			continue
		}
		c, u := parseDockerHostConfig(path, e.Name(), raw)
		claims = append(claims, c...)
		unknown = append(unknown, u...)
	}
	return claims, unknown
}

func parseDockerHostConfig(source, container string, raw []byte) (claims []ConfigPortClaim, unknown []string) {
	var hc struct {
		PortBindings map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"PortBindings"`
	}
	if err := json.Unmarshal(raw, &hc); err != nil {
		return nil, []string{"docker " + source + " is not parseable JSON"}
	}
	short := container
	if len(short) > 12 {
		short = short[:12]
	}
	// A host-network container declares no ports; while running, its
	// listeners are seen by the live-socket check. A stopped one is the same
	// residual as a hand-run daemon and is covered by the acknowledgement.
	for spec, bindings := range hc.PortBindings {
		if !strings.HasSuffix(spec, "/tcp") && strings.Contains(spec, "/") {
			continue // udp/sctp
		}
		for _, b := range bindings {
			if strings.TrimSpace(b.HostPort) == "" {
				continue // ephemeral
			}
			lo, hi, err := parsePortRange(b.HostPort)
			if err != nil {
				unknown = append(unknown, fmt.Sprintf("docker container %s %s: %v", short, spec, err))
				continue
			}
			claims = append(claims, ConfigPortClaim{Kind: "docker published port", Source: source, Tag: short + " " + spec, From: lo, To: hi})
		}
	}
	return claims, unknown
}
