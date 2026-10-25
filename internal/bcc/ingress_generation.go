package bcc

// ingressOperationRetired reports a recorded Agent retire receipt (phase
// "retired"). It is inert evidence: absence is still verified elsewhere.
func ingressOperationRetired(op PublicIngressOperation) bool {
	return op.RetireReceipt != nil && op.RetireReceipt.Phase == "retired"
}

// nextIngressGenerationLocked is the generation a NEW operation on this IR
// node and public port must carry: one more than the highest generation whose
// ingress was retired there (1 for a port never used). The Agent independently
// refuses any generation that is not strictly greater than its released one.
func (s *Store) nextIngressGenerationLocked(irNode string, port int) uint64 {
	var highest uint64
	for _, op := range s.st.PublicIngressOperations {
		if op.Spec.IRNode == irNode && op.Spec.PublicPort == port && ingressOperationRetired(op) && op.Proof.Generation > highest {
			highest = op.Proof.Generation
		}
	}
	return highest + 1
}
