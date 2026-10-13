package tunnelnode

import (
	"context"
	"errors"
	"testing"
)

type healthSampleSystem struct {
	System
	restarts string
	err      error
}

func (s healthSampleSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	if args[0] == "show" {
		return s.restarts, s.err
	}
	return s.System.Systemctl(ctx, args...)
}

func TestHealthDoesNotAcceptMissingOrInvalidRestartEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		err         error
	}{
		{"counter query failed", "", errors.New("show unavailable")},
		{"empty counter", "", nil}, {"malformed counter", "unknown", nil}, {"negative counter", "-1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := newNode(t)
			n.host.active = true
			n.System = healthSampleSystem{System: n.host, restarts: tc.value, err: tc.err}
			if _, err := n.stableSample(context.Background()); err == nil {
				t.Fatal("missing restart evidence accepted as a healthy sample")
			}
		})
	}
}
