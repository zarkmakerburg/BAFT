package ingressinventory

import (
	"os"
	"path/filepath"
	"testing"
)

func covers(claims []ConfigPortClaim, port uint16) bool {
	for _, c := range claims {
		if c.Covers(port) {
			return true
		}
	}
	return false
}

func TestParseNginxListen(t *testing.T) {
	conf := `
# listen 9999;  a comment is not a claim
http {
  server { listen 80; server_name a; }
  server {
    listen [::]:443 ssl http2;
    listen 127.0.0.1:8080 default_server;
    listen 10.0.0.1;            # address only -> default port 80
    listen [::1];
    listen unix:/run/nginx.sock;
    proxy_pass http://127.0.0.1:7777;
    access_log /var/log/nginx/listen.log;
  }
}
stream { server { listen 2053; proxy_pass 127.0.0.1:1444; } }
`
	claims, unknown := parseNginxListen("nginx.conf", conf)
	if len(unknown) != 0 {
		t.Fatalf("unknown: %v", unknown)
	}
	for _, p := range []uint16{80, 443, 8080, 2053} {
		if !covers(claims, p) {
			t.Fatalf("port %d not claimed: %+v", p, claims)
		}
	}
	for _, p := range []uint16{9999, 7777, 1444} {
		if covers(claims, p) {
			t.Fatalf("port %d wrongly claimed: %+v", p, claims)
		}
	}
	if _, u := parseNginxListen("x", "server { listen ; }"); len(u) != 1 {
		t.Fatalf("listen without address must be unknown: %v", u)
	}
	if _, u := parseNginxListen("x", "server { listen 127.0.0.1:http; }"); len(u) != 1 {
		t.Fatalf("non-numeric port must be unknown: %v", u)
	}
}

func TestCollectNginxSkipsAvailableAndFollowsEnabledLinks(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "sites-available"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "sites-enabled"), 0o755))
	must(os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("events {}\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "sites-available", "on"), []byte("server { listen 8443; }\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "sites-available", "off"), []byte("server { listen 9443; }\n"), 0o644))
	must(os.Symlink(filepath.Join(root, "sites-available", "on"), filepath.Join(root, "sites-enabled", "on")))
	claims, unknown := collectNginxListenClaims(root)
	if len(unknown) != 0 || !covers(claims, 8443) || covers(claims, 9443) {
		t.Fatalf("claims=%+v unknown=%v", claims, unknown)
	}
	if c, u := collectNginxListenClaims(filepath.Join(root, "absent")); len(c)+len(u) != 0 {
		t.Fatal("nginx not installed must contribute nothing")
	}
	must(os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "sites-enabled", "dangling")))
	if _, u := collectNginxListenClaims(root); len(u) != 1 {
		t.Fatalf("an unreadable enabled site must be unknown: %v", u)
	}
}

func TestParseHAProxyBind(t *testing.T) {
	cfg := `
global
  stats socket /run/haproxy/admin.sock mode 660
frontend fe
  bind :443 ssl crt /etc/ssl/x.pem
  bind *:80,[::]:8080
  bind ipv6@:2096 v4v6
  bind 0.0.0.0:9000-9002
  bind unix@/run/hap.sock
  bind abns@internal
  bind quic4@:443
  # bind :7000
`
	claims, unknown := parseHAProxyBind("haproxy.cfg", cfg)
	if len(unknown) != 0 {
		t.Fatalf("unknown: %v", unknown)
	}
	for _, p := range []uint16{443, 80, 8080, 2096, 9000, 9001, 9002} {
		if !covers(claims, p) {
			t.Fatalf("port %d not claimed: %+v", p, claims)
		}
	}
	if covers(claims, 7000) || covers(claims, 9003) {
		t.Fatalf("over-claimed: %+v", claims)
	}
	if _, u := parseHAProxyBind("x", "bind :${PORT}\n"); len(u) != 1 {
		t.Fatalf("environment port must be unknown: %v", u)
	}
}

func TestDockerPublishedPorts(t *testing.T) {
	root := t.TempDir()
	write := func(id, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, id), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, id, "hostconfig.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("aaaaaaaaaaaaaaaaaaaa", `{"NetworkMode":"bridge","PortBindings":{"80/tcp":[{"HostIp":"","HostPort":"8081"}],"443/tcp":[{"HostIp":"::","HostPort":"8443"}],"53/udp":[{"HostIp":"","HostPort":"5353"}],"9000/tcp":[{"HostIp":"","HostPort":""}],"7000/tcp":[{"HostIp":"","HostPort":"7000-7002"}]}}`)
	write("bbbbbbbbbbbbbbbbbbbb", `{"NetworkMode":"host","PortBindings":{}}`)
	claims, unknown := collectDockerPublishedClaims(root)
	if len(unknown) != 0 {
		t.Fatalf("unknown: %v", unknown)
	}
	for _, p := range []uint16{8081, 8443, 7000, 7002} {
		if !covers(claims, p) {
			t.Fatalf("port %d not claimed: %+v", p, claims)
		}
	}
	for _, p := range []uint16{5353, 9000, 80, 443} {
		if covers(claims, p) {
			t.Fatalf("port %d wrongly claimed (udp, ephemeral or container-side): %+v", p, claims)
		}
	}
	write("cccccccccccccccccccc", `{not json`)
	if _, u := collectDockerPublishedClaims(root); len(u) != 1 {
		t.Fatalf("unparseable hostconfig must be unknown: %v", u)
	}
	if c, u := collectDockerPublishedClaims(filepath.Join(root, "absent")); len(c)+len(u) != 0 {
		t.Fatal("docker not installed must contribute nothing")
	}
}
