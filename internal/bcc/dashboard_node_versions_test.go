package bcc

import (
 "strings"
 "testing"
)

func TestDashboardNodeVersionSecurityContract(t *testing.T) {
 checks:=[]string{
  "BAFT / Agent / BCC",
  "id=\"catalogHealth\"",
  "catalogStatus",
  "Release catalog: ",
  "async function loadNodes()",
  "fetch('/api/nodes/versions',{headers:ah()})",
  "function componentVersionsHTML(item)",
  "['transport','agent','bcc']",
  "state==='verified'&&c.version?c.version:state",
  "state==='mismatch'?'down':'unknown'",
  "esc(label)",
 }
 for _,want:=range checks{
  if !strings.Contains(dashboardHTML,want){t.Errorf("node version dashboard contract missing %q",want)}
 }
 if strings.Contains(dashboardHTML,"fetch('/api/nodes/versions')"){t.Fatal("version endpoint request lacks auth headers")}
}
