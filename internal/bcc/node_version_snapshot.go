package bcc

// ComponentVersionSnapshot returns node metadata and authenticated digest
// receipts from one consistent state lock. Never return the mutable digests
// maps directly to the caller.
func (s *Store) ComponentVersionSnapshot() ([]Node,map[string]ComponentDigestReceipt) {
 s.mu.Lock()
 defer s.mu.Unlock()
 nodes:=make([]Node,0,len(s.st.Nodes))
 for _,node:=range s.st.Nodes{nodes=append(nodes,publicNode(node))}
 receipts:=make(map[string]ComponentDigestReceipt,len(s.st.ComponentDigestReceipts))
 for nodeID,r:=range s.st.ComponentDigestReceipts{
  copyR:=r
  copyR.Digests=make(map[string]string,len(r.Digests))
  for k,v:=range r.Digests{copyR.Digests[k]=v}
  receipts[nodeID]=copyR
 }
 return nodes,receipts
}
