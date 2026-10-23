package ingressmanifest

import "testing"

func TestPreviewMatchesCanonicalPreparedManifest(t *testing.T) {
	s, _ := fixtureStore(t)
	spec := fixtureSpec()
	preview, err := Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := s.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview != prepared {
		t.Fatalf("preview differs from durable prepared bytes: %+v %+v", preview, prepared)
	}
	spec.Config.Target = "0.0.0.0:1443"
	if _, err := Preview(spec); err == nil {
		t.Fatal("invalid target was previewed")
	}
}
