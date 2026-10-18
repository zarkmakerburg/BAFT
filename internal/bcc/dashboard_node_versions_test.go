package bcc

import (
 "strings"
 "testing"
)

func TestDashboardNodeVersionSecurityContract(t *testing.T) {
 checks:=[]string{
  "BAFT / Agent / BCC",
  "id=\"nodeCards\"",
  "function renderNodeCards(versionMap)",
  "t.ex_node===n.id||t.ir_node===n.id",
  "onclick=\"tnEdit(this.dataset.id)\"",
  "onclick=\"tnDrift(this.dataset.id)\"",
  "Edit / ویرایش",
  "renderNodeCards(window.nodeVersionMap||Object.create(null))",
  "No tunnels / بدون تونل",
  "id=\"catalogHealth\"",
  "catalogStatus",
  "Release catalog: ",
  "async function loadNodes()",
 "Object.create(null)",
 "typeof item.node_id",
 "Array.isArray(item.components)",
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
