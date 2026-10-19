package installer_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the generated production Bash with an isolated fake kernel sysctl
// store. No host sysctls, modules or persistent configuration are modified.
func TestNetworkToolRuntimeRollback(t *testing.T) {
	if os.Geteuid() != 0 {
		if os.Getenv("BAFT_REQUIRE_NETWORK_TESTS") == "1" {
			t.Fatal("network rollback acceptance requires root")
		}
		t.Skip("generated tool deliberately requires root")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		if os.Getenv("BAFT_REQUIRE_NETWORK_TESTS") == "1" {
			t.Fatal("network rollback acceptance requires flock")
		}
		t.Skip("flock unavailable")
	}
	_, file, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(src), "    cat <<'NET'\n")
	if start < 0 {
		t.Fatal("NET heredoc missing")
	}
	body := string(src)[start+len("    cat <<'NET'\n"):]
	end := strings.Index(body, "\nNET\n")
	if end < 0 {
		t.Fatal("NET terminator missing")
	}
	body = body[:end]
	initial := map[string]string{
		"net.core.default_qdisc": "cake", "net.ipv4.tcp_congestion_control": "reno",
		"net.core.rmem_max": "212992", "net.core.wmem_max": "212993",
		"net.ipv4.tcp_rmem": "4096 87380 6291456", "net.ipv4.tcp_wmem": "4096\t16384\t4194304",
		"net.ipv4.tcp_mtu_probing": "0", "net.ipv4.tcp_slow_start_after_idle": "1",
		"net.ipv4.tcp_notsent_lowat": "4294967295", "net.core.somaxconn": "128",
		"net.ipv4.tcp_max_syn_backlog": "256", "net.ipv4.ip_forward": "1",
	}
	for _, scenario := range []string{"roundtrip", "partial-apply", "restore-retry", "legacy", "symlink", "no-bbr", "missing-key", "busy-lock", "invalid-snapshot", "state-symlink", "rollback-retry", "cleanup-failure", "snapshot-write-fail", "snapshot-lost-bbr", "profile-write-fail", "profile-body-write-fail"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			store := filepath.Join(root, "kernel")
			bin := filepath.Join(root, "bin")
			for _, d := range []string{store, bin} {
				if err := os.Mkdir(d, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for k, v := range initial {
				if err := os.WriteFile(filepath.Join(store, k), []byte(v+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write := func(p, s string) {
				t.Helper()
				if err := os.WriteFile(p, []byte(s), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(bin, "modprobe"), "#!/bin/bash\nexit 0\n")
			write(filepath.Join(bin, "sysctl"), `#!/bin/bash
set -eu
if [[ "$1" == -n ]]; then
 if [[ "$2" == net.ipv4.tcp_available_congestion_control ]]; then echo "${FAKE_CC:-reno cubic bbr}"; exit; fi
 cat "$FAKE_KERNEL/$2"; exit
fi
[[ "$1" == -q && "$2" == -p ]] || exit 90
while IFS= read -r line; do
 [[ "$line" == net.* ]] || continue
 key="${line%% =*}"; value="${line#* = }"
 if [[ -f "$FAKE_KERNEL/fail" && "$key" == net.core.wmem_max ]]; then
   [[ -f "$FAKE_KERNEL/persistent" ]] || rm "$FAKE_KERNEL/fail"
   exit 1
 fi
 printf '%s\n' "$value" >"$FAKE_KERNEL/$key"
done <"$3"
`)
			profile := filepath.Join(root, "99-baft-net.conf")
			tool := filepath.Join(root, "baft-net")
			// Fail shell writes without changing the production body. A successful
			// but truncated snapshot also exercises its pre-apply validation.
			faults := `printf() {
  if [[ "$1" == '%s = %s\n' ]]; then
    [[ "${FAKE_IO_FAULT:-}" != snapshot-write-fail ]] || return 1
    if [[ "${FAKE_IO_FAULT:-}" == snapshot-lost-bbr && ( "$2" == net.core.default_qdisc || "$2" == net.ipv4.tcp_congestion_control ) ]]; then return 0; fi
  fi
  if [[ "${FAKE_IO_FAULT:-}" == profile-write-fail && "$1" == '%s\n' && "$2" == '# Written by baft-net.'* ]]; then return 1; fi
  builtin printf "$@"
}
cat() {
  if [[ "${FAKE_IO_FAULT:-}" == profile-body-write-fail && "$#" == 0 ]]; then return 1; fi
  command cat "$@"
}
`
			write(tool, fmt.Sprintf("#!/bin/bash\nSYSCTL_FILE=%q\n", profile)+faults+body)
			env := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FAKE_KERNEL="+store)
			env = append(env, "FAKE_IO_FAULT="+scenario)
			if scenario == "no-bbr" {
				env = append(env, "FAKE_CC=reno cubic")
			}
			run := func(action string, success bool) {
				t.Helper()
				c := exec.Command("bash", tool, action)
				c.Env = env
				out, e := c.CombinedOutput()
				if (e == nil) != success {
					t.Fatalf("%s: err=%v output=%s", action, e, out)
				}
			}
			absent := func(path string) {
				t.Helper()
				if _, e := os.Lstat(path); !os.IsNotExist(e) {
					t.Fatalf("expected absent %s, err=%v", path, e)
				}
			}
			checkOriginal := func() {
				t.Helper()
				for k, want := range initial {
					got, e := os.ReadFile(filepath.Join(store, k))
					if e != nil || strings.TrimSpace(string(got)) != want {
						t.Errorf("%s got %q err=%v want %q", k, got, e, want)
					}
				}
			}
			switch scenario {
			case "snapshot-write-fail", "snapshot-lost-bbr", "profile-write-fail", "profile-body-write-fail":
				run("on", false)
				checkOriginal()
				absent(profile)
				absent(profile + ".state/before.conf")
			case "legacy":
				write(profile, "net.core.rmem_max = 999\n")
				run("off", false)
				run("on", false)
				checkOriginal()
			case "symlink":
				victim := filepath.Join(root, "victim")
				write(victim, "keep me")
				if e := os.Symlink(victim, profile); e != nil {
					t.Fatal(e)
				}
				run("on", false)
				run("off", false)
				got, _ := os.ReadFile(victim)
				if string(got) != "keep me" {
					t.Fatal("symlink target changed")
				}
				checkOriginal()
			case "state-symlink":
				victim := filepath.Join(root, "victim-state")
				if e := os.Mkdir(victim, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(victim, profile+".state"); e != nil {
					t.Fatal(e)
				}
				run("on", false)
				checkOriginal()
			case "invalid-snapshot":
				run("on", true)
				write(profile+".state/before.conf", "net.ipv4.ip_forward = 0\n")
				run("off", false)
				if _, e := os.Stat(profile); e != nil {
					t.Fatal("invalid snapshot removed profile")
				}
				got, _ := os.ReadFile(filepath.Join(store, "net.ipv4.ip_forward"))
				if strings.TrimSpace(string(got)) != "1" {
					t.Fatal("restore changed unrelated key")
				}
			case "missing-key":
				os.Remove(filepath.Join(store, "net.core.wmem_max"))
				run("on", false)
				absent(profile)
				absent(profile + ".state/before.conf")
			case "busy-lock":
				state := profile + ".state"
				os.Mkdir(state, 0700)
				locker := exec.Command("flock", filepath.Join(state, "lock"), "bash", "-c", "echo ready; cat")
				stdin, e := locker.StdinPipe()
				if e != nil {
					t.Fatal(e)
				}
				stdout, e := locker.StdoutPipe()
				if e != nil {
					t.Fatal(e)
				}
				if e = locker.Start(); e != nil {
					t.Fatal(e)
				}
				ready := make([]byte, 6)
				if _, e = stdout.Read(ready); e != nil {
					t.Fatal(e)
				}
				run("on", false)
				stdin.Close()
				locker.Wait()
				checkOriginal()
			case "cleanup-failure":
				run("on", true)
				if e := os.Remove(profile); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(profile, 0700); e != nil {
					t.Fatal(e)
				}
				run("off", false)
				if _, e := os.Stat(profile + ".state/before.conf"); e != nil {
					t.Fatal("failed profile cleanup lost snapshot")
				}
				if e := os.Remove(profile); e != nil {
					t.Fatal(e)
				}
				run("off", true)
				checkOriginal()
			case "rollback-retry":
				write(filepath.Join(store, "fail"), "failure")
				write(filepath.Join(store, "persistent"), "retain failure")
				run("on", false)
				if _, e := os.Stat(profile + ".state/before.conf"); e != nil {
					t.Fatal("failed rollback lost snapshot")
				}
				os.Remove(filepath.Join(store, "fail"))
				os.Remove(filepath.Join(store, "persistent"))
				run("off", true)
				checkOriginal()
				absent(profile)
			case "partial-apply":
				write(filepath.Join(store, "fail"), "once")
				run("on", false)
				checkOriginal()
				absent(profile)
				absent(profile + ".state/before.conf")
			default:
				run("on", true)
				snap, e := os.ReadFile(profile + ".state/before.conf")
				if e != nil {
					t.Fatal(e)
				}
				st, e := os.Stat(profile + ".state/before.conf")
				if e != nil || st.Mode().Perm() != 0600 {
					t.Fatalf("snapshot permissions: %v %v", st, e)
				}
				run("on", true)
				again, _ := os.ReadFile(profile + ".state/before.conf")
				if string(snap) != string(again) {
					t.Fatal("repeat on overwrote original snapshot")
				}
				if scenario == "restore-retry" {
					write(filepath.Join(store, "fail"), "once")
					run("off", false)
					if _, e = os.Stat(profile + ".state/before.conf"); e != nil {
						t.Fatal("failed restore lost snapshot")
					}
				}
				run("off", true)
				checkOriginal()
				absent(profile)
				absent(profile + ".state/before.conf")
				run("off", true)
			}
		})
	}
}
