package bcc

import "fmt"

// WizardPhase represents a backend-confirmed stage, not an optimistic UI event.
type WizardPhase string

const (
	WizardPreview  WizardPhase = "preview"
	WizardPrepare  WizardPhase = "prepare"
	WizardCommit   WizardPhase = "commit"
	WizardFinalize WizardPhase = "finalize"
)

// WizardProgress reports completed stages only after backend confirmation.
// A failure never advances the progress bar or claims deployment success.
func WizardProgress(phase WizardPhase, confirmed bool) (int, string, error) {
	if !confirmed {
		return 0, "awaiting backend confirmation", nil
	}
	switch phase {
	case WizardPreview:
		return 25, "preview verified", nil
	case WizardPrepare:
		return 50, "prepare verified", nil
	case WizardCommit:
		return 75, "commit verified", nil
	case WizardFinalize:
		return 100, "deployment verified", nil
	default:
		return 0, "", fmt.Errorf("unknown wizard phase %q", phase)
	}
}
