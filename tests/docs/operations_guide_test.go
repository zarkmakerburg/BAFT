package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The v0.4.0 operations guide may only show commands, flags and HTTP routes
// that exist in the code. Each shell block of the guide is parsed; every
// --flag is looked up in the program that receives it, every /api/ route in
// the BCC mux. The Persian page must carry the same command blocks.

const opsGuide = "docs/release/v0.4.0-operations.md"
const opsGuideFA = "docs/release/v0.4.0-operations.fa.md"

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func goSources(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Join(repoRoot(t), filepath.FromSlash(dir))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		sb.WriteString(readRepoFile(t, dir+"/"+e.Name()))
	}
	return sb.String()
}

type fence struct{ lang, body string }

func fences(md string) []fence {
	var out []fence
	var cur *fence
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			if cur == nil {
				cur = &fence{lang: strings.TrimPrefix(t, "```")}
			} else {
				out = append(out, *cur)
				cur = nil
			}
			continue
		}
		if cur != nil {
			cur.body += line + "\n"
		}
	}
	return out
}

// logicalLines joins shell continuations and drops comments and blanks.
func logicalLines(body string) []string {
	var out []string
	var acc string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "\\") {
			acc += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		line = acc + line
		acc = ""
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func flagsIn(fields []string) []string {
	var out []string
	for _, f := range fields {
		if strings.HasPrefix(f, "--") && len(f) > 2 {
			out = append(out, strings.SplitN(strings.TrimPrefix(f, "--"), "=", 2)[0])
		}
	}
	return out
}

func TestOperationsGuideCommandsExistInCode(t *testing.T) {
	guide := readRepoFile(t, opsGuide)
	installer := readRepoFile(t, "install.sh")
	installerFlag := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+((?:--?[a-z0-9-]+\|?)+)\)`).FindAllStringSubmatch(installer, -1) {
		for _, f := range strings.Split(m[1], "|") {
			installerFlag[strings.TrimLeft(f, "-")] = true
		}
	}
	if !installerFlag["bcc-only"] || !installerFlag["provision-ingress-policy"] {
		t.Fatalf("installer flag extraction is broken: %v", installerFlag)
	}
	agentSrc := goSources(t, "cmd/baft-agent")
	bccSrc := goSources(t, "cmd/baft-bcc")
	baftSrc := goSources(t, "cmd/baft")
	tunnelSrc := readRepoFile(t, "cmd/baft/tunnel.go")
	uninstallSrc := readRepoFile(t, "cmd/baft/uninstall.go")
	routes := map[string]bool{}
	for _, m := range regexp.MustCompile(`HandleFunc\("(/api/[a-z0-9/_-]+)"`).FindAllStringSubmatch(goSources(t, "internal/bcc"), -1) {
		routes[m[1]] = true
	}
	has := func(src, flag string) bool { return strings.Contains(src, `"`+flag+`"`) }

	checked := 0
	for _, f := range fences(guide) {
		if f.lang == "ini" {
			for _, line := range logicalLines(f.body) {
				if !strings.HasPrefix(line, "ExecStart=/") {
					continue
				}
				for _, fl := range flagsIn(strings.Fields(line)) {
					if !has(bccSrc, fl) {
						t.Errorf("ini ExecStart: baft-bcc has no --%s", fl)
					}
					checked++
				}
			}
			continue
		}
		if f.lang != "sh" {
			continue
		}
		for _, line := range logicalLines(f.body) {
			fields := strings.Fields(line)
			i := 0
			for i < len(fields) && (fields[i] == "sudo" || fields[i] == "env" || fields[i] == "export" || strings.Contains(strings.Split(fields[i], "$")[0], "=")) {
				i++
			}
			if i < len(fields) && fields[i] == "bash" {
				i++
			}
			if i >= len(fields) {
				continue
			}
			cmd, rest := fields[i], fields[i+1:]
			switch {
			case cmd == "install.sh":
				for _, fl := range flagsIn(rest) {
					if !installerFlag[fl] {
						t.Errorf("%q: install.sh has no --%s", line, fl)
					}
					checked++
				}
			case cmd == "baft-agent":
				for _, fl := range flagsIn(rest) {
					if !has(agentSrc, fl) {
						t.Errorf("%q: baft-agent has no --%s", line, fl)
					}
					checked++
				}
			case cmd == "baft-bcc":
				sub := ""
				if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
					sub = rest[0]
				}
				if sub != "" && !regexp.MustCompile(`os\.Args\[1\]=="`+sub+`"`).MatchString(bccSrc) {
					t.Errorf("%q: baft-bcc has no subcommand %q", line, sub)
				}
				for _, fl := range flagsIn(rest) {
					if !has(bccSrc, fl) {
						t.Errorf("%q: baft-bcc has no --%s", line, fl)
					}
					checked++
				}
			case cmd == "baft":
				if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
					if !regexp.MustCompile(`case "`+rest[0]+`"`).MatchString(baftSrc) {
						t.Errorf("%q: baft has no subcommand %q", line, rest[0])
					}
				}
				src := baftSrc
				if len(rest) > 0 && rest[0] == "tunnel" {
					src = tunnelSrc
					if len(rest) > 1 && !strings.Contains(src, `"`+rest[1]+`"`) {
						t.Errorf("%q: baft tunnel has no action %q", line, rest[1])
					}
				}
				if len(rest) > 0 && rest[0] == "uninstall" {
					src = uninstallSrc
				}
				for _, fl := range flagsIn(rest) {
					if !has(src, fl) {
						t.Errorf("%q: baft %v has no --%s", line, rest[:1], fl)
					}
					checked++
				}
			case cmd == "curl":
				for _, tok := range rest {
					if i := strings.Index(tok, "/api/"); i >= 0 {
						route := strings.TrimRight(strings.SplitN(tok[i:], "?", 2)[0], `"'`)
						if !routes[route] {
							t.Errorf("%q: BCC has no route %s", line, route)
						}
						checked++
					}
				}
			}
		}
	}
	if checked < 40 {
		t.Fatalf("only %d flags/routes were checked; the guide parser is not seeing the commands", checked)
	}
	// Routes named in prose (`GET /api/...`, `POST /api/...`).
	for _, m := range regexp.MustCompile("`(?:GET|POST) (/api/[a-z0-9/_-]+)").FindAllStringSubmatch(guide, -1) {
		if !routes[m[1]] {
			t.Errorf("prose names route %s, which BCC does not serve", m[1])
		}
	}
	// Inline flags of the programs, in prose.
	for _, m := range regexp.MustCompile("`(--[a-z0-9-]+)").FindAllStringSubmatch(guide, -1) {
		fl := strings.TrimPrefix(m[1], "--")
		if !installerFlag[fl] && !has(agentSrc, fl) && !has(bccSrc, fl) && !has(tunnelSrc, fl) && !has(baftSrc, fl) && !has(uninstallSrc, fl) {
			t.Errorf("prose names --%s, which no program defines", fl)
		}
	}
}

func TestOperationsGuideLinksResolveAndPersianMirrorsCommands(t *testing.T) {
	en := readRepoFile(t, opsGuide)
	fa := readRepoFile(t, opsGuideFA)
	for _, md := range []struct{ name, text string }{{opsGuide, en}, {opsGuideFA, fa}} {
		for _, m := range regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]*)?\)`).FindAllStringSubmatch(md.text, -1) {
			target := filepath.Join(repoRoot(t), filepath.Dir(filepath.FromSlash(md.name)), filepath.FromSlash(m[1]))
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s links to missing %s", md.name, m[1])
			}
		}
	}
	norm := func(md string) []string {
		var out []string
		for _, f := range fences(md) {
			out = append(out, f.lang+"\n"+f.body)
		}
		return out
	}
	e, p := norm(en), norm(fa)
	if len(e) != len(p) {
		t.Fatalf("English guide has %d code blocks, Persian has %d", len(e), len(p))
	}
	for i := range e {
		if e[i] != p[i] {
			t.Errorf("code block %d differs between the English and Persian guide:\n--- en\n%s--- fa\n%s", i+1, e[i], p[i])
		}
	}
}
