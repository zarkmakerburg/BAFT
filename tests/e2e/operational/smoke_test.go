//go:build linux

package operational

import (
	"context"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

func TestSmoke(t *testing.T) {
	oplab.Require(t)
	ctx := context.Background()
	p, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, "ir-opv1")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", p)
	ev, err := ingressinventory.CollectInstalledManagedTCP(ctx, "ir-opv1", 54700)
	t.Logf("%+v %v", ev, err)
}
