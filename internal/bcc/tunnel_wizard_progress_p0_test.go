package bcc

import "testing"

func TestWizardProgressRequiresBackendConfirmation(t *testing.T) {
	phases := []struct {
		phase   WizardPhase
		percent int
	}{{WizardPreview, 25}, {WizardPrepare, 50}, {WizardCommit, 75}, {WizardFinalize, 100}}
	for _, tc := range phases {
		if p, _, err := WizardProgress(tc.phase, false); err != nil || p != 0 {
			t.Fatalf("unconfirmed %s: %d %v", tc.phase, p, err)
		}
		if p, _, err := WizardProgress(tc.phase, true); err != nil || p != tc.percent {
			t.Fatalf("confirmed %s: %d %v", tc.phase, p, err)
		}
	}
	for _, confirmed := range []bool{false, true} {
		if _, _, err := WizardProgress("unknown", confirmed); err == nil {
			t.Fatalf("unknown stage accepted with confirmed=%t", confirmed)
		}
	}
}
