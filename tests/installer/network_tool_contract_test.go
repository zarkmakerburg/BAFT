package installer_test

import (
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

func TestNetworkToolSafetyContracts(t *testing.T) {
 _, current, _, ok := runtime.Caller(0)
 if !ok { t.Fatal("cannot resolve test source") }
 data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "..", "install.sh"))
 if err != nil { t.Fatal(err) }
 script := string(data)
 for _, forbidden := range []string{
  "sysctl -q -w net.ipv4.tcp_congestion_control=cubic",
  "sysctl -q -w net.core.default_qdisc=fq_codel",
  "sysctl -q -p \"$SYSCTL_FILE\" >/dev/null 2>&1 || true",
 } {
  if strings.Contains(script, forbidden) { t.Errorf("unsafe network tuning pattern still present: %s", forbidden) }
 }
 for _, required := range []string{
  "if ! sysctl -q -p \"$SYSCTL_FILE\"",
  "if ! sysctl -q --system",
  "invalid TCP port: enter a number from 1 to 65535",
  "Choose: \" c || return 0",
 } {
  if !strings.Contains(script, required) { t.Errorf("network safety guard missing: %s", required) }
 }
}
