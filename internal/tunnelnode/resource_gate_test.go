package tunnelnode

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// This regression can also run unchanged on the predecessor: reflect sets
// the explicit gate when supported, exposing the earlier unconditional prepare.
func TestResourceGateRefusesActualOccupiedBindBeforeMutation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("passive inventory requires Linux")
	}
	n := newNode(t)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p := ExParams{PublicAddress: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port, Target: "127.0.0.1:2443", RouteID: "r", MetricsListen: "127.0.0.1:59191"}
	f := reflect.ValueOf(&p).Elem().FieldByName("VerifyResources")
	if f.IsValid() {
		f.SetBool(true)
	}
	if _, err := n.PrepareEX(context.Background(), "occupied", p); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("prepare did not reject actual occupied listener before mutation: %v", err)
	}
	for _, path := range []string{n.noiseKey(), n.activePath(), n.stage("occupied"), n.liveConfig(), n.unitPath()} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("refused prepare mutated %s", filepath.Base(path))
		}
	}
}
